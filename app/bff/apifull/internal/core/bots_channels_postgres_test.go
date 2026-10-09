package core

import (
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/state"
)

func TestBotMenuButtonPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is required")
	}
	ownerID := int64(880000000 + os.Getpid()%100000)
	botID := ownerID + 1
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: ownerID}}
	button := mtproto.MakeTLBotMenuButton(&mtproto.BotMenuButton{Text: "Open", Url: "https://bot.example.test"}).To_BotMenuButton()
	if _, err := c.BotsSetBotMenuButton(&mtproto.TLBotsSetBotMenuButton{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: 1}).To_InputUser(), Button: button,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := c.BotsGetBotMenuButton(&mtproto.TLBotsGetBotMenuButton{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: 1}).To_InputUser(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetText() != button.GetText() || got.GetUrl() != button.GetUrl() {
		t.Fatalf("menu button = %#v", got)
	}
}

func TestChannelsSetStickersPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !domain.Ready() || !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is required")
	}
	ownerID := int64(890000000 + os.Getpid()%100000)
	channelID := ownerID + 1
	set, err := persist.CreateStickerSet(t.Context(), ownerID, "Channel stickers", fmt.Sprintf("channel_stickers_%d", ownerID), false, false, false, false, false, []persist.StickerDocumentInput{{ID: ownerID + 2, AccessHash: ownerID + 3, Alt: "😀"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.DeleteStickerSet(t.Context(), ownerID, set.ID) })
	if err = domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID + 9, Creator: ownerID, Title: "Sticker channel", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(ownerID, channelID) })
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: ownerID}}
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 9}).To_InputChannel()
	inputSet := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID, AccessHash: set.AccessHash}).To_InputStickerSet()
	if _, err = c.ChannelsSetStickers(&mtproto.TLChannelsSetStickers{Channel: inputChannel, Stickerset: inputSet}); err != nil {
		t.Fatal(err)
	}
	gotID, found, err := domain.LoadChannelStickerSet(channelID)
	if err != nil || !found || gotID != set.ID {
		t.Fatalf("channel sticker binding: id=%d found=%v err=%v", gotID, found, err)
	}
	emojiSet, err := persist.CreateStickerSet(t.Context(), ownerID, "Channel emoji", fmt.Sprintf("channel_emoji_%d", ownerID), false, true, false, false, false, []persist.StickerDocumentInput{{ID: ownerID + 4, AccessHash: ownerID + 5, Alt: "😀"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.DeleteStickerSet(t.Context(), ownerID, emojiSet.ID) })
	t.Cleanup(func() { _ = domain.SetChannelEmojiStickerSetAfterAuthorization(ownerID, channelID, 0) })
	emojiRef := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: emojiSet.ID, AccessHash: emojiSet.AccessHash}).To_InputStickerSet()
	if err = state.SetChannelEmojiStickerSet(t.Context(), ownerID, channelID, emojiRef); err != nil {
		t.Fatal(err)
	}
	full, err := c.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: inputChannel})
	if err != nil {
		t.Fatal(err)
	}
	channelFull := full.GetFullChat().To_ChannelFull().GetData2()
	if channelFull.GetStickerset() == nil || channelFull.GetStickerset().GetId() != set.ID {
		t.Fatalf("regular channel sticker set = %+v", channelFull.GetStickerset())
	}
	if channelFull.GetEmojiset() == nil || channelFull.GetEmojiset().GetId() != emojiSet.ID || !channelFull.GetEmojiset().GetEmojis() {
		t.Fatalf("channel emoji sticker set = %+v", channelFull.GetEmojiset())
	}
	if err = domain.SetChannelEmojiStickerSetAfterAuthorization(ownerID, channelID, 0); err != nil {
		t.Fatal(err)
	}
	full, err = c.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: inputChannel})
	if err != nil || full.GetFullChat().To_ChannelFull().GetData2().GetEmojiset() != nil {
		t.Fatalf("cleared channel emoji set: full=%+v err=%v", full, err)
	}
	empty := mtproto.MakeTLInputStickerSetEmpty(&mtproto.InputStickerSet{}).To_InputStickerSet()
	if _, err = c.ChannelsSetStickers(&mtproto.TLChannelsSetStickers{Channel: inputChannel, Stickerset: empty}); err != nil {
		t.Fatal(err)
	}
	if _, found, err = domain.LoadChannelStickerSet(channelID); err != nil || found {
		t.Fatalf("cleared channel sticker binding: found=%v err=%v", found, err)
	}
}
