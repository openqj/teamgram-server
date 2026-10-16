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

func TestFeaturedEmojiStickersFiltersBeforeLimit(t *testing.T) {
	if !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	uid := int64(886000000 + os.Getpid()%100000)
	prefix := fmt.Sprintf("emoji_limit_%d_", uid)
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const distractors = 100
	_, err = db.ExecContext(ctx, `
		INSERT INTO apifull_sticker_set
			(access_hash, owner_user_id, title, short_name, featured, emojis)
		SELECT $1 + n, $2, 'non-emoji', $3 || n::text, TRUE, FALSE
		FROM generate_series(1, $4) AS n`, uid, uid, prefix, distractors)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO apifull_sticker_user_set
			(user_id, set_id, installed, archived, unread, order_index)
		SELECT $1, id, TRUE, FALSE, FALSE, 0
		FROM apifull_sticker_set
		WHERE owner_user_id=$1 AND short_name LIKE ($2 || '%')`, uid, prefix)
	if err != nil {
		t.Fatal(err)
	}
	var targetID int64
	targetShortName := prefix + "target"
	err = db.QueryRowContext(ctx, `
		INSERT INTO apifull_sticker_set
			(access_hash, owner_user_id, title, short_name, featured, emojis)
		VALUES ($1, $2, 'emoji', $3, TRUE, TRUE)
		RETURNING id`, uid+1000, uid, targetShortName).Scan(&targetID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO apifull_sticker_user_set
			(user_id, set_id, installed, archived, unread, order_index)
		VALUES ($1, $2, TRUE, FALSE, FALSE, 1)`, uid, targetID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM apifull_sticker_set WHERE owner_user_id=$1`, uid)
	})

	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	featured, err := c.MessagesGetFeaturedEmojiStickers(&mtproto.TLMessagesGetFeaturedEmojiStickers{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsCoveredStickerSet(featured.GetSets(), targetID) {
		t.Fatalf("featured emoji stickers=%v, want target %d despite non-emoji rows before it", featured, targetID)
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
