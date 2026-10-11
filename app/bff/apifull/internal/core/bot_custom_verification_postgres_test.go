package core

import (
	"context"
	"encoding/json"
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

func TestBotsSetCustomVerificationPostgresRoundTrip(t *testing.T) {
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
		Bot:               miniBotInput(botID, accessHash),
		Peer:              mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		CustomDescription: wrapperspb.String("trusted support"),
	}
	if result, err := c.BotsSetCustomVerification(request); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("set custom verification = (%v, %v)", result, err)
	}
	var raw string
	if err := cleanupDB.QueryRow(`SELECT v FROM apifull_kv WHERE k=$1`, key).Scan(&raw); err != nil {
		t.Fatalf("read persisted verification: %v", err)
	}
	var state botCustomVerificationState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("decode persisted verification: %v", err)
	}
	if !state.Enabled || state.Description != "trusted support" || state.UpdatedAt <= 0 {
		t.Fatalf("persisted verification = %+v", state)
	}

	request.Enabled = false
	request.CustomDescription = wrapperspb.String("")
	if result, err := c.BotsSetCustomVerification(request); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("clear custom verification = (%v, %v)", result, err)
	}
	if err := cleanupDB.QueryRow(`SELECT v FROM apifull_kv WHERE k=$1`, key).Scan(&raw); err != nil {
		t.Fatalf("read cleared verification: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil || state.Enabled || state.Description != "" {
		t.Fatalf("cleared verification = %+v (decode=%v)", state, err)
	}
}
