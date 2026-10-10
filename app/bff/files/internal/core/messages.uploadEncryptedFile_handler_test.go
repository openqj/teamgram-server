package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	"github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
)

type encryptedMediaClient struct {
	mediaclient.MediaClient
	called bool
	owner  int64
}

func (m *encryptedMediaClient) MediaUploadEncryptedFile(_ context.Context, in *media.TLMediaUploadEncryptedFile) (*mtproto.EncryptedFile, error) {
	m.called = true
	m.owner = in.GetOwnerId()
	return mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
		Id: 901, AccessHash: 902, Size2_INT64: 7, Size2_INT32: 7, DcId: 2, KeyFingerprint: in.GetFile().GetKeyFingerprint(),
	}).To_EncryptedFile(), nil
}

func TestMessagesUploadEncryptedFileUsesMediaPostgresBoundary(t *testing.T) {
	mediaClient := &encryptedMediaClient{}
	c := &FilesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MediaClient: mediaClient}},
		MD:     &metadata.RpcMetadata{PermAuthKeyId: -77},
		Logger: logx.WithContext(context.Background()),
	}
	file := mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{Id: 5, Parts: 1, KeyFingerprint: 12}).To_InputEncryptedFile()
	got, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{
		Peer: &mtproto.InputEncryptedChat{ChatId: 8, AccessHash: 9}, File: file,
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !mediaClient.called || mediaClient.owner != -77 {
		t.Fatalf("media upload call = called %v owner %d", mediaClient.called, mediaClient.owner)
	}
	if got.GetId() != 901 || got.GetKeyFingerprint() != 12 {
		t.Fatalf("reply = %v", got)
	}
}

func TestMessagesUploadEncryptedFileRequiresAuthMetadata(t *testing.T) {
	if got, err := (&FilesCore{}).MessagesUploadEncryptedFile(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("missing metadata = (%v, %v)", got, err)
	}
}
