package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesGetDocumentByHashFailsClosedWithoutProvider(t *testing.T) {
	request := &mtproto.TLMessagesGetDocumentByHash{
		Sha256:      make([]byte, 32),
		Size2_INT64: 1,
		MimeType:    "application/octet-stream",
	}

	c := &FilesCore{Logger: logx.WithContext(context.Background())}
	document, err := c.MessagesGetDocumentByHash(request)
	if document != nil {
		t.Fatalf("MessagesGetDocumentByHash() document = %#v, want nil", document)
	}
	if !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("MessagesGetDocumentByHash() error = %v, want METHOD_NOT_IMPL", err)
	}
}

func TestMessagesGetDocumentByHashRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLMessagesGetDocumentByHash
	}{
		{name: "nil hash", in: &mtproto.TLMessagesGetDocumentByHash{Size2_INT64: 1, MimeType: "application/octet-stream"}},
		{name: "empty mime type", in: &mtproto.TLMessagesGetDocumentByHash{Sha256: make([]byte, 32), Size2_INT64: 1}},
		{name: "negative size", in: &mtproto.TLMessagesGetDocumentByHash{Sha256: make([]byte, 32), Size2_INT64: -1, MimeType: "application/octet-stream"}},
	}

	c := &FilesCore{Logger: logx.WithContext(context.Background())}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := c.MessagesGetDocumentByHash(tt.in)
			if document != nil {
				t.Fatalf("MessagesGetDocumentByHash() document = %#v, want nil", document)
			}
			if !errors.Is(err, mtproto.ErrDocumentInvalid) {
				t.Fatalf("MessagesGetDocumentByHash() error = %v, want DOCUMENT_INVALID", err)
			}
		})
	}
}
