package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestStickerMethodsRequireAuthentication(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{}}
	cases := []struct {
		name string
		call func() (any, error)
	}{
		{"get stickers", func() (any, error) { return c.MessagesGetStickers(nil) }},
		{"get set", func() (any, error) { return c.MessagesGetStickerSet(nil) }},
		{"install", func() (any, error) { return c.MessagesInstallStickerSet(nil) }},
		{"fave", func() (any, error) { return c.MessagesFaveSticker(nil) }},
		{"create", func() (any, error) { return c.StickersCreateStickerSet(nil) }},
		{"delete", func() (any, error) { return c.StickersDeleteStickerSet(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if !nilRPCResult(result) || !sameRPCErrorCode(err, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("result=%v err=%v, want nil result and AUTH_KEY_UNREGISTERED", result, err)
			}
		})
	}
}

func TestStickerMethodsFailClosedWithoutProvider(t *testing.T) {
	uid := int64(229902)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	t.Cleanup(func() { _ = persist.Default.Set(installedStickerKey(uid), "") })
	if err := persist.Default.Set(installedStickerKey(uid), "sentinel"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		call func() (any, error)
	}{
		{"get stickers", func() (any, error) { return c.MessagesGetStickers(nil) }},
		{"get all", func() (any, error) { return c.MessagesGetAllStickers(nil) }},
		{"get set", func() (any, error) { return c.MessagesGetStickerSet(nil) }},
		{"install", func() (any, error) { return c.MessagesInstallStickerSet(nil) }},
		{"uninstall", func() (any, error) { return c.MessagesUninstallStickerSet(nil) }},
		{"reorder", func() (any, error) { return c.MessagesReorderStickerSets(nil) }},
		{"featured", func() (any, error) { return c.MessagesGetFeaturedStickers(nil) }},
		{"read featured", func() (any, error) { return c.MessagesReadFeaturedStickers(nil) }},
		{"recent", func() (any, error) { return c.MessagesGetRecentStickers(nil) }},
		{"save recent", func() (any, error) { return c.MessagesSaveRecentSticker(nil) }},
		{"clear recent", func() (any, error) { return c.MessagesClearRecentStickers(nil) }},
		{"archived", func() (any, error) { return c.MessagesGetArchivedStickers(nil) }},
		{"mask", func() (any, error) { return c.MessagesGetMaskStickers(nil) }},
		{"attached", func() (any, error) { return c.MessagesGetAttachedStickers(nil) }},
		{"faved", func() (any, error) { return c.MessagesGetFavedStickers(nil) }},
		{"fave", func() (any, error) { return c.MessagesFaveSticker(nil) }},
		{"search sets", func() (any, error) { return c.MessagesSearchStickerSets(nil) }},
		{"toggle sets", func() (any, error) { return c.MessagesToggleStickerSets(nil) }},
		{"old featured", func() (any, error) { return c.MessagesGetOldFeaturedStickers(nil) }},
		{"search emoji sets", func() (any, error) { return c.MessagesSearchEmojiStickerSets(nil) }},
		{"my stickers", func() (any, error) { return c.MessagesGetMyStickers(nil) }},
		{"search stickers", func() (any, error) { return c.MessagesSearchStickers(nil) }},
		{"create", func() (any, error) { return c.StickersCreateStickerSet(nil) }},
		{"remove", func() (any, error) { return c.StickersRemoveStickerFromSet(nil) }},
		{"position", func() (any, error) { return c.StickersChangeStickerPosition(nil) }},
		{"add", func() (any, error) { return c.StickersAddStickerToSet(nil) }},
		{"thumb", func() (any, error) { return c.StickersSetStickerSetThumb(nil) }},
		{"check short name", func() (any, error) { return c.StickersCheckShortName(nil) }},
		{"suggest short name", func() (any, error) { return c.StickersSuggestShortName(nil) }},
		{"change", func() (any, error) { return c.StickersChangeSticker(nil) }},
		{"rename", func() (any, error) { return c.StickersRenameStickerSet(nil) }},
		{"delete", func() (any, error) { return c.StickersDeleteStickerSet(nil) }},
		{"replace", func() (any, error) { return c.StickersReplaceSticker(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.call()
			if !nilRPCResult(result) || !sameRPCErrorCode(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("result=%v err=%v, want nil result and METHOD_NOT_IMPL", result, err)
			}
		})
	}
	raw, err := persist.Default.Get(installedStickerKey(uid))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "sentinel" {
		t.Fatalf("unsupported sticker methods mutated local state: %q", raw)
	}
}

func TestStickerSetIDRequiresExactAccessHash(t *testing.T) {
	set := &persist.StickerSet{ID: 42, AccessHash: 73}
	for _, accessHash := range []int64{0, 72, 74} {
		input := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID, AccessHash: accessHash}).To_InputStickerSet()
		if err := validateStickerSetAccess(input, set); !errors.Is(err, mtproto.ErrStickersetInvalid) {
			t.Errorf("access hash %d error = %v, want STICKERSET_INVALID", accessHash, err)
		}
	}
	valid := mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.ID, AccessHash: set.AccessHash}).To_InputStickerSet()
	if err := validateStickerSetAccess(valid, set); err != nil {
		t.Fatalf("valid access hash rejected: %v", err)
	}
}
