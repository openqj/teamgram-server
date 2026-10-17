package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	postgres_dao "github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogReorderPinnedSavedDialogsRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogReorderPinnedSavedDialogs
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogReorderPinnedSavedDialogs{}, want: mtproto.ErrInputRequestInvalid},
		{name: "negative user", in: &dialog.TLDialogReorderPinnedSavedDialogs{UserId: -1}, want: mtproto.ErrInputRequestInvalid},
		{name: "nil order peer", in: &dialog.TLDialogReorderPinnedSavedDialogs{UserId: 1, Order: []*mtproto.PeerUtil{nil}}, want: mtproto.ErrPeerIdInvalid},
		{name: "missing peer id", in: &dialog.TLDialogReorderPinnedSavedDialogs{UserId: 1, Order: []*mtproto.PeerUtil{{PeerType: mtproto.PEER_USER}}}, want: mtproto.ErrPeerIdInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogReorderPinnedSavedDialogs{UserId: 1, Order: []*mtproto.PeerUtil{{PeerType: 99, PeerId: 2}}}, want: mtproto.ErrPeerIdInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogReorderPinnedSavedDialogs(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogReorderPinnedSavedDialogs() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogReorderPinnedSavedDialogsRequiresPostgresPersistence(t *testing.T) {
	request := &dialog.TLDialogReorderPinnedSavedDialogs{UserId: 1}
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
			got, err := c.DialogReorderPinnedSavedDialogs(request)
			if got != nil || err != mtproto.ErrInternalServerError {
				t.Fatalf("DialogReorderPinnedSavedDialogs() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
			}
		})
	}
}
