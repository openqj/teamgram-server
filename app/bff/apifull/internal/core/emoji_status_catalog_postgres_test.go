package core

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestEmojiStatusCatalogPostgresRoundTrip(t *testing.T) {
	if !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	uid := int64(883000000 + os.Getpid()%100000)
	docID := uid + 1
	docID2 := uid + 2
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO apifull_emoji_document
			(id, access_hash, status, channel_status, restricted_status, background)
		VALUES ($1, $2, TRUE, TRUE, TRUE, TRUE), ($3, $4, TRUE, FALSE, FALSE, FALSE)`,
		docID, docID+100, docID2, docID2+100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_emoji_document WHERE id IN ($1, $2)`, docID, docID2)
	})

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	defaults, err := c.AccountGetDefaultEmojiStatuses(&mtproto.TLAccountGetDefaultEmojiStatuses{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.GetPredicateName() != mtproto.Predicate_account_emojiStatuses || !containsEmojiStatusDocument(defaults.GetStatuses(), docID) || !containsEmojiStatusDocument(defaults.GetStatuses(), docID2) {
		t.Fatalf("default statuses=%v", defaults)
	}
	same, err := c.AccountGetDefaultEmojiStatuses(&mtproto.TLAccountGetDefaultEmojiStatuses{Hash: defaults.GetHash()})
	if err != nil || same.GetPredicateName() != mtproto.Predicate_account_emojiStatusesNotModified {
		t.Fatalf("default statuses same hash=%v err=%v", same, err)
	}

	channel, err := c.AccountGetChannelDefaultEmojiStatuses(&mtproto.TLAccountGetChannelDefaultEmojiStatuses{})
	if err != nil || !containsEmojiStatusDocument(channel.GetStatuses(), docID) {
		t.Fatalf("channel statuses=%v err=%v", channel, err)
	}
	restricted, err := c.AccountGetChannelRestrictedStatusEmojis(&mtproto.TLAccountGetChannelRestrictedStatusEmojis{})
	if err != nil || !containsInt64(restricted.GetDocumentId(), docID) {
		t.Fatalf("restricted statuses=%v err=%v", restricted, err)
	}
	background, err := c.AccountGetDefaultBackgroundEmojis(&mtproto.TLAccountGetDefaultBackgroundEmojis{})
	if err != nil || !containsInt64(background.GetDocumentId(), docID) {
		t.Fatalf("background statuses=%v err=%v", background, err)
	}
	backgroundSame, err := c.AccountGetDefaultBackgroundEmojis(&mtproto.TLAccountGetDefaultBackgroundEmojis{Hash: background.GetHash()})
	if err != nil || backgroundSame.GetPredicateName() != mtproto.Predicate_emojiListNotModified {
		t.Fatalf("background same hash=%v err=%v", backgroundSame, err)
	}
}

func containsEmojiStatusDocument(statuses []*mtproto.EmojiStatus, id int64) bool {
	for _, status := range statuses {
		if status != nil && status.GetDocumentId() == id {
			return true
		}
	}
	return false
}

func containsInt64(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
