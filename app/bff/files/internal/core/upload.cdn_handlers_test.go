package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

func newCDNCore() *FilesCore {
	return &FilesCore{Logger: logx.WithContext(context.Background())}
}

func TestUploadGetCdnFileValidatesInputBeforeFailingClosed(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLUploadGetCdnFile
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty file token", in: &mtproto.TLUploadGetCdnFile{Limit: 1}, want: mtproto.ErrFileTokenInvalid},
		{name: "negative offset", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Offset_INT64: -1, Limit: 1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative legacy offset", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Offset_INT32: -1, Limit: 1}, want: mtproto.ErrOffsetInvalid},
		{name: "zero limit", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}}, want: mtproto.ErrLimitInvalid},
		{name: "provider unavailable", in: &mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Limit: 1}, want: mtproto.ErrCdnMethodInvalid},
	}

	core := newCDNCore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.UploadGetCdnFile(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("UploadGetCdnFile() = (%v, %v), want (%v)", got, err, tt.want)
			}
		})
	}
}

func TestUploadGetCdnFileHashesValidatesInputBeforeFailingClosed(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLUploadGetCdnFileHashes
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty file token", in: &mtproto.TLUploadGetCdnFileHashes{}, want: mtproto.ErrFileTokenInvalid},
		{name: "negative offset", in: &mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}, Offset_INT64: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "negative legacy offset", in: &mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}, Offset_INT32: -1}, want: mtproto.ErrOffsetInvalid},
		{name: "provider unavailable", in: &mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}}, want: mtproto.ErrCdnMethodInvalid},
	}

	core := newCDNCore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.UploadGetCdnFileHashes(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("UploadGetCdnFileHashes() = (%v, %v), want (%v)", got, err, tt.want)
			}
		})
	}
}

func TestUploadReuploadCdnFileValidatesInputBeforeFailingClosed(t *testing.T) {
	tests := []struct {
		name string
		in   *mtproto.TLUploadReuploadCdnFile
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "empty file token", in: &mtproto.TLUploadReuploadCdnFile{RequestToken: []byte{2}}, want: mtproto.ErrFileTokenInvalid},
		{name: "empty request token", in: &mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}}, want: mtproto.ErrTokenInvalid},
		{name: "provider unavailable", in: &mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}, RequestToken: []byte{2}}, want: mtproto.ErrCdnMethodInvalid},
	}

	core := newCDNCore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.UploadReuploadCdnFile(tt.in)
			if got != nil || !errors.Is(err, tt.want) {
				t.Fatalf("UploadReuploadCdnFile() = (%v, %v), want (%v)", got, err, tt.want)
			}
		})
	}
}

func TestMessagesGetDocumentByHashRejectsNilRequest(t *testing.T) {
	core := &FilesCore{}
	if got, err := core.MessagesGetDocumentByHash(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("MessagesGetDocumentByHash() = (%v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}

func TestCDNMethodsDoNotPanicWithoutLogger(t *testing.T) {
	if got, err := (*FilesCore)(nil).UploadGetCdnFile(&mtproto.TLUploadGetCdnFile{FileToken: []byte{1}, Limit: 1}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("UploadGetCdnFile() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
	if got, err := (&FilesCore{}).UploadGetCdnFileHashes(&mtproto.TLUploadGetCdnFileHashes{FileToken: []byte{1}}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("UploadGetCdnFileHashes() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
	if got, err := (&FilesCore{}).UploadReuploadCdnFile(&mtproto.TLUploadReuploadCdnFile{FileToken: []byte{1}, RequestToken: []byte{2}}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("UploadReuploadCdnFile() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
}

func TestHelpGetCdnConfigFailsClosedWithoutProvider(t *testing.T) {
	core := newCDNCore()
	if got, err := core.HelpGetCdnConfig(&mtproto.TLHelpGetCdnConfig{}); got != nil || !errors.Is(err, mtproto.ErrCdnMethodInvalid) {
		t.Fatalf("HelpGetCdnConfig() = (%v, %v), want CDN_METHOD_INVALID", got, err)
	}
}
