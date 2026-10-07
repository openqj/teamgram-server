package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	dfsclient "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/zeromicro/go-zero/core/logx"
)

type saveFilePartDFS struct {
	dfsclient.DfsClient
	got *dfs.TLDfsWriteFilePartData
}

func (f *saveFilePartDFS) DfsWriteFilePartData(_ context.Context, in *dfs.TLDfsWriteFilePartData) (*mtproto.Bool, error) {
	f.got = in
	return mtproto.BoolTrue, nil
}

func newSaveFilePartCore(client dfsclient.DfsClient) *FilesCore {
	return &FilesCore{
		svcCtx: &svc.ServiceContext{
			Config: config.Config{},
			Dao:    &dao.Dao{DfsClient: client},
		},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 99},
	}
}

func TestUploadSaveFilePartValidatesEnvelope(t *testing.T) {
	client := &saveFilePartDFS{}
	core := newSaveFilePartCore(client)
	for name, in := range map[string]*mtproto.TLUploadSaveFilePart{
		"nil request":       nil,
		"missing file":      {FilePart: 0, Bytes: []byte{1}},
		"negative part":     {FileId: 1, FilePart: -1, Bytes: []byte{1}},
		"empty part":        {FileId: 1, FilePart: 0},
		"part out of range": {FileId: 1, FilePart: 3000, Bytes: []byte{1}},
		"oversized part":    {FileId: 1, FilePart: 0, Bytes: make([]byte, 512*1024+1)},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := core.UploadSaveFilePart(in)
			if got != nil || err == nil {
				t.Fatalf("UploadSaveFilePart() = (%v, %v), want validation error", got, err)
			}
			if client.got != nil {
				t.Fatal("invalid request reached DFS")
			}
		})
	}
}

func TestUploadSaveFilePartForwardsAuthenticatedPart(t *testing.T) {
	client := &saveFilePartDFS{}
	core := newSaveFilePartCore(client)
	got, err := core.UploadSaveFilePart(&mtproto.TLUploadSaveFilePart{FileId: 7, FilePart: 2, Bytes: []byte{1, 2}})
	if got != mtproto.BoolTrue || err != nil {
		t.Fatalf("UploadSaveFilePart() = (%v, %v), want BoolTrue", got, err)
	}
	if client.got == nil || client.got.GetBig() || client.got.GetCreator() != 99 || client.got.GetFileId() != 7 || client.got.GetFilePart() != 2 {
		t.Fatalf("DFS request = %#v, want authenticated small-file envelope", client.got)
	}
}

func TestUploadSaveFilePartRejectsMissingAuth(t *testing.T) {
	if got, err := (&FilesCore{}).UploadSaveFilePart(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("UploadSaveFilePart() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
