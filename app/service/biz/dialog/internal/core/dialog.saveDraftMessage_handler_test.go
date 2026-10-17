package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func saveDraftMessageTestRequest() *dialog.TLDialogSaveDraftMessage {
	return &dialog.TLDialogSaveDraftMessage{
		UserId:   1,
		PeerType: mtproto.PEER_USER,
		PeerId:   2,
		Message:  mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{Message: "hello"}).To_DraftMessage(),
	}
}

func TestDialogSaveDraftMessageRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogSaveDraftMessage
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogSaveDraftMessage{PeerType: mtproto.PEER_USER, PeerId: 2, Message: saveDraftMessageTestRequest().Message}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogSaveDraftMessage{UserId: 1, PeerType: mtproto.PEER_USER, Message: saveDraftMessageTestRequest().Message}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing message", in: &dialog.TLDialogSaveDraftMessage{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogSaveDraftMessage{UserId: 1, PeerType: 99, PeerId: 2, Message: saveDraftMessageTestRequest().Message}, want: mtproto.ErrPeerIdInvalid},
		{name: "unknown draft predicate", in: &dialog.TLDialogSaveDraftMessage{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2, Message: &mtproto.DraftMessage{Message: "hello"}}, want: mtproto.ErrInputRequestInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogSaveDraftMessage(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogSaveDraftMessage() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogSaveDraftMessageRequiresPostgresPersistence(t *testing.T) {
	request := saveDraftMessageTestRequest()
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil pool":            {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Store: &postgres_dao.Store{}}}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogSaveDraftMessage(request)
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("DialogSaveDraftMessage() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
			}
		})
	}
}
