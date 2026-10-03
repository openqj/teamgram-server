package core

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestStoredChannelDeliveryValidatesInputPeer(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured")
	}
	if err := channelview.Open(dsn); err != nil {
		t.Fatal("open channel store:", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("open channel store database:", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	channelID := time.Now().UnixNano()
	const owner int64 = 98001
	const member int64 = 98002
	const outsider int64 = 98003
	if _, err = db.Exec(`INSERT INTO apifull_channel
		(id, access_hash, creator_user_id, title, broadcast, megagroup, created_at)
		VALUES (?,?,?,?,?,?,?)`, channelID, channelID, owner, "stored-channel-delivery", 1, 0, time.Now().Unix()); err != nil {
		t.Fatal("insert channel fixture:", err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at)
		VALUES (?,?,?,?)`, channelID, member, owner, time.Now().Unix()); err != nil {
		t.Fatal("insert channel member fixture:", err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := db.Exec(query, channelID); err != nil {
				t.Errorf("clean up channel fixture: %v", err)
			}
		}
	})

	input := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: channelID}).To_InputPeer()
	coreFor := func(userID int64) *MessagesCore {
		return &MessagesCore{
			ctx:    context.Background(),
			MD:     &metadata.RpcMetadata{UserId: userID},
			Logger: logx.WithContext(context.Background()),
		}
	}
	ownerCore := coreFor(owner)
	validSends := []struct {
		name string
		send func() (*mtproto.Updates, error)
	}{
		{
			name: "sendMessage",
			send: func() (*mtproto.Updates, error) {
				return ownerCore.MessagesSendMessage(&mtproto.TLMessagesSendMessage{Peer: input, Message: "stored text"})
			},
		},
		{
			name: "sendMedia",
			send: func() (*mtproto.Updates, error) {
				return ownerCore.MessagesSendMedia(&mtproto.TLMessagesSendMedia{Peer: input, Message: "stored media caption"})
			},
		},
		{
			name: "sendMultiMedia",
			send: func() (*mtproto.Updates, error) {
				return ownerCore.MessagesSendMultiMedia(&mtproto.TLMessagesSendMultiMedia{
					Peer: input,
					MultiMedia: []*mtproto.InputSingleMedia{
						mtproto.MakeTLInputSingleMedia(&mtproto.InputSingleMedia{Message: "stored album caption"}).To_InputSingleMedia(),
					},
				})
			},
		},
	}
	for _, tc := range validSends {
		updates, err := tc.send()
		if err != nil || updates == nil || len(updates.GetUpdates()) != 1 ||
			updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateNewChannelMessage {
			t.Fatalf("%s valid channel send: updates=%+v err=%v", tc.name, updates, err)
		}
	}

	history, err := channelview.HistoryForInputPeer(member, input, 0, 20)
	if err != nil {
		t.Fatal("member reads stored channel messages:", err)
	}
	texts := make(map[string]struct{}, len(history.GetMessages()))
	for _, message := range history.GetMessages() {
		texts[message.GetMessage()] = struct{}{}
	}
	for _, want := range []string{"stored text", "stored media caption", "stored album caption"} {
		if _, ok := texts[want]; !ok {
			t.Fatalf("member history omitted %q: %+v", want, history.GetMessages())
		}
	}

	if _, err = coreFor(outsider).MessagesSendMessage(&mtproto.TLMessagesSendMessage{Peer: input, Message: "outsider write"}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider channel send: got %v, want USER_NOT_PARTICIPANT", err)
	}
	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: channelID + 1}).To_InputPeer()
	if _, err = ownerCore.MessagesSendMessage(&mtproto.TLMessagesSendMessage{Peer: badHash, Message: "bad hash write"}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad-hash channel send: got %v, want CHANNEL_INVALID", err)
	}
	if history, err = channelview.HistoryForInputPeer(member, input, 0, 20); err != nil || len(history.GetMessages()) != 3 {
		t.Fatalf("rejected sends changed stored messages: history=%+v err=%v", history, err)
	}
}
