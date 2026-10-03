package core

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func exportMessageLinkCore(userID int64) *ApiFullCore {
	return &ApiFullCore{
		MD: &metadata.RpcMetadata{UserId: userID},
	}
}

func openExportMessageLinkAuditDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", isolatedAuditDSN(t))
	if err != nil {
		t.Fatal("open isolated audit MySQL:", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestChannelsExportMessageLinkPublicMessageRoundTrip(t *testing.T) {
	cleanupDB := openExportMessageLinkAuditDB(t)

	channelID := time.Now().UnixNano()
	const owner, member, outsider int64 = 91041, 91042, 91043
	username := "audit_" + time.Now().Format("150405")
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := cleanupDB.Exec(query, channelID); err != nil {
				t.Errorf("clean up channel fixture: %v", err)
			}
		}
	})
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Broadcast: true}); err != nil {
		t.Fatal("save channel fixture:", err)
	}
	if err := channelview.UpdateChannelUsername(channelID, username); err != nil {
		t.Fatal("persist channel username:", err)
	}
	if err := channelview.UpdateChannelUsername(channelID, username); err != nil {
		t.Fatal("persist unchanged channel username:", err)
	}
	if err := domain.JoinChannel(channelID, member); err != nil {
		t.Fatal("join channel member fixture:", err)
	}
	if _, err := channelview.Post(owner, channelID, "link target", time.Now().Unix()); err != nil {
		t.Fatal("insert channel message:", err)
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	core := exportMessageLinkCore(owner)
	result, err := core.ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 1})
	if err != nil {
		t.Fatal("export public message link:", err)
	}
	want := "https://t.me/" + username + "/1"
	if result == nil || result.GetLink() != want || result.GetHtml() != want {
		t.Fatalf("exported link = %+v, want link/html %q", result, want)
	}
	memberResult, err := exportMessageLinkCore(member).ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 1})
	if err != nil || memberResult == nil || memberResult.GetLink() != want {
		t.Fatalf("member exported link = (%+v, %v), want %q", memberResult, err, want)
	}
	if err = result.To_ExportedMessageLink().Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatal("encode exported link:", err)
	}

	if _, err = core.ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 2}); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("missing source message error = %v, want MESSAGE_ID_INVALID", err)
	}
	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 1}).To_InputChannel()
	if _, err = core.ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: badHash, Id: 1}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash error = %v, want CHANNEL_INVALID", err)
	}
	if _, err = exportMessageLinkCore(outsider).ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 1}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("non-member export error = %v, want USER_NOT_PARTICIPANT", err)
	}
	if _, err = core.ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 1, Grouped: true}); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("grouped link error = %v, want METHOD_NOT_IMPL", err)
	}
	if _, err = core.ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 1, Thread: true}); !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("thread link error = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestChannelsExportMessageLinkRequiresAuthenticationAndPublicChannel(t *testing.T) {
	unauthenticated := &ApiFullCore{}
	if result, err := unauthenticated.ChannelsExportMessageLink(nil); result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated export = (%v, %v), want nil result and AUTH_KEY_UNREGISTERED", result, err)
	}

	channelID := time.Now().UnixNano()
	const owner int64 = 91042
	cleanupDB := openExportMessageLinkAuditDB(t)
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM apifull_channel_message_hidden WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message WHERE channel_id=?`,
			`DELETE FROM apifull_channel_message_seq WHERE channel_id=?`,
			`DELETE FROM apifull_channel_read_state WHERE channel_id=?`,
			`DELETE FROM apifull_channel_member WHERE channel_id=?`,
			`DELETE FROM apifull_channel WHERE id=?`,
		} {
			if _, err := cleanupDB.Exec(query, channelID); err != nil {
				t.Errorf("clean up private channel fixture: %v", err)
			}
		}
	})
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Broadcast: true}); err != nil {
		t.Fatal("save private channel fixture:", err)
	}
	if _, err := channelview.Post(owner, channelID, "private link target", time.Now().Unix()); err != nil {
		t.Fatal("insert private channel message:", err)
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	core := exportMessageLinkCore(owner)
	if result, err := core.ChannelsExportMessageLink(&mtproto.TLChannelsExportMessageLink{Channel: input, Id: 1}); result != nil || !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("private channel export = (%v, %v), want nil result and CHANNEL_PRIVATE", result, err)
	}
}
