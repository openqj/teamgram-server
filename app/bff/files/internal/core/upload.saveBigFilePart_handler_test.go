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

type saveBigPartDFS struct {
	dfsclient.DfsClient
	got *dfs.TLDfsWriteFilePartData
	err error
}

func (f *saveBigPartDFS) DfsWriteFilePartData(_ context.Context, in *dfs.TLDfsWriteFilePartData) (*mtproto.Bool, error) {
	f.got = in
	if f.err != nil {
		return nil, f.err
	}
	return mtproto.BoolTrue, nil
}

func newSaveBigPartCore(client dfsclient.DfsClient) *FilesCore {
	return &FilesCore{
		svcCtx: &svc.ServiceContext{Config: config.Config{}, Dao: &dao.Dao{DfsClient: client}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
	}
}

func TestUploadSaveBigFilePartValidatesEnvelope(t *testing.T) {
	client := &saveBigPartDFS{}
	core := newSaveBigPartCore(client)
	for name, in := range map[string]*mtproto.TLUploadSaveBigFilePart{
		"nil request":         nil,
		"missing file":        {FilePart: 0, FileTotalParts: 1, Bytes: []byte{1}},
		"negative part":       {FileId: 1, FilePart: -1, FileTotalParts: 1, Bytes: []byte{1}},
		"invalid total":       {FileId: 1, FilePart: 1, FileTotalParts: 1, Bytes: []byte{1}},
		"total exceeds limit": {FileId: 1, FilePart: 0, FileTotalParts: 3001, Bytes: []byte{1}},
		"empty part":          {FileId: 1, FilePart: 0, FileTotalParts: 1},
		"part out of range":   {FileId: 1, FilePart: 3000, FileTotalParts: 3001, Bytes: []byte{1}},
		"oversized part":      {FileId: 1, FilePart: 0, FileTotalParts: 1, Bytes: make([]byte, 512*1024+1)},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := core.UploadSaveBigFilePart(in)
			if got != nil || err == nil {
				t.Fatalf("UploadSaveBigFilePart() = (%v, %v), want validation error", got, err)
			}
			if client.got != nil {
				t.Fatal("invalid request reached DFS")
			}
		})
	}
}

func TestUploadSaveBigFilePartForwardsBigPartAndProviderError(t *testing.T) {
	providerErr := errors.New("dfs unavailable")
	client := &saveBigPartDFS{err: providerErr}
	core := newSaveBigPartCore(client)
	in := &mtproto.TLUploadSaveBigFilePart{FileId: 7, FilePart: 2, FileTotalParts: 4, Bytes: []byte{1, 2}}
	got, err := core.UploadSaveBigFilePart(in)
	if got != nil || !errors.Is(err, providerErr) {
		t.Fatalf("UploadSaveBigFilePart() = (%v, %v), want provider error", got, err)
	}
	if client.got == nil || !client.got.GetBig() || client.got.GetCreator() != 99 || client.got.GetFileId() != 7 || client.got.GetFilePart() != 2 || client.got.GetFileTotalParts().GetValue() != 4 {
		t.Fatalf("DFS request = %#v, want authenticated big-file envelope", client.got)
	}
}
