package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
)

func mediaValidationCore() *MediaCore {
	return &MediaCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
}

func TestMediaUploadHandlersRejectMalformedEnvelopes(t *testing.T) {
	c := mediaValidationCore()
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{name: "photo nil", call: func() error { _, err := c.MediaUploadPhotoFile(nil); return err }, want: mtproto.ErrMediaInvalid},
		{name: "profile photo nil", call: func() error { _, err := c.MediaUploadProfilePhotoFile(nil); return err }, want: mtproto.ErrMediaInvalid},
		{name: "encrypted nil", call: func() error { _, err := c.MediaUploadEncryptedFile(nil); return err }, want: mtproto.ErrMediaInvalid},
		{name: "document media nil", call: func() error { _, err := c.MediaUploadedDocumentMedia(nil); return err }, want: mtproto.ErrMediaInvalid},
		{name: "sticker missing owner", call: func() error { _, err := c.MediaUploadStickerFile(&media.TLMediaUploadStickerFile{}); return err }, want: mtproto.ErrMediaInvalid},
		{name: "ringtone nil", call: func() error { _, err := c.MediaUploadRingtoneFile(nil); return err }, want: mtproto.ErrInputRequestInvalid},
		{name: "theme nil", call: func() error { _, err := c.MediaUploadThemeFile(nil); return err }, want: mtproto.ErrThemeFileInvalid},
		{name: "wallpaper nil", call: func() error { _, err := c.MediaUploadWallPaperFile(nil); return err }, want: mtproto.ErrWallpaperFileInvalid},
		{name: "profile photo lookup nil", call: func() error { _, err := c.MediaUploadedProfilePhoto(nil); return err }, want: mtproto.ErrMediaInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
