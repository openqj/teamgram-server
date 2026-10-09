package core

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelsToggleAutotranslationPostgresRoundTripAndAuthorization(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is required for PostgreSQL channel settings")
	}
	const owner, other int64 = 98401, 98402
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "autotranslation-pg"}); err != nil {
		t.Fatal("save channel:", err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	ownerCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	updated, err := ownerCore.ChannelsToggleAutotranslation(&mtproto.TLChannelsToggleAutotranslation{Channel: input, Enabled: mtproto.BoolTrue})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 || !updated.GetChats()[0].GetAutotranslation() {
		t.Fatalf("enable autotranslation: result=%+v err=%v", updated, err)
	}
	loaded, ok, err := domain.LoadChannel(channelID)
	if err != nil || !ok || !loaded.Autotranslation {
		t.Fatalf("enabled state not persisted: channel=%+v ok=%v err=%v", loaded, ok, err)
	}

	updated, err = ownerCore.ChannelsToggleAutotranslation(&mtproto.TLChannelsToggleAutotranslation{Channel: input, Enabled: mtproto.BoolFalse})
	if err != nil || updated == nil || len(updated.GetChats()) != 1 || updated.GetChats()[0].GetAutotranslation() {
		t.Fatalf("disable autotranslation: result=%+v err=%v", updated, err)
	}
	loaded, ok, err = domain.LoadChannel(channelID)
	if err != nil || !ok || loaded.Autotranslation {
		t.Fatalf("disabled state not persisted: channel=%+v ok=%v err=%v", loaded, ok, err)
	}

	otherCore := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: other}}
	if _, err = otherCore.ChannelsToggleAutotranslation(&mtproto.TLChannelsToggleAutotranslation{Channel: input, Enabled: mtproto.BoolTrue}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("non-owner toggle error=%v, want CHAT_ADMIN_REQUIRED", err)
	}
}
