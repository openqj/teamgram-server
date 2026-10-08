package dao

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/config"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestPostgresRuntimeSmoke(t *testing.T) {
	if os.Getenv("AUTHSESSION_POSTGRES_SMOKE") != "1" {
		t.Skip("set AUTHSESSION_POSTGRES_SMOKE=1 to run the PostgreSQL runtime probe")
	}
	dsn := os.Getenv("AUTHSESSION_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://teamgram:teamgram@127.0.0.1:5432/teamgram?sslmode=disable"
	}
	d, err := NewPostgres(config.Config{Postgres: postgres.Config{DSN: dsn}, Cache: cache.CacheConf{}, KV: kv.KvConf{{RedisConf: redis.RedisConf{Host: "127.0.0.1:6379", Type: redis.NodeType}, Weight: 100}}})
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer d.Close()
	ctx := context.Background()
	const keyID int64 = 918273645001
	const userID int64 = 918273645002
	const tempID int64 = 918273645003
	const secondaryKeyID int64 = 918273645004
	// The salt cache is process independent; clear the probe key so a prior
	// run cannot add a previous salt to the requested window.
	_, _ = d.kv.DelCtx(ctx, fmt.Sprintf("%s_%d", pgSaltPrefix, keyID))
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_users WHERE auth_key_id=$1`, keyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auths WHERE auth_key_id=$1`, keyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_key_infos WHERE auth_key_id=$1`, keyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_keys WHERE auth_key_id=$1`, keyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_key_infos WHERE auth_key_id=$1`, tempID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_keys WHERE auth_key_id=$1`, tempID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_users WHERE auth_key_id=$1`, secondaryKeyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auths WHERE auth_key_id=$1`, secondaryKeyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_key_infos WHERE auth_key_id=$1`, secondaryKeyID)
	_, _ = d.pool.Exec(ctx, `DELETE FROM auth_keys WHERE auth_key_id=$1`, secondaryKeyID)
	defer func() {
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_users WHERE auth_key_id=$1`, keyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auths WHERE auth_key_id=$1`, keyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_key_infos WHERE auth_key_id=$1`, keyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_keys WHERE auth_key_id=$1`, keyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_key_infos WHERE auth_key_id=$1`, tempID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_keys WHERE auth_key_id=$1`, tempID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_users WHERE auth_key_id=$1`, secondaryKeyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auths WHERE auth_key_id=$1`, secondaryKeyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_key_infos WHERE auth_key_id=$1`, secondaryKeyID)
		_, _ = d.pool.Exec(ctx, `DELETE FROM auth_keys WHERE auth_key_id=$1`, secondaryKeyID)
	}()
	key := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{AuthKeyId: keyID, AuthKey: []byte("key"), AuthKeyType: mtproto.AuthKeyTypePerm, PermAuthKeyId: keyID}).To_AuthKeyInfo()
	if err := d.SetAuthKeyV2(ctx, key, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.SetClientSessionInfo(ctx, &authsession.ClientSession{AuthKeyId: keyID, Layer: 229, ApiId: 1, DeviceModel: "smoke", Ip: "127.0.0.1", Params: `{"x":1}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.QueryAuthKeyV2(ctx, keyID); err != nil {
		t.Fatal(err)
	}
	if salts, err := d.GetFutureSalts(ctx, keyID, 2); err != nil || salts == nil || len(salts.GetSalts()) != 2 {
		t.Fatalf("future salts=%v err=%v", salts, err)
	}
	temp := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{AuthKeyId: tempID, AuthKey: []byte("temp"), AuthKeyType: mtproto.AuthKeyTypeTemp}).To_AuthKeyInfo()
	if err := d.SetAuthKeyV2(ctx, temp, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.BindTempAuthKeyV2(ctx, keyID, tempID, mtproto.AuthKeyTypeTemp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.BindAuthKeyUser(ctx, keyID, userID); err != nil {
		t.Fatal(err)
	}
	if got := d.GetAuthKeyUserId(ctx, keyID); got != userID {
		t.Fatalf("user id = %d", got)
	}
	if _, err := d.GetAuthorization(ctx, keyID); err != nil {
		t.Fatal(err)
	}
	keys, err := d.ResetAuthorization(ctx, userID, keyID, 0)
	if err != nil || len(keys) != 0 {
		t.Fatalf("reset keys=%v err=%v", keys, err)
	}
	secondary := mtproto.MakeTLAuthKeyInfo(&mtproto.AuthKeyInfo{AuthKeyId: secondaryKeyID, AuthKey: []byte("secondary"), AuthKeyType: mtproto.AuthKeyTypePerm, PermAuthKeyId: secondaryKeyID}).To_AuthKeyInfo()
	if err := d.SetAuthKeyV2(ctx, secondary, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.SetClientSessionInfo(ctx, &authsession.ClientSession{AuthKeyId: secondaryKeyID, Layer: 229, ApiId: 1, DeviceModel: "secondary", Ip: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.BindAuthKeyUser(ctx, secondaryKeyID, userID); err != nil {
		t.Fatal(err)
	}
	keys, err = d.ResetAuthorization(ctx, userID, keyID, 0)
	if err != nil || len(keys) != 1 || keys[0] != secondaryKeyID {
		t.Fatalf("reset secondary keys=%v err=%v", keys, err)
	}
}
