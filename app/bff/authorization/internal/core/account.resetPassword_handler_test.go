package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/pkg/twofa"
	"github.com/zeromicro/go-zero/core/logx"
)

type memoryPasswordResetStore struct {
	values map[string]string
	writes int
}

func (s *memoryPasswordResetStore) Get(key string) (string, error) {
	return s.values[key], nil
}

func (s *memoryPasswordResetStore) Set(key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	s.writes++
	return nil
}

func TestAccountResetPasswordKeepsOriginalRequestedDeadline(t *testing.T) {
	const userID = int64(712346)
	store := &memoryPasswordResetStore{values: make(map[string]string)}
	setResetPasswordState(t, store, userID, twofa.PasswordState{
		HasPassword: true,
		Secret:      []byte("password-verifier"),
	})
	withResetPasswordStore(t, store)
	c := newResetPasswordCore(userID)

	first, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if first.GetPredicateName() != mtproto.Predicate_account_resetPasswordRequestedWait {
		t.Fatalf("first result predicate = %q, want requestedWait", first.GetPredicateName())
	}
	if first.GetUntilDate() == 0 {
		t.Fatal("first result has no wait deadline")
	}

	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(store.values[acctPasswordKey(userID)]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResetRequestedAt == 0 || saved.ResetUntilDate != int64(first.GetUntilDate()) {
		t.Fatalf("persisted request state = %+v, result deadline = %d", saved, first.GetUntilDate())
	}

	second, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if second.GetUntilDate() != first.GetUntilDate() {
		t.Fatalf("repeated request deadline = %d, want original %d", second.GetUntilDate(), first.GetUntilDate())
	}
	if store.writes != 1 {
		t.Fatalf("repeated request rewrote the persisted deadline %d times", store.writes)
	}
}

func TestAccountResetPasswordKeepsOriginalDeclinedRetryDate(t *testing.T) {
	const userID = int64(712347)
	store := &memoryPasswordResetStore{values: make(map[string]string)}
	setResetPasswordState(t, store, userID, twofa.PasswordState{
		HasPassword:   true,
		ResetDeclined: true,
	})
	withResetPasswordStore(t, store)
	c := newResetPasswordCore(userID)

	first, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if first.GetPredicateName() != mtproto.Predicate_account_resetPasswordFailedWait {
		t.Fatalf("first result predicate = %q, want failedWait", first.GetPredicateName())
	}
	if first.GetRetryDate() == 0 {
		t.Fatal("first result has no retry date")
	}

	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(store.values[acctPasswordKey(userID)]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResetRetryDate != int64(first.GetRetryDate()) {
		t.Fatalf("persisted retry date = %d, result retry date = %d", saved.ResetRetryDate, first.GetRetryDate())
	}

	second, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if second.GetRetryDate() != first.GetRetryDate() {
		t.Fatalf("repeated request retry date = %d, want original %d", second.GetRetryDate(), first.GetRetryDate())
	}
	if store.writes != 1 {
		t.Fatalf("repeated request rewrote the persisted retry date %d times", store.writes)
	}
}

func TestAccountResetPasswordCompletesAfterRequestedWait(t *testing.T) {
	const userID = int64(712351)
	store := &memoryPasswordResetStore{values: make(map[string]string)}
	setResetPasswordState(t, store, userID, twofa.PasswordState{
		HasPassword:      true,
		Secret:           []byte("password-verifier"),
		ResetRequestedAt: time.Now().Unix() - passwordResetWait - 1,
		ResetUntilDate:   time.Now().Unix() - 1,
	})
	withResetPasswordStore(t, store)
	c := newResetPasswordCore(userID)

	result, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetPredicateName() != mtproto.Predicate_account_resetPasswordOk {
		t.Fatalf("result predicate = %q, want resetPasswordOk", result.GetPredicateName())
	}
	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(store.values[acctPasswordKey(userID)]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.HasPassword || len(saved.Secret) != 0 || saved.ResetUntilDate != 0 || saved.ResetRequestedAt != 0 {
		t.Fatalf("completed reset retained password state: %+v", saved)
	}
}

func TestAccountResetPasswordRetriesAfterDeclinedWait(t *testing.T) {
	const userID = int64(712352)
	store := &memoryPasswordResetStore{values: make(map[string]string)}
	setResetPasswordState(t, store, userID, twofa.PasswordState{
		HasPassword:    true,
		Secret:         []byte("password-verifier"),
		ResetDeclined:  true,
		ResetRetryDate: time.Now().Unix() - 1,
	})
	withResetPasswordStore(t, store)
	c := newResetPasswordCore(userID)

	result, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetPredicateName() != mtproto.Predicate_account_resetPasswordRequestedWait {
		t.Fatalf("result predicate = %q, want requestedWait", result.GetPredicateName())
	}
	if int64(result.GetUntilDate()) <= time.Now().Unix() {
		t.Fatalf("new reset deadline = %d, want future deadline", result.GetUntilDate())
	}
	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(store.values[acctPasswordKey(userID)]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResetDeclined || saved.ResetRetryDate != 0 || saved.ResetUntilDate != int64(result.GetUntilDate()) {
		t.Fatalf("retry state = %+v", saved)
	}
}

func TestAccountResetPasswordPostgresCompletesAndPersists(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	const userID = int64(712353)
	key := acctPasswordKey(userID)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key)
		_ = db.Close()
	})
	state, err := json.Marshal(twofa.PasswordState{
		HasPassword:      true,
		Secret:           []byte("postgres-password-verifier"),
		ResetRequestedAt: time.Now().Unix() - passwordResetWait - 1,
		ResetUntilDate:   time.Now().Unix() - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)
		ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, key, string(state)); err != nil {
		t.Fatal(err)
	}
	c := newResetPasswordCore(userID)
	result, err := c.AccountResetPassword(&mtproto.TLAccountResetPassword{})
	if err != nil {
		t.Fatal(err)
	}
	if result.GetPredicateName() != mtproto.Predicate_account_resetPasswordOk {
		t.Fatalf("result predicate = %q, want resetPasswordOk", result.GetPredicateName())
	}
	var raw string
	if err = db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, key).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(raw), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.HasPassword || len(saved.Secret) != 0 || saved.ResetUntilDate != 0 {
		t.Fatalf("PostgreSQL state retained password after reset: %+v", saved)
	}
}

