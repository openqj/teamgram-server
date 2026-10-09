package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	"github.com/teamgram/teamgram-server/app/service/media/media/hashrpc"
	"github.com/zeromicro/go-zero/core/logx"
)

type documentByHashMediaClient struct {
	mediaclient.MediaClient
	request  *hashrpc.DocumentHashRequest
	document *mtproto.Document
	err      error
}

func (c *documentByHashMediaClient) MediaGetDocumentByHash(_ context.Context, in *hashrpc.DocumentHashRequest) (*mtproto.Document, error) {
	c.request = in
	return c.document, c.err
}

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

func TestMessagesGetDocumentByHashUsesMediaProvider(t *testing.T) {
	want := mtproto.MakeTLDocumentEmpty(&mtproto.Document{Id: 41}).To_Document()
	media := &documentByHashMediaClient{document: want}
	request := &mtproto.TLMessagesGetDocumentByHash{
		Sha256:      make([]byte, 32),
		Size2_INT64: 12,
		MimeType:    "application/octet-stream",
	}
	c := &FilesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MediaClient: media}},
		Logger: logx.WithContext(context.Background()),
	}

	got, err := c.MessagesGetDocumentByHash(request)
	if err != nil {
		t.Fatalf("MessagesGetDocumentByHash() error = %v", err)
	}
	if got != want {
		t.Fatalf("MessagesGetDocumentByHash() = %v, want %v", got, want)
	}
	if media.request == nil || media.request.GetSize() != 12 || media.request.GetMimeType() != request.GetMimeType() {
		t.Fatalf("media lookup request = %v", media.request)
	}
}

func TestMessagesGetDocumentByHashAcceptsInt32SizeConstructor(t *testing.T) {
	media := &documentByHashMediaClient{document: mtproto.MakeTLDocumentEmpty(&mtproto.Document{}).To_Document()}
	c := &FilesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MediaClient: media}},
		Logger: logx.WithContext(context.Background()),
	}
	_, err := c.MessagesGetDocumentByHash(&mtproto.TLMessagesGetDocumentByHash{
		Sha256:      make([]byte, 32),
		Size2_INT32: 12,
		MimeType:    "application/octet-stream",
	})
	if err != nil {
		t.Fatalf("MessagesGetDocumentByHash() error = %v", err)
	}
	if media.request == nil || media.request.GetSize() != 12 {
		t.Fatalf("media lookup request = %v, want legacy int32 size 12", media.request)
	}
}
