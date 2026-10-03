package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestChannelsUpdateColorRoundTrip(t *testing.T) {
	const owner, outsider int64 = 81471, 81472
	ownerCore, _ := newAccentColorCore(owner)
	if _, err := ownerCore.AccountUpdateColor(&mtproto.TLAccountUpdateColor{
		BackgroundEmojiId: wrapperspb.Int64(71471),
	}); err != nil {
		t.Fatalf("set account color: %v", err)
	}
	created, err := ownerCore.ChannelsCreateChannel(&mtproto.TLChannelsCreateChannel{Title: "color-test"})
	if err != nil || created == nil || len(created.GetChats()) != 1 {
		t.Fatalf("create channel: result=%+v err=%v", created, err)
	}
	channelID := created.GetChats()[0].GetId()
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  channelID,
		AccessHash: created.GetChats()[0].GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()

	updated, err := ownerCore.ChannelsUpdateColor(&mtproto.TLChannelsUpdateColor{
		Channel:           input,
		Color_FLAGINT32:   wrapperspb.Int32(2),
		BackgroundEmojiId: wrapperspb.Int64(81473),
	})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 {
		t.Fatalf("update channel color: result=%+v err=%v", updated, err)
	}
	chat := updated.GetChats()[0]
	if chat.GetColor_FLAGPEERCOLOR().GetColor().GetValue() != 2 || chat.GetColor_FLAGPEERCOLOR().GetBackgroundEmojiId().GetValue() != 81473 {
		t.Fatalf("channel color update: %+v", chat.GetColor_FLAGPEERCOLOR())
	}
	if err = updated.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode color update: %v", err)
	}

	profileUpdate, err := ownerCore.ChannelsUpdateColor(&mtproto.TLChannelsUpdateColor{
		ForProfile:      true,
		Channel:         input,
		Color_FLAGINT32: wrapperspb.Int32(1),
	})
	if err != nil || profileUpdate == nil || len(profileUpdate.GetChats()) != 1 {
		t.Fatalf("update channel profile color: result=%+v err=%v", profileUpdate, err)
	}
	chat = profileUpdate.GetChats()[0]
	if chat.GetProfileColor().GetColor().GetValue() != 1 || chat.GetColor_FLAGPEERCOLOR().GetColor().GetValue() != 2 {
		t.Fatalf("channel profile color update: color=%+v profile=%+v", chat.GetColor_FLAGPEERCOLOR(), chat.GetProfileColor())
	}

	personal, err := ownerCore.AccountGetDefaultBackgroundEmojis(nil)
	if personal != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("missing background emoji catalogue: result=%+v err=%v", personal, err)
	}
	outsiderCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsider}}
	if _, err = outsiderCore.ChannelsUpdateColor(&mtproto.TLChannelsUpdateColor{
		Channel: input, Color_FLAGINT32: wrapperspb.Int32(0),
	}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("outsider color update: got %v", err)
	}
	invalidAccess := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  channelID,
		AccessHash: created.GetChats()[0].GetAccessHash_FLAGINT64().GetValue() + 1,
	}).To_InputChannel()
	if _, err = ownerCore.ChannelsUpdateColor(&mtproto.TLChannelsUpdateColor{
		Channel: invalidAccess, Color_FLAGINT32: wrapperspb.Int32(0),
	}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("invalid access hash update: got %v", err)
	}
	got, err := ownerCore.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{
		Id: []*mtproto.InputChannel{input},
	})
	if err != nil || got == nil || len(got.GetChats()) != 1 || got.GetChats()[0].GetColor_FLAGPEERCOLOR().GetColor().GetValue() != 2 {
		t.Fatalf("read channel color after rejected update: result=%+v err=%v", got, err)
	}
}
