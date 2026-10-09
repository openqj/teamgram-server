package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDialogMarkSavedHistoryReadPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("DIALOG_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DIALOG_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}
	pg, err := dao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)

	ctx := context.Background()
	var version int
	if err := pg.Pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL version is %d, want 18.x", version)
	}

	userID := time.Now().UnixNano()
	peerID := userID + 1
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO saved_dialogs (user_id, peer_type, peer_id, read_max_id) VALUES ($1, $2, $3, 0)`, userID, mtproto.PEER_CHAT, peerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM saved_dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, userID, mtproto.PEER_CHAT, peerID)
	})

	c := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	request := func(maxID int32) *dialog.TLDialogInsertOrUpdateDialog {
		return &dialog.TLDialogInsertOrUpdateDialog{UserId: userID, PeerType: mtproto.PEER_CHAT, PeerId: peerID, ReadInboxMaxId: wrapperspb.Int32(maxID)}
	}
	for _, maxID := range []int32{450, 420} {
		result, err := c.DialogMarkSavedHistoryRead(request(maxID))
		if err != nil || !mtproto.FromBool(result) {
			t.Fatalf("mark saved history %d = (%v, %v), want true", maxID, result, err)
		}
	}
	var readMaxID int32
	if err := pg.Pool.QueryRow(ctx, `SELECT read_max_id FROM saved_dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, userID, mtproto.PEER_CHAT, peerID).Scan(&readMaxID); err != nil {
		t.Fatal(err)
	}
	if readMaxID != 450 {
		t.Fatalf("saved history cursor = %d, want monotonic 450", readMaxID)
	}

	if result, err := c.DialogMarkSavedHistoryRead(request(451)); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("advance saved history = (%v, %v), want true", result, err)
	}
	if err := pg.Pool.QueryRow(ctx, `SELECT read_max_id FROM saved_dialogs WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3`, userID, mtproto.PEER_CHAT, peerID).Scan(&readMaxID); err != nil {
		t.Fatal(err)
	}
	if readMaxID != 451 {
		t.Fatalf("advanced saved history cursor = %d, want 451", readMaxID)
	}
}
