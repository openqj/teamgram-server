package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesUploadMediaRejectsMissingRequest(t *testing.T) {
	c := &FilesCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
	if got, err := c.MessagesUploadMedia(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MessagesUploadMedia(nil) = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
	}
	if got, err := c.MessagesUploadMedia(&mtproto.TLMessagesUploadMedia{}); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MessagesUploadMedia(empty) = (%v, %v), want (nil, INPUT_REQUEST_INVALID)", got, err)
	}
}

func TestMessagesUploadMediaFailsClosedForRemoteAndUnsupportedInputs(t *testing.T) {
	c := &FilesCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
	for _, predicate := range []string{
		mtproto.Predicate_inputMediaPhotoExternal,
		mtproto.Predicate_inputMediaDocumentExternal,
		mtproto.Predicate_inputMediaGame,
		mtproto.Predicate_inputMediaInvoice,
	} {
		t.Run(predicate, func(t *testing.T) {
			got, err := c.makeMediaByInputMedia(&mtproto.InputMedia{PredicateName: predicate})
			if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("makeMediaByInputMedia(%s) = (%v, %v), want (nil, METHOD_NOT_IMPL)", predicate, got, err)
			}
		})
	}
}

func TestMessagesUploadMediaRejectsUnknownDice(t *testing.T) {
	c := &FilesCore{ctx: context.Background(), Logger: logx.WithContext(context.Background())}
	got, err := c.makeMediaByInputMedia(&mtproto.InputMedia{
		PredicateName: mtproto.Predicate_inputMediaDice,
		Emoticon:      "🙂",
	})
	if got != nil || !errors.Is(err, mtproto.ErrMediaInvalid) {
		t.Fatalf("makeMediaByInputMedia(unknown dice) = (%v, %v), want (nil, MEDIA_INVALID)", got, err)
	}
}