func TestAccountResetPasswordPostgresConcurrentFirstRequest(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	const userID = int64(712354)
	key := acctPasswordKey(userID)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key)
		_ = db.Close()
	})
	state, err := json.Marshal(twofa.PasswordState{
		HasPassword: true,
		Secret:      []byte("concurrent-password-verifier"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)
		ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`, key, string(state)); err != nil {
		t.Fatal(err)
	}
	const callers = 8
	results := make(chan *mtproto.Account_ResetPasswordResult, callers)
	errors := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, callErr := newResetPasswordCore(userID).AccountResetPassword(&mtproto.TLAccountResetPassword{})
			if callErr != nil {
				errors <- callErr
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for callErr := range errors {
		t.Fatal(callErr)
	}
	var until int32
	count := 0
	for result := range results {
		count++
		if result.GetPredicateName() != mtproto.Predicate_account_resetPasswordRequestedWait {
			t.Fatalf("concurrent result predicate = %q, want requestedWait", result.GetPredicateName())
		}
		if until == 0 {
			until = result.GetUntilDate()
		} else if result.GetUntilDate() != until {
			t.Fatalf("concurrent reset deadlines differ: first=%d current=%d", until, result.GetUntilDate())
		}
	}
	if count != callers || until <= 0 {
		t.Fatalf("concurrent reset results = %d/%d, until=%d", count, callers, until)
	}
	var raw string
	if err = db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, key).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var saved twofa.PasswordState
	if err = json.Unmarshal([]byte(raw), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ResetUntilDate != int64(until) || saved.ResetRequestedAt == 0 {
		t.Fatalf("persisted concurrent reset state = %+v, result deadline=%d", saved, until)
	}
}

func setResetPasswordState(t *testing.T, store *memoryPasswordResetStore, userID int64, state twofa.PasswordState) {
	t.Helper()
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	store.values[acctPasswordKey(userID)] = string(encoded)
}

func withResetPasswordStore(t *testing.T, store *memoryPasswordResetStore) {
	t.Helper()
	oldStore := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = oldStore })
}

func newResetPasswordCore(userID int64) *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}
