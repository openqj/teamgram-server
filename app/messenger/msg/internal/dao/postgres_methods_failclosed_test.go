package dao

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

func TestPostgresMethodsFailClosedWhenStoreIsMissing(t *testing.T) {
	ctx := context.Background()
	d := &Dao{}
	cases := []struct {
		name string
		call func() error
	}{
		{name: "chat participants", call: func() error {
			_, err := d.SelectChatParticipants(ctx, 1, nil)
			return err
		}},
		{name: "message data list", call: func() error {
			_, err := d.SelectMessageByDataIDList(ctx, 1, []int64{1}, nil)
			return err
		}},
		{name: "message data users", call: func() error {
			_, err := d.SelectMessagesByDataIDUsers(ctx, 1, []int64{1}, nil)
			return err
		}},
		{name: "phone call messages", call: func() error {
			_, err := d.SelectPhoneCallMessages(ctx, 1, 1, 0, 10, nil)
			return err
		}},
		{name: "delete in transaction", call: func() error {
			_, err := d.DeleteMessageByIDListOn(ctx, nil, 1, []int32{1})
			return err
		}},
		{name: "mentioned count", call: func() error {
			_, err := d.CountMentionedMessages(ctx, 1, 1, 1)
			return err
		}},
		{name: "unread incoming count", call: func() error {
			_, err := d.CountUnreadIncoming(ctx, 1, 1, 1, 2, 0, 1)
			return err
		}},
		{name: "pinned messages", call: func() error {
			_, err := d.SelectPinnedMessages(ctx, 1, 1, 1, nil)
			return err
		}},
		{name: "unpin by id list", call: func() error {
			_, err := d.UpdateUnpinnedByIDList(ctx, 1, []int32{1})
			return err
		}},
		{name: "dialog last message", call: func() error {
			_, err := d.SelectDialogLastMessageID(ctx, 1, 1, 1)
			return err
		}},
		{name: "dialog last message excluding ids", call: func() error {
			_, err := d.SelectDialogLastMessageIDNotIDList(ctx, 1, 1, 1, []int32{1})
			return err
		}},
		{name: "unread in dialog", call: func() error {
			_, err := d.CountUnreadInDialog(ctx, 1, 1, 1, 2, 0, 1)
			return err
		}},
		{name: "edit in transaction", call: func() error {
			_, err := d.UpdateMessageEditOn(ctx, nil, "{}", "text", 1, 1)
			return err
		}},
		{name: "inbox send", call: func() error {
			_, err := d.SendUserMessageToInbox(ctx, 1, 2, 1, 1, &mtproto.Message{})
			return err
		}},
		{name: "inbox delete", call: func() error {
			return d.DeleteInboxMessages(ctx, 1, mtproto.MakePeerUtil(mtproto.PEER_USER, 2), []int64{1}, nil)
		}},
		{name: "batch delivery", call: func() error {
			_, err := d.SendMessagesWithDeliveries(ctx, 1, mtproto.MakePeerUtil(mtproto.PEER_USER, 2), []*msg.OutboxMessage{{Message: &mtproto.Message{}}}, []*inbox.TLInboxSendUserMessageToInboxV2{{UserId: 2, FromId: 1, PeerType: mtproto.PEER_USER, PeerId: 2}})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("expected PostgreSQL configuration error")
			}
		})
	}
}
