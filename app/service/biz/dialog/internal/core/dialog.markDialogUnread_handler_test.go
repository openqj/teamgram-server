package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestDialogMarkDialogUnreadRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogMarkDialogUnread
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogMarkDialogUnread{PeerType: mtproto.PEER_USER, PeerId: 2, UnreadMark: mtproto.BoolTrue}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogMarkDialogUnread{UserId: 1, PeerType: mtproto.PEER_USER, UnreadMark: mtproto.BoolTrue}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing unread marker", in: &dialog.TLDialogMarkDialogUnread{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogMarkDialogUnread{UserId: 1, PeerType: 99, PeerId: 2, UnreadMark: mtproto.BoolTrue}, want: mtproto.ErrPeerIdInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogMarkDialogUnread(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogMarkDialogUnread() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogMarkDialogUnreadRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogMarkDialogUnread{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2, UnreadMark: mtproto.BoolTrue}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogMarkDialogUnread(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogMarkDialogUnread() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
