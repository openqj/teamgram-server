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

func TestCustomEmojiStickersPostgresRoundTrip(t *testing.T) {
	if !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	uid := int64(885000000 + os.Getpid()%100000)
	shortName := fmt.Sprintf("emoji_catalog_%d", uid)
	set, err := persist.CreateStickerSet(context.Background(), uid, "Emoji catalog test", shortName, false, true, false, false, false, []persist.StickerDocumentInput{{
		ID: uid + 1, AccessHash: uid + 2, Alt: "😀",
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.DeleteStickerSet(context.Background(), uid, set.ID) })
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(context.Background(), `UPDATE apifull_sticker_set SET featured=TRUE WHERE id=$1`, set.ID); err != nil {
		t.Fatal(err)
	}

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	all, err := c.MessagesGetEmojiStickers(&mtproto.TLMessagesGetEmojiStickers{})
	if err != nil || !containsStickerSet(all.GetSets(), set.ID) {
		t.Fatalf("emoji stickers=%v err=%v", all, err)
	}
	same, err := c.MessagesGetEmojiStickers(&mtproto.TLMessagesGetEmojiStickers{Hash: all.GetHash()})
	if err != nil || same.GetPredicateName() != mtproto.Predicate_messages_allStickersNotModified {
		t.Fatalf("emoji stickers same hash=%v err=%v", same, err)
	}
	featured, err := c.MessagesGetFeaturedEmojiStickers(&mtproto.TLMessagesGetFeaturedEmojiStickers{})
	if err != nil || !containsCoveredStickerSet(featured.GetSets(), set.ID) {
		t.Fatalf("featured emoji stickers=%v err=%v", featured, err)
	}
}

func containsStickerSet(sets []*mtproto.StickerSet, id int64) bool {
	for _, set := range sets {
		if set != nil && set.GetId() == id {
			return true
		}
	}
	return false
}

func containsCoveredStickerSet(sets []*mtproto.StickerSetCovered, id int64) bool {
	for _, set := range sets {
		if set != nil && set.GetSet() != nil && set.GetSet().GetId() == id {
			return true
		}
	}
	return false
}
