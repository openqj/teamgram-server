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
)

func TestDialogReorderPinnedSavedDialogsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("DIALOG_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DIALOG_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}
	pg, err := dao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	t.Cleanup(pg.Close)

	ctx := context.Background()
	var version int
	if err := pg.Pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL version is %d, want 18.x", version)
	}

	userID := time.Now().UnixNano()
	peerIDs := []int64{userID + 1, userID + 2, userID + 3}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO saved_dialogs(user_id, peer_type, peer_id, pinned) VALUES
($1, 2, $2, 11), ($1, 2, $3, 22), ($1, 2, $4, 0)`, userID, peerIDs[0], peerIDs[1], peerIDs[2]); err != nil {
		t.Fatalf("insert saved-dialog fixtures: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM saved_dialogs WHERE user_id = $1`, userID)
	})

	core := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	request := &dialog.TLDialogReorderPinnedSavedDialogs{
		UserId: userID,
		Force:  mtproto.BoolTrue,
		Order: []*mtproto.PeerUtil{
			{PeerType: 2, PeerId: peerIDs[1]},
			{PeerType: 2, PeerId: peerIDs[0]},
		},
	}
	if result, err := core.DialogReorderPinnedSavedDialogs(request); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("force reorder = (%v, %v), want true", result, err)
	}
	rows, err := pg.Store.SavedDialogs.SelectPinnedDialogs(ctx, userID)
	if err != nil || len(rows) != 2 || rows[0].PeerId != peerIDs[1] || rows[1].PeerId != peerIDs[0] || rows[0].Pinned <= rows[1].Pinned {
		t.Fatalf("pinned order after force reorder = (%+v, %v), want requested order", rows, err)
	}
	third, err := pg.Store.SavedDialogs.Select(ctx, userID, 2, peerIDs[2])
	if err != nil || third == nil || third.Pinned != 0 {
		t.Fatalf("unlisted dialog after force reorder = (%+v, %v), want unpinned", third, err)
	}

	request.Force = mtproto.BoolFalse
	request.Order = []*mtproto.PeerUtil{{PeerType: 2, PeerId: peerIDs[2]}}
	if result, err := core.DialogReorderPinnedSavedDialogs(request); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("non-force reorder = (%v, %v), want true", result, err)
	}
	rows, err = pg.Store.SavedDialogs.SelectPinnedDialogs(ctx, userID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("pinned rows after non-force reorder = (%+v, %v), want all three", rows, err)
	}
}
