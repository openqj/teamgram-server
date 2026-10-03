package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

func TestStickerUploadMediaValidatesFile(t *testing.T) {
	if got, err := stickerUploadMedia(nil); got != nil || !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("nil request: got=%v err=%v", got, err)
	}
	if got, err := stickerUploadMedia(&media.TLMediaUploadStickerFile{}); got != nil || !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("missing file: got=%v err=%v", got, err)
	}
	if got, err := stickerUploadMedia(&media.TLMediaUploadStickerFile{
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 7, Parts: 1, Name: "sticker.webp"}).To_InputFile(),
	}); got != nil || !errors.Is(err, mtproto.ErrDocumentInvalid) {
		t.Fatalf("missing sticker attribute: got=%v err=%v", got, err)
	}
}

func TestStickerUploadMediaPreservesStickerInput(t *testing.T) {
	attribute := mtproto.MakeTLDocumentAttributeSticker(&mtproto.DocumentAttribute{Alt: "😀"}).To_DocumentAttribute()
	input := &media.TLMediaUploadStickerFile{
		OwnerId:                  42,
		File:                     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 7, Parts: 1, Name: "ignored.webp"}).To_InputFile(),
		MimeType:                 "image/webp",
		FileName:                 "sticker.webp",
		DocumentAttributeSticker: attribute,
	}

	got, err := stickerUploadMedia(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetFile() == nil || got.GetFile().GetName() != "sticker.webp" {
		t.Fatalf("file name = %v", got.GetFile())
	}
	if got.GetMimeType() != "image/webp" || len(got.GetAttributes()) != 1 || got.GetAttributes()[0] != attribute {
		t.Fatalf("media fields = %+v", got)
	}
}
