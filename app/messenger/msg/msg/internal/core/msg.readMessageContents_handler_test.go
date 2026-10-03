package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/plugin"
)

type readReactionPluginStub struct {
	plugin.MsgPlugin
	err error
}

func (s *readReactionPluginStub) ReadReactionUnreadMessage(context.Context, int64, int32) error {
	return s.err
}

func TestMsgReadMessageContentsRejectsNilRequest(t *testing.T) {
	got, err := (&MsgCore{}).MsgReadMessageContents(nil)
	if got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MsgReadMessageContents(nil) = (%v, %v), want nil result and INPUT_REQUEST_INVALID", got, err)
	}
}

func TestReadReactionUnreadMessageContentsFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		plugin plugin.MsgPlugin
		want   error
	}{
		{name: "provider missing", want: mtproto.ErrMethodNotImpl},
		{name: "provider error", plugin: &readReactionPluginStub{err: errors.New("reaction store unavailable")}, want: errors.New("reaction store unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core := &MsgCore{svcCtx: &svc.ServiceContext{MsgPlugin: tc.plugin}}
			got, err := core.readReactionUnreadMessageContents(&msg.TLMsgReadMessageContents{
				UserId: 1,
				PeerId: 2,
				Id:     []*msg.ContentMessage{{Id: 3, Reaction: true}},
			})
			if got != 0 || err == nil || err.Error() != tc.want.Error() {
				t.Fatalf("readReactionUnreadMessageContents() = (%d, %v), want error %v", got, err, tc.want)
			}
		})
	}
}
