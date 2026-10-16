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
	"github.com/zeromicro/go-zero/core/jsonx"
)

func TestDialogGetAllDraftsRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogGetAllDrafts
	}{
		{name: "nil request"},
		{name: "missing user", in: &dialog.TLDialogGetAllDrafts{}},
		{name: "negative user", in: &dialog.TLDialogGetAllDrafts{UserId: -1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogGetAllDrafts(tc.in)
			if got != nil || err != mtproto.ErrInputRequestInvalid {
				t.Fatalf("DialogGetAllDrafts() = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
			}
		})
	}
}

func TestDialogGetAllDraftsRequiresPostgresStore(t *testing.T) {
	got, err := (&DialogCore{}).DialogGetAllDrafts(&dialog.TLDialogGetAllDrafts{UserId: 1})
	if got != nil || err != mtproto.ErrMethodNotImpl {
		t.Fatalf("DialogGetAllDrafts() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestDialogGetAllDraftsPostgresRoundTrip(t *testing.T) {
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
	peerType := int32(mtproto.PEER_USER)
	validPeerID := userID + 1
	validPeerDialogID := mtproto.MakePeerDialogId(peerType, validPeerID)
	draftData, err := jsonx.MarshalToString(mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "hello"}).To_DraftMessage())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, draft_type, draft_message_data) VALUES ($1, $2, $3, $4, 2, $5::jsonb)`, userID, peerType, validPeerID, validPeerDialogID, draftData); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM dialogs WHERE user_id = $1`, userID)
	})

	c := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	got, err := c.DialogGetAllDrafts(&dialog.TLDialogGetAllDrafts{UserId: userID})
	if err != nil || got == nil || len(got.GetDatas()) != 1 || got.GetDatas()[0].GetDraft().GetMessage() != "hello" {
		t.Fatalf("stored draft = (%v, %v), want one draft with message hello", got, err)
	}

	nullPeerID := userID + 2
	nullPeerDialogID := mtproto.MakePeerDialogId(peerType, nullPeerID)
	if _, err := pg.Pool.Exec(ctx, `INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, draft_type, draft_message_data) VALUES ($1, $2, $3, $4, 2, 'null'::jsonb)`, userID, peerType, nullPeerID, nullPeerDialogID); err != nil {
		t.Fatal(err)
	}
	failed, err := c.DialogGetAllDrafts(&dialog.TLDialogGetAllDrafts{UserId: userID})
	if failed != nil || err != mtproto.ErrInternalServerError {
		t.Fatalf("null draft = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", failed, err)
	}
}
