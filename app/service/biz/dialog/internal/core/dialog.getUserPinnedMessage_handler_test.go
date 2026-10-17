package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func TestPinnedMessageValue(t *testing.T) {
	if got := pinnedMessageValue(nil).GetV(); got != 0 {
		t.Fatalf("nil dialog: got %d, want 0", got)
	}
	if got := pinnedMessageValue(&dataobject.DialogsDO{PinnedMsgId: 42}).GetV(); got != 42 {
		t.Fatalf("stored pinned message: got %d, want 42", got)
	}
}

func TestDialogGetUserPinnedMessageRejectsInvalidRequest(t *testing.T) {
	cases := []struct {
		name string
		in   *dialog.TLDialogGetUserPinnedMessage
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing user", in: &dialog.TLDialogGetUserPinnedMessage{PeerType: mtproto.PEER_USER, PeerId: 2}, want: mtproto.ErrInputRequestInvalid},
		{name: "missing peer", in: &dialog.TLDialogGetUserPinnedMessage{UserId: 1, PeerType: mtproto.PEER_USER}, want: mtproto.ErrInputRequestInvalid},
		{name: "unknown peer type", in: &dialog.TLDialogGetUserPinnedMessage{UserId: 1, PeerType: 99, PeerId: 2}, want: mtproto.ErrPeerIdInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (&DialogCore{}).DialogGetUserPinnedMessage(tc.in)
			if got != nil || err != tc.want {
				t.Fatalf("DialogGetUserPinnedMessage() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestDialogGetUserPinnedMessageRequiresPostgresStore(t *testing.T) {
	request := &dialog.TLDialogGetUserPinnedMessage{UserId: 1, PeerType: mtproto.PEER_USER, PeerId: 2}
	cores := map[string]*DialogCore{
		"nil core":            nil,
		"nil service context": {svcCtx: nil},
		"nil dao":             {svcCtx: &svc.ServiceContext{}},
		"nil postgres":        {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}}},
		"nil store":           {svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: &dao.Postgres{}}}},
	}

	for name, c := range cores {
		t.Run(name, func(t *testing.T) {
			got, err := c.DialogGetUserPinnedMessage(request)
			if got != nil || err != mtproto.ErrMethodNotImpl {
				t.Fatalf("DialogGetUserPinnedMessage() = (%v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
			}
		})
	}
}
