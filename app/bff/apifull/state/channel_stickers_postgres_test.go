package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestChannelEmojiStickerSetPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.Close() })

	ctx := context.Background()
	ownerID := int64(1_700_000_000 + os.Getpid()%100_000)
	channelID := ownerID + 1
	set, err := persist.CreateStickerSet(ctx, ownerID, "Channel emoji", fmt.Sprintf("channel_emoji_%d", ownerID), false, true, false, false, false, []persist.StickerDocumentInput{{ID: ownerID + 2, AccessHash: ownerID + 3, Alt: "😀"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.DeleteStickerSet(ctx, ownerID, set.ID) })
	t.Cleanup(func() { _ = domain.SetChannelEmojiStickerSetAfterAuthorization(ownerID, channelID, 0) })

	setRef := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID, AccessHash: set.AccessHash}).To_InputStickerSet()
	if err = SetChannelEmojiStickerSet(ctx, ownerID, channelID, setRef); err != nil {
		t.Fatal(err)
	}
	gotID, found, err := LoadChannelEmojiStickerSet(channelID)
	if err != nil || !found || gotID != set.ID {
		t.Fatalf("channel emoji binding: id=%d found=%v err=%v", gotID, found, err)
	}
	wrongHash := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID, AccessHash: set.AccessHash + 1}).To_InputStickerSet()
	if err = SetChannelEmojiStickerSet(ctx, ownerID, channelID, wrongHash); !errors.Is(err, mtproto.ErrStickersetInvalid) {
		t.Fatalf("wrong sticker-set access hash error = %v, want STICKERSET_INVALID", err)
	}
	missingHash := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID}).To_InputStickerSet()
	if err = SetChannelEmojiStickerSet(ctx, ownerID, channelID, missingHash); !errors.Is(err, mtproto.ErrStickersetInvalid) {
		t.Fatalf("missing sticker-set access hash error = %v, want STICKERSET_INVALID", err)
	}

	invalidSet := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID + 100, AccessHash: set.AccessHash}).To_InputStickerSet()
	if err = SetChannelEmojiStickerSet(ctx, ownerID, channelID, invalidSet); !errors.Is(err, mtproto.ErrStickersetInvalid) {
		t.Fatalf("missing sticker set error = %v, want STICKERSET_INVALID", err)
	}
	regularSet, err := persist.CreateStickerSet(ctx, ownerID, "Regular sticker", fmt.Sprintf("regular_sticker_%d", ownerID), false, false, false, false, false, []persist.StickerDocumentInput{{ID: ownerID + 4, AccessHash: ownerID + 5, Alt: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = persist.DeleteStickerSet(ctx, ownerID, regularSet.ID) })
	regularRef := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: regularSet.ID, AccessHash: regularSet.AccessHash}).To_InputStickerSet()
	if err = SetChannelEmojiStickerSet(ctx, ownerID, channelID, regularRef); !errors.Is(err, mtproto.ErrStickersetInvalid) {
		t.Fatalf("regular sticker set error = %v, want STICKERSET_INVALID", err)
	}
	if err = domain.SetChannelEmojiStickerSetAfterAuthorization(ownerID, channelID, regularSet.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("domain regular sticker set error = %v, want sql.ErrNoRows", err)
	}
	gotID, found, err = LoadChannelEmojiStickerSet(channelID)
	if err != nil || !found || gotID != set.ID {
		t.Fatalf("failed update changed channel emoji binding: id=%d found=%v err=%v", gotID, found, err)
	}

	empty := mtproto.MakeTLInputStickerSetEmpty(&mtproto.InputStickerSet{}).To_InputStickerSet()
	if err = SetChannelEmojiStickerSet(ctx, ownerID, channelID, empty); err != nil {
		t.Fatal(err)
	}
	if _, found, err = LoadChannelEmojiStickerSet(channelID); err != nil || found {
		t.Fatalf("cleared channel emoji binding: found=%v err=%v", found, err)
	}

	if err = domain.SetChannelEmojiStickerSetAfterAuthorization(ownerID, channelID, -1); err == nil {
		t.Fatal("negative sticker set ID unexpectedly succeeded")
	}
	if _, found, err = LoadChannelEmojiStickerSet(channelID); err != nil || found {
		t.Fatalf("failed transaction left channel emoji binding: found=%v err=%v", found, err)
	}
}
