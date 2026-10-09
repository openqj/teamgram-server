package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	sharedpersist "github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestAccountChangeAuthorizationSettingsPostgresRoundTrip(t *testing.T) {
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
	const authKeyID int64 = 870001
	key := authorizationSettingsStoreKey(authKeyID)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key) })
	_, _ = db.Exec(`DELETE FROM apifull_kv WHERE k = $1`, key)

	c := &AuthorizationCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: authKeyID, SessionId: 17},
		svcCtx: &svc.ServiceContext{},
		Logger: logx.WithContext(context.Background()),
	}
	if got, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{
		Confirmed:                 true,
		EncryptedRequestsDisabled: mtproto.BoolTrue,
	}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("initial settings = (%v, %v), want BoolTrue", got, err)
	}
	if got, err := c.AccountChangeAuthorizationSettings(&mtproto.TLAccountChangeAuthorizationSettings{
		CallRequestsDisabled: mtproto.BoolTrue,
	}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("partial settings = (%v, %v), want BoolTrue", got, err)
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
		t.Fatalf("stored authorization settings = %+v", flags)
	}
}
