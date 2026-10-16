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

func TestDialogGetTopMessageRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogGetTopMessage
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogGetTopMessage{PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogGetTopMessage{UserId: 1, PeerType: mtproto.PEER_USER}, want: mtproto.ErrInputRequestInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogGetTopMessage{UserId: 1, PeerType: 99, PeerId: 2}, want: mtproto.ErrPeerIdInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogGetTopMessage(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogGetTopMessage() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogGetTopMessageRequiresPostgresStore(t *testing.T) {
	got, err := (&DialogCore{}).DialogGetTopMessage(&dialog.TLDialogGetTopMessage{
		UserId:   1,
		PeerType: mtproto.PEER_USER,
		PeerId:   2,
	})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("DialogGetTopMessage() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestDialogGetTopMessagePostgresRoundTrip(t *testing.T) {
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
	const peerType int32 = mtproto.PEER_USER
	peerDialogID := mtproto.MakePeerDialogId(peerType, peerID)
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, top_message) VALUES ($1, $2, $3, $4, $5)`, userID, peerType, peerID, peerDialogID, 73); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM dialogs WHERE user_id = $1`, userID)
	})

	c := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	request := &dialog.TLDialogGetTopMessage{UserId: userID, PeerType: peerType, PeerId: peerID}
	got, err := c.DialogGetTopMessage(request)
	if err != nil || got == nil || got.GetV() != 73 {
		t.Fatalf("stored top message = (%v, %v), want 73", got, err)
	}

	missing, err := c.DialogGetTopMessage(&dialog.TLDialogGetTopMessage{UserId: userID, PeerType: peerType, PeerId: peerID + 1})
	if err != nil || missing == nil || missing.GetV() != 0 {
		t.Fatalf("missing dialog top message = (%v, %v), want 0 without error", missing, err)
	}

	errorCtx, cancel := context.WithCancel(ctx)
	cancel()
	failed, err := (&DialogCore{ctx: errorCtx, svcCtx: c.svcCtx}).DialogGetTopMessage(request)
	if failed != nil || err == nil {
		t.Fatalf("canceled PostgreSQL lookup = (%v, %v), want propagated error", failed, err)
	}
}
