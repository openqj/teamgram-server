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

func TestDialogToggleSavedDialogPinRejectsInvalidRequest(t *testing.T) {
	validPeer := mtproto.MakePeerUtil(mtproto.PEER_USER, 2)
	cases := []struct {
		name string
		in   *dialog.TLDialogToggleSavedDialogPin
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogToggleSavedDialogPin{Peer: validPeer, Pinned: mtproto.BoolTrue}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogToggleSavedDialogPin{UserId: 1, Pinned: mtproto.BoolTrue}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing pinned", in: &dialog.TLDialogToggleSavedDialogPin{UserId: 1, Peer: validPeer}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer id", in: &dialog.TLDialogToggleSavedDialogPin{UserId: 1, Peer: mtproto.MakePeerUtil(mtproto.PEER_USER, 0), Pinned: mtproto.BoolTrue}, want: mtproto.ErrPeerIdInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogToggleSavedDialogPin{UserId: 1, Peer: mtproto.MakePeerUtil(99, 2), Pinned: mtproto.BoolTrue}, want: mtproto.ErrPeerIdInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogToggleSavedDialogPin(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogToggleSavedDialogPin() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogToggleSavedDialogPinRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogToggleSavedDialogPin{
		UserId: 1,
		Peer:   mtproto.MakePeerUtil(mtproto.PEER_USER, 2),
		Pinned: mtproto.BoolTrue,
	}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogToggleSavedDialogPin(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogToggleSavedDialogPin() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}

func TestDialogToggleSavedDialogPinPostgresRoundTrip(t *testing.T) {
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
	deletedPeerID := userID + 2
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO saved_dialogs (user_id, peer_type, peer_id, pinned, deleted) VALUES ($1, $2, $3, 0, FALSE), ($1, $2, $4, 123, TRUE)`, userID, mtproto.PEER_USER, peerID, deletedPeerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM saved_dialogs WHERE user_id = $1`, userID)
	})

	c := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	request := func(id int64, pinned *mtproto.Bool) *dialog.TLDialogToggleSavedDialogPin {
		return &dialog.TLDialogToggleSavedDialogPin{UserId: userID, Peer: mtproto.MakePeerUtil(mtproto.PEER_USER, id), Pinned: pinned}
	}
	if result, err := c.DialogToggleSavedDialogPin(request(peerID, mtproto.BoolTrue)); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("pin existing saved dialog = (%v, %v), want true", result, err)
	}
	var pinned int64
	if err := pg.Pool.QueryRow(ctx, `SELECT pinned FROM saved_dialogs WHERE user_id = $1 AND peer_id = $2`, userID, peerID).Scan(&pinned); err != nil {
		t.Fatal(err)
	}
	if pinned <= 0 {
		t.Fatalf("pinned value = %d, want positive timestamp", pinned)
	}

	if result, err := c.DialogToggleSavedDialogPin(request(userID+3, mtproto.BoolTrue)); err != nil || mtproto.FromBool(result) {
		t.Fatalf("pin missing saved dialog = (%v, %v), want false,nil", result, err)
	}
	if result, err := c.DialogToggleSavedDialogPin(request(deletedPeerID, mtproto.BoolFalse)); err != nil || mtproto.FromBool(result) {
		t.Fatalf("pin deleted saved dialog = (%v, %v), want false,nil", result, err)
	}
}
