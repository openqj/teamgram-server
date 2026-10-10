package core

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestEmojiGroupsCatalogPostgresRoundTrip(t *testing.T) {
	if !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	uid := int64(884000000 + os.Getpid()%100000)
	title := fmt.Sprintf("emoji-group-%d", uid)
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO apifull_emoji_group(kind, title, icon_emoji_id, emoticons, position)
		VALUES ('status', $1, $2, '["😀"]'::jsonb, 1),
		       ('profile', $1, $2, '["😎"]'::jsonb, 2),
		       ('sticker', $1, $2, '["🎉"]'::jsonb, 3)`, title, uid+1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_emoji_group WHERE title=$1`, title) })

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	status, err := c.MessagesGetEmojiStatusGroups(&mtproto.TLMessagesGetEmojiStatusGroups{})
	if err != nil || !hasEmojiGroup(status.GetGroups(), title, "😀") {
		t.Fatalf("status groups=%v err=%v", status, err)
	}
	statusSame, err := c.MessagesGetEmojiStatusGroups(&mtproto.TLMessagesGetEmojiStatusGroups{Hash: status.GetHash()})
	if err != nil || statusSame.GetPredicateName() != mtproto.Predicate_messages_emojiGroupsNotModified {
		t.Fatalf("status same hash=%v err=%v", statusSame, err)
	}
	profile, err := c.MessagesGetEmojiProfilePhotoGroups(&mtproto.TLMessagesGetEmojiProfilePhotoGroups{})
	if err != nil || !hasEmojiGroup(profile.GetGroups(), title, "😎") {
		t.Fatalf("profile groups=%v err=%v", profile, err)
	}
	sticker, err := c.MessagesGetEmojiStickerGroups(&mtproto.TLMessagesGetEmojiStickerGroups{})
	if err != nil || !hasEmojiGroup(sticker.GetGroups(), title, "🎉") {
		t.Fatalf("sticker groups=%v err=%v", sticker, err)
	}
}

func hasEmojiGroup(groups []*mtproto.EmojiGroup, title, emoticon string) bool {
	for _, group := range groups {
		if group == nil || group.GetTitle() != title {
			continue
		}
		for _, value := range group.GetEmoticons() {
			if value == emoticon {
				return true
			}
		}
	}
	return false
}
