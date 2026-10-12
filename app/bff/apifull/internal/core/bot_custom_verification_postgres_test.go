package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBotsSetCustomVerificationFailsClosedWithoutVerifierAuthority(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open PostgreSQL schema: %v", err)
	}
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatalf("open cleanup connection: %v", err)
	}
	defer cleanupDB.Close()

	botID := time.Now().UnixNano()
	accessHash := botID + 1
	key := customVerificationKey(botID, mtproto.PEER_SELF, botID)
	defer func() { _, _ = cleanupDB.Exec(`DELETE FROM apifull_kv WHERE k=$1`, key) }()

	client := &miniBotAppsUserClient{
		users: map[int64]*mtproto.ImmutableUser{
			botID: miniBotProfile(botID, accessHash, true),
		},
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: botID},
	}
	request := &mtproto.TLBotsSetCustomVerification{
		Enabled:           true,
		Peer:              mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		CustomDescription: wrapperspb.String("trusted support"),
	}
	if result, err := c.BotsSetCustomVerification(request); result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("set custom verification without verifier authority = (%v, %v), want METHOD_NOT_IMPL", result, err)
	}
	var count int
	if err := cleanupDB.QueryRow(`SELECT COUNT(*) FROM apifull_kv WHERE k=$1`, key).Scan(&count); err != nil {
		t.Fatalf("check rejected verification persistence: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected verification wrote %d rows", count)
	}
}
