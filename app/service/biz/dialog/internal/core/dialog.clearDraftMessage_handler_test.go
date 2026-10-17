package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	postgres_dao "github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogClearDraftMessageRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogClearDraftMessage
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogClearDraftMessage{PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "negative user", in: &dialog.TLDialogClearDraftMessage{UserId: -1, PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogClearDraftMessage{UserId: 1, PeerType: mtproto.PEER_USER}, want: mtproto.ErrInputRequestInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogClearDraftMessage{UserId: 1, PeerType: 99, PeerId: 2}, want: mtproto.ErrPeerIdInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogClearDraftMessage(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogClearDraftMessage() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogClearDraftMessageRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogClearDraftMessage{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
		"nil dialogs":         {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{Store: &postgres_dao.Store{}}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogClearDraftMessage(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogClearDraftMessage() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
