package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelsReadMessageContentsPersistsMemberReceipt(t *testing.T) {
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	memberID := channelID + 2
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID + 10, Creator: ownerID, Title: "content-read-core", Broadcast: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(ownerID, channelID) })
	if err := domain.InviteChannelMembers(channelID, ownerID, []int64{memberID}); err != nil {
		t.Fatal(err)
	}
	message, err := domain.InsertChannelMessage(channelID, ownerID, 0, "content")
	if err != nil {
		t.Fatal(err)
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 10}).To_InputChannel()
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: memberID}}
	result, err := c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input, Id: []int32{message.MessageID}})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("read content = (%v, %v), want BoolTrue", result, err)
	}
	if result, err = c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input, Id: []int32{message.MessageID}}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("idempotent read content = (%v, %v), want BoolTrue", result, err)
	}

	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 11}).To_InputChannel()
	if result, err = c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: badHash, Id: []int32{message.MessageID}}); result != nil || !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("wrong hash = (%v, %v), want CHANNEL_INVALID", result, err)
	}
	if result, err = (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: ownerID}}).ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input, Id: []int32{999}}); result != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("missing message = (%v, %v), want MESSAGE_ID_INVALID", result, err)
	}
	if result, err = c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input, Id: []int32{message.MessageID, message.MessageID}}); result != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("duplicate message = (%v, %v), want MESSAGE_ID_INVALID", result, err)
	}
	if result, err = c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input, Id: []int32{0}}); result != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("invalid message ID = (%v, %v), want MESSAGE_ID_INVALID", result, err)
	}
	if result, err = c.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input}); result != nil || !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("empty message IDs = (%v, %v), want MESSAGE_ID_INVALID", result, err)
	}
	outsider := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: ownerID + 99}}
	if result, err = outsider.ChannelsReadMessageContents(&mtproto.TLChannelsReadMessageContents{Channel: input, Id: []int32{message.MessageID}}); result != nil || !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider = (%v, %v), want USER_NOT_PARTICIPANT", result, err)
	}
}
