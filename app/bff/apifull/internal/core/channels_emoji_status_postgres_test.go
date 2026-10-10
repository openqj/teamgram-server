package core

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestChannelsUpdateEmojiStatusPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !domain.Ready() || !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN with APIFull PostgreSQL schema is required")
	}
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	channelID := time.Now().UnixNano()
	owner, admin, outsider := channelID+1, channelID+2, channelID+3
	docID := channelID + 4
	if _, err = db.ExecContext(context.Background(), `INSERT INTO apifull_emoji_document
		(id, access_hash, channel_status) VALUES ($1, $2, TRUE)`, docID, docID+100); err != nil {
		t.Fatal(err)
	}
	if err = domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID + 99, Creator: owner, Title: "emoji-status", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_emoji_document WHERE id=$1`, docID)
		_ = domain.DeleteChannel(owner, channelID)
	})
	if err = domain.InviteChannelMembers(channelID, owner, []int64{admin}); err != nil {
		t.Fatal(err)
	}
	if err = domain.EditChannelAdmin(channelID, owner, admin, &domain.ChannelAdminRights{ChangeInfo: true}, "profile"); err != nil {
		t.Fatal(err)
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 99}).To_InputChannel()
	status := mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: docID, Until_FLAGINT32: mtproto.MakeFlagsInt32(1234)}).To_EmojiStatus()
	call := func(uid int64, emoji *mtproto.EmojiStatus) (*mtproto.Updates, error) {
		return (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}).ChannelsUpdateEmojiStatus(&mtproto.TLChannelsUpdateEmojiStatus{Channel: input, EmojiStatus: emoji})
	}
	updates, err := call(owner, status)
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatalf("owner update: updates=%v err=%v", updates, err)
	}
	if len(updates.GetUpdates()) != 1 || updates.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateChannel || len(updates.GetChats()) != 1 {
		t.Fatalf("update payload: %+v", updates)
	}
	chat := updates.GetChats()[0]
	if chat.GetEmojiStatus() == nil || chat.GetEmojiStatus().GetDocumentId() != docID || chat.GetEmojiStatus().GetUntil_INT32() != 1234 {
		t.Fatalf("chat emoji status: %+v", chat)
	}
	loaded, ok, err := domain.LoadChannel(channelID)
	if err != nil || !ok || loaded.EmojiStatusDocumentID != docID || loaded.EmojiStatusUntil != 1234 {
		t.Fatalf("readback: channel=%+v ok=%v err=%v", loaded, ok, err)
	}
	if _, err = call(admin, mtproto.MakeTLEmojiStatusEmpty(nil).To_EmojiStatus()); err != nil {
		t.Fatalf("admin clear: %v", err)
	}
	loaded, _, err = domain.LoadChannel(channelID)
	if err != nil || loaded.EmojiStatusDocumentID != 0 || loaded.EmojiStatusUntil != 0 {
		t.Fatalf("cleared readback: %+v err=%v", loaded, err)
	}
	if _, err = call(outsider, status); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("outsider error=%v, want CHAT_ADMIN_REQUIRED", err)
	}
	if _, err = (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}).ChannelsUpdateEmojiStatus(&mtproto.TLChannelsUpdateEmojiStatus{
		Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 100}).To_InputChannel(), EmojiStatus: status,
	}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash error=%v, want CHANNEL_INVALID", err)
	}
	if _, err = call(owner, mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{DocumentId: docID + 999}).To_EmojiStatus()); !errors.Is(err, mtproto.ErrDocumentInvalid) {
		t.Fatalf("uncatalogued document error=%v, want DOCUMENT_INVALID", err)
	}
	if got := channelview.Chat(loaded, true); got.GetEmojiStatus() != nil {
		t.Fatalf("cleared channel view retained status: %+v", got.GetEmojiStatus())
	}
}
