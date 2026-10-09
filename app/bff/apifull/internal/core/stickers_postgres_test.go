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

func TestStickerPostgresRoundTrip(t *testing.T) {
	if !persist.StickerProviderReady() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	uid := int64(880000000 + os.Getpid()%100000)
	short := fmt.Sprintf("pg18_sticker_%d", uid)
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: uid}}
	created, err := c.StickersCreateStickerSet(&mtproto.TLStickersCreateStickerSet{
		Title: "PostgreSQL 18 stickers", ShortName: short, Emojis: true,
		Stickers: []*mtproto.InputStickerSetItem{mtproto.MakeTLInputStickerSetItem(&mtproto.InputStickerSetItem{
			Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: uid + 1, AccessHash: uid + 2}).To_InputDocument(), Emoji: "😀",
		}).To_InputStickerSetItem(), mtproto.MakeTLInputStickerSetItem(&mtproto.InputStickerSetItem{
			Document: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: uid + 3, AccessHash: uid + 4}).To_InputDocument(), Emoji: "😎",
		}).To_InputStickerSetItem()},
	})
	if err != nil {
		t.Fatal(err)
	}
	set := created.GetSet()
	t.Cleanup(func() { _ = persist.DeleteStickerSet(context.Background(), uid, set.GetId()) })
	if set.GetShortName() != short || len(created.GetDocuments()) != 2 {
		t.Fatalf("created set=%v", created)
	}
	got, err := c.MessagesGetStickerSet(&mtproto.TLMessagesGetStickerSet{Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.GetId(), AccessHash: set.GetAccessHash()}).To_InputStickerSet()})
	if err != nil || len(got.GetDocuments()) != 2 {
		t.Fatalf("get set=%v err=%v", got, err)
	}
	attached, err := c.MessagesGetAttachedStickers(&mtproto.TLMessagesGetAttachedStickers{Media: mtproto.MakeTLInputStickeredMediaDocument(&mtproto.InputStickeredMedia{Id_INPUTDOCUMENT: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: got.GetDocuments()[0].GetId(), AccessHash: got.GetDocuments()[0].GetAccessHash()}).To_InputDocument()}).To_InputStickeredMedia()})
	if err != nil || len(attached.GetDatas()) != 1 || attached.GetDatas()[0].GetSet().GetId() != set.GetId() {
		t.Fatalf("attached=%v err=%v", attached, err)
	}
	first, second := got.GetDocuments()[0], got.GetDocuments()[1]
	moved, err := c.StickersChangeStickerPosition(&mtproto.TLStickersChangeStickerPosition{
		Sticker:  mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: first.GetId(), AccessHash: first.GetAccessHash()}).To_InputDocument(),
		Position: 1,
	})
	if err != nil || len(moved.GetDocuments()) != 2 || moved.GetDocuments()[0].GetId() != second.GetId() || moved.GetDocuments()[1].GetId() != first.GetId() {
		t.Fatalf("move sticker result=%v err=%v", moved, err)
	}
	if _, err = c.MessagesInstallStickerSet(&mtproto.TLMessagesInstallStickerSet{Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.GetId(), AccessHash: set.GetAccessHash()}).To_InputStickerSet()}); err != nil {
		t.Fatal(err)
	}
	doc := got.GetDocuments()[0]
	if _, err = c.MessagesSaveRecentSticker(&mtproto.TLMessagesSaveRecentSticker{Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: doc.GetId(), AccessHash: doc.GetAccessHash()}).To_InputDocument()}); err != nil {
		t.Fatal(err)
	}
	recent, err := c.MessagesGetRecentStickers(&mtproto.TLMessagesGetRecentStickers{})
	if err != nil || len(recent.GetStickers()) != 1 {
		t.Fatalf("recent=%v err=%v", recent, err)
	}
	if _, err = c.MessagesFaveSticker(&mtproto.TLMessagesFaveSticker{Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{Id: doc.GetId(), AccessHash: doc.GetAccessHash()}).To_InputDocument()}); err != nil {
		t.Fatal(err)
	}
	faved, err := c.MessagesGetFavedStickers(&mtproto.TLMessagesGetFavedStickers{})
	if err != nil || len(faved.GetStickers()) != 1 {
		t.Fatalf("faved=%v err=%v", faved, err)
	}
	if _, err = c.StickersRenameStickerSet(&mtproto.TLStickersRenameStickerSet{Stickerset: mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: set.GetId(), AccessHash: set.GetAccessHash()}).To_InputStickerSet(), Title: "renamed"}); err != nil {
		t.Fatal(err)
	}
}
