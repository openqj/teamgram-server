package core

import (
	"context"
	"encoding/json"
	"testing"

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
