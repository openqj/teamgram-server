package dao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func testPasskeyDAO(t *testing.T) (*Dao, string) {
	t.Helper()
	dsn := os.Getenv("TEAMGRAM_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	db, err := openPostgres(dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	prefix := fmt.Sprintf("dao-test-%d-", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = db.Exec(ctx, `DELETE FROM apifull_passkey_session WHERE challenge LIKE $1`, prefix+"%")
		_, _ = db.Exec(ctx, `DELETE FROM apifull_passkey_credential WHERE encode(credential_id, 'hex') LIKE encode($1::bytea, 'hex') || '%'`, []byte(prefix))
		db.Close()
	})
	return &Dao{DB: db}, prefix
}

func TestPostgresSessionLifecycleIsOneTimeAndPrunesExpired(t *testing.T) {
	dao, prefix := testPasskeyDAO(t)
	ctx := context.Background()
	challenge := prefix + "session"
	session := &Session{
		Challenge: challenge,
		UserID:    17,
		Kind:      SessionRegistration,
		Data:      []byte(`{"challenge":"session"}`),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	if err := dao.PutSession(ctx, session); err != nil {
		t.Fatalf("PutSession() error = %v", err)
	}
	got, err := dao.GetSession(ctx, challenge, SessionRegistration)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.UserID != session.UserID || string(got.Data) != string(session.Data) {
		t.Fatalf("GetSession() = %+v, want user/data from PutSession", got)
	}
	if err := dao.PutSession(ctx, session); err == nil {
		t.Fatal("PutSession() duplicate challenge unexpectedly succeeded")
	}
	if err := dao.ConsumeSession(ctx, challenge, SessionRegistration); err != nil {
		t.Fatalf("ConsumeSession() error = %v", err)
	}
	if err := dao.ConsumeSession(ctx, challenge, SessionRegistration); !errors.Is(err, ErrSessionUsed) {
		t.Fatalf("second ConsumeSession() error = %v, want ErrSessionUsed", err)
	}
	if _, err := dao.GetSession(ctx, challenge, SessionRegistration); !errors.Is(err, ErrSessionUsed) {
		t.Fatalf("GetSession() after consume error = %v, want ErrSessionUsed", err)
	}

	expired := prefix + "expired"
	if _, err := dao.DB.Exec(ctx, `
		INSERT INTO apifull_passkey_session (challenge, user_id, kind, data, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, $5, FALSE, $6)
	`, expired, 17, SessionRegistration, []byte("expired"), time.Now().Unix(), time.Now().Unix()); err != nil {
		t.Fatalf("insert expired session fixture: %v", err)
	}
	// Creating another challenge prunes the expired row in the same transaction.
	if err := dao.PutSession(ctx, &Session{
		Challenge: prefix + "fresh",
		UserID:    17,
		Kind:      SessionRegistration,
		Data:      []byte("fresh"),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("PutSession(fresh) error = %v", err)
	}
	if _, err := dao.GetSession(ctx, expired, SessionRegistration); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expired session after prune error = %v, want ErrSessionNotFound", err)
	}
}

func TestPutSessionRejectsExpiredOrUnknownCeremony(t *testing.T) {
	dao, _ := testPasskeyDAO(t)
	ctx := context.Background()
	for _, session := range []*Session{
		{Challenge: "expired", UserID: 1, Kind: SessionLogin, Data: []byte("data"), ExpiresAt: time.Now().Unix()},
		{Challenge: "unknown-kind", UserID: 1, Kind: "other", Data: []byte("data"), ExpiresAt: time.Now().Add(time.Minute).Unix()},
		{Challenge: "negative-user", UserID: -1, Kind: SessionLogin, Data: []byte("data"), ExpiresAt: time.Now().Add(time.Minute).Unix()},
	} {
		if err := dao.PutSession(ctx, session); err == nil {
			t.Fatalf("PutSession(%q) unexpectedly succeeded", session.Challenge)
		}
	}
}

func TestPostgresCredentialMutationIsAtomicAndOwnershipScoped(t *testing.T) {
	dao, prefix := testPasskeyDAO(t)
	ctx := context.Background()
	credentialID := []byte(prefix + "credential")
	registration := prefix + "registration"
	if err := dao.PutSession(ctx, &Session{
		Challenge: registration,
		UserID:    23,
		Kind:      SessionRegistration,
		Data:      []byte("registration"),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("PutSession(registration) error = %v", err)
	}
	credential := &Credential{
		ID:            credentialID,
		UserID:        23,
		Name:          "Passkey",
		Date:          time.Now().Unix(),
		LastUsageDate: 0,
		Data:          []byte("credential-data"),
	}
	if err := dao.ConsumeAndInsertCredential(ctx, registration, SessionRegistration, credential); err != nil {
		t.Fatalf("ConsumeAndInsertCredential() error = %v", err)
	}
	if err := dao.ConsumeAndInsertCredential(ctx, registration, SessionRegistration, credential); !errors.Is(err, ErrSessionUsed) {
		t.Fatalf("replayed ConsumeAndInsertCredential() error = %v, want ErrSessionUsed", err)
	}
	if got, err := dao.GetCredential(ctx, credentialID); err != nil || got.UserID != credential.UserID {
		t.Fatalf("GetCredential() = (%+v, %v), want persisted credential", got, err)
	}
	if deleted, err := dao.DeleteCredential(ctx, 999, credentialID); err != nil || deleted {
		t.Fatalf("DeleteCredential(wrong owner) = (%v, %v), want (false, nil)", deleted, err)
	}
	if deleted, err := dao.DeleteCredential(ctx, credential.UserID, credentialID); err != nil || !deleted {
		t.Fatalf("DeleteCredential(owner) = (%v, %v), want (true, nil)", deleted, err)
	}
	if _, err := dao.GetCredential(ctx, credentialID); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("GetCredential() after delete error = %v, want ErrCredentialNotFound", err)
	}
	// A deleted credential may be registered again by the same owner. An
	// active credential ID remains a conflict and must leave its challenge
	// usable because the transaction rolls back.
	reRegister := prefix + "reregister"
	if err := dao.PutSession(ctx, &Session{
		Challenge: reRegister,
		UserID:    credential.UserID,
		Kind:      SessionRegistration,
		Data:      []byte("reregister"),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("PutSession(reregister) error = %v", err)
	}
	if err := dao.ConsumeAndInsertCredential(ctx, reRegister, SessionRegistration, credential); err != nil {
		t.Fatalf("ConsumeAndInsertCredential(reregister) error = %v", err)
	}
	activeConflict := prefix + "active-conflict"
	if err := dao.PutSession(ctx, &Session{
		Challenge: activeConflict,
		UserID:    credential.UserID,
		Kind:      SessionRegistration,
		Data:      []byte("active-conflict"),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}); err != nil {
		t.Fatalf("PutSession(active conflict) error = %v", err)
	}
	if err := dao.ConsumeAndInsertCredential(ctx, activeConflict, SessionRegistration, credential); !errors.Is(err, ErrCredentialConflict) {
		t.Fatalf("ConsumeAndInsertCredential(active conflict) error = %v, want ErrCredentialConflict", err)
	}
	if _, err := dao.GetSession(ctx, activeConflict, SessionRegistration); err != nil {
		t.Fatalf("active conflict challenge was consumed after rollback: %v", err)
	}
}

func TestPostgresLoginCounterAndChallengeAreAtomic(t *testing.T) {
	dao, prefix := testPasskeyDAO(t)
	ctx := context.Background()
	credentialID := []byte(prefix + "credential")
	if err := dao.InsertCredential(ctx, &Credential{
		ID:     credentialID,
		UserID: 31,
		Name:   "Passkey",
		Date:   time.Now().Unix(),
		Data:   []byte("credential-data"),
	}); err != nil {
		t.Fatalf("InsertCredential() error = %v", err)
	}
	first := prefix + "login-1"
	second := prefix + "login-2"
	for _, challenge := range []string{first, second} {
		if err := dao.PutSession(ctx, &Session{
			Challenge: challenge,
			Kind:      SessionLogin,
			Data:      []byte("login"),
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		}); err != nil {
			t.Fatalf("PutSession(%s) error = %v", challenge, err)
		}
	}
	if err := dao.ConsumeLoginAndUpdateCredential(ctx, first, SessionLogin, credentialID, 0, 1, []byte("updated"), time.Now().Unix()); err != nil {
		t.Fatalf("ConsumeLoginAndUpdateCredential(first) error = %v", err)
	}
	if err := dao.ConsumeLoginAndUpdateCredential(ctx, second, SessionLogin, credentialID, 0, 1, []byte("replay"), time.Now().Unix()); !errors.Is(err, ErrCounterReplay) {
		t.Fatalf("ConsumeLoginAndUpdateCredential(counter replay) error = %v, want ErrCounterReplay", err)
	}
	if err := dao.ConsumeLoginAndUpdateCredential(ctx, second, SessionLogin, credentialID, 1, 2, []byte("updated-again"), time.Now().Unix()); err != nil {
		t.Fatalf("ConsumeLoginAndUpdateCredential(second) error = %v", err)
	}
	if _, err := dao.GetSession(ctx, second, SessionLogin); !errors.Is(err, ErrSessionUsed) {
		t.Fatalf("second login session after success error = %v, want ErrSessionUsed", err)
	}
}
