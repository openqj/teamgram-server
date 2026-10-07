package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelsGetChannelsUsesStoredChannelAndValidatesHash(t *testing.T) {
	channelID := time.Now().UnixNano()
	const owner int64 = 98301
	const member int64 = 98302
	const admin int64 = 98303
	accessHash := channelID + 11
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: accessHash, Creator: owner, Title: "stored-channel", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.InviteChannelMembers(channelID, owner, []int64{member, admin}); err != nil {
		t.Fatal(err)
	}
	if err := domain.EditChannelAdmin(channelID, owner, admin, &domain.ChannelAdminRights{AddAdmins: true}, "moderator"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash,
	}).To_InputChannel()
	chats, err := core.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{input}})
	if err != nil || chats == nil || len(chats.GetChats()) != 1 || chats.GetChats()[0].GetTitle() != "stored-channel" {
		t.Fatalf("get channels: result=%+v err=%v", chats, err)
	}

	full, err := core.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil || full == nil || full.GetFullChat() == nil {
		t.Fatalf("get full channel: result=%+v err=%v", full, err)
	}
	channelFull := full.GetFullChat().To_ChannelFull()
	if channelFull.GetParticipantsCount().GetValue() != 3 || channelFull.GetAdminsCount().GetValue() != 2 {
		t.Fatalf("member counts: %+v", channelFull)
	}
	if err = full.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode full channel: %v", err)
	}

	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: accessHash + 1,
	}).To_InputChannel()
	if _, err = core.ChannelsGetChannels(&mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{badHash}}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash: %v", err)
	}
}

func TestChannelsUnsupportedDataMethodsFailClosed(t *testing.T) {
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 98311}}
	checks := []struct {
		name string
		call func() error
	}{
		{"set stickers", func() error { _, err := core.ChannelsSetStickers(nil); return err }},
	}
	for _, check := range checks {
		if err := check.call(); !errors.Is(err, mtproto.ErrMethodNotImpl) {
			t.Fatalf("%s: %v", check.name, err)
		}
	}
}

func TestChannelsGetInactiveChannelsReturnsTypedEmptyResult(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_MYSQL_DSN is required for canonical channel storage")
	}
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 98311}}
	result, err := core.ChannelsGetInactiveChannels(nil)
	if err != nil {
		t.Fatalf("get inactive channels: %v", err)
	}
	if result == nil || result.GetChats() == nil || result.GetDates() == nil || result.GetUsers() == nil {
		t.Fatalf("get inactive channels returned incomplete result: %+v", result)
	}
	if err = result.Encode(mtproto.NewEncodeBuf(1024), 229); err != nil {
		t.Fatalf("encode inactive channels: %v", err)
	}
}

func TestChannelsGetAdminLogRoundTrip(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_MYSQL_DSN is required for canonical channel storage")
	}
	const owner, member int64 = 98312, 98313
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "admin-log-test", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	result, err := core.ChannelsGetAdminLog(&mtproto.TLChannelsGetAdminLog{Channel: input, Limit: 20})
	if err != nil {
		t.Fatalf("get admin log: %v", err)
	}
	if result == nil || len(result.GetEvents()) == 0 || len(result.GetChats()) != 1 {
		t.Fatalf("admin log result: %+v", result)
	}
	if result.GetEvents()[0].GetUserId() != member {
		t.Fatalf("admin log actor: %+v", result.GetEvents()[0])
	}
	if err = result.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode admin log: %v", err)
	}
}
