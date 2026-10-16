package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	authsessionclient "github.com/teamgram/teamgram-server/app/service/authsession/client"
	"github.com/zeromicro/go-zero/core/logx"
)

type changeAuthorizationSettingsAuthsessionClient struct {
	authsessionclient.AuthsessionClient
	authorizations *mtproto.Account_Authorizations
	err            error
	request        *authsession.TLAuthsessionGetAuthorizations
}

func (s *changeAuthorizationSettingsAuthsessionClient) AuthsessionGetAuthorizations(_ context.Context, in *authsession.TLAuthsessionGetAuthorizations) (*mtproto.Account_Authorizations, error) {
	s.request = in
	return s.authorizations, s.err
}

func newChangeAuthorizationSettingsCore(client authsessionclient.AuthsessionClient) *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 1001, UserId: 7},
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{AuthsessionClient: client}},
	}
}

func TestAccountChangeAuthorizationSettingsRejectsUnknownAuthorizationHash(t *testing.T) {
	client := &changeAuthorizationSettingsAuthsessionClient{
		authorizations: &mtproto.Account_Authorizations{
			Authorizations: []*mtproto.Authorization{{Hash: 2001}},
		},
	}
	c := newChangeAuthorizationSettingsCore(client)

	result, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{
		Hash:                      2002,
		EncryptedRequestsDisabled: mtproto.BoolTrue,
	})
	if result != nil || !errors.Is(err, mtproto.ErrHashInvalid) {
		t.Fatalf("unknown authorization hash = (%v, %v), want (nil, HASH_INVALID)", result, err)
	}
	if client.request == nil || client.request.GetUserId() != 7 || client.request.GetExcludeAuthKeyId() != 1001 {
		t.Fatalf("authorization lookup request = %v, want user 7 and current key 1001", client.request)
	}
}

func TestAccountChangeAuthorizationSettingsRequiresProviderForTargetHash(t *testing.T) {
	c := newChangeAuthorizationSettingsCore(nil)

	result, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{Hash: 2002})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing authorization provider = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
}

func TestAccountChangeAuthorizationSettingsRequiresUserForTargetHash(t *testing.T) {
	client := &changeAuthorizationSettingsAuthsessionClient{}
	c := newChangeAuthorizationSettingsCore(client)
	c.MD.UserId = 0

	result, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{Hash: 2002})
	if result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("target hash without user = (%v, %v), want (nil, AUTH_KEY_UNREGISTERED)", result, err)
	}
	if client.request != nil {
		t.Fatal("authorization provider called without an authenticated user")
	}
}

func TestAccountChangeAuthorizationSettingsPostgresOwnedHashRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := sharedpersist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const (
		authKeyID  int64 = 870003
		targetHash int64 = 990003
	)
	key := authorizationSettingsStoreKey(targetHash)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key) })
	if _, err := db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key); err != nil {
		t.Fatal(err)
	}
	client := &changeAuthorizationSettingsAuthsessionClient{
		authorizations: &mtproto.Account_Authorizations{
			Authorizations: []*mtproto.Authorization{{Hash: targetHash}},
		},
	}
	c := newChangeAuthorizationSettingsCore(client)
	c.MD.PermAuthKeyId = authKeyID

	if got, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{
		Hash:                      targetHash,
		Confirmed:                 true,
		EncryptedRequestsDisabled: mtproto.BoolTrue,
	}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("initial target settings = (%v, %v), want BoolTrue", got, err)
	}
	if got, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{
		Hash:                 targetHash,
		CallRequestsDisabled: mtproto.BoolTrue,
	}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("partial target settings = (%v, %v), want BoolTrue", got, err)
	}
	var raw string
	if err := db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, key).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var flags authorizationSettingsFlags
	if err := json.Unmarshal([]byte(raw), &flags); err != nil {
		t.Fatal(err)
	}
	if !flags.Confirmed || flags.EncryptedRequestsDisabled == nil || !*flags.EncryptedRequestsDisabled || flags.CallRequestsDisabled == nil || !*flags.CallRequestsDisabled {
		t.Fatalf("stored target authorization settings = %+v", flags)
	}
}
