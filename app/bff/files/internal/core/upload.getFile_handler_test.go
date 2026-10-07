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

type getFileDFS struct {
	dfsclient.DfsClient
}

func (getFileDFS) DfsDownloadFile(context.Context, *dfs.TLDfsDownloadFile) (*mtproto.Upload_File, error) {
	return nil, errors.New("unexpected DFS call")
}

func newGetFileCore() *FilesCore {
	return &FilesCore{
		svcCtx: &svc.ServiceContext{
			Config: config.Config{},
			Dao:    &dao.Dao{DfsClient: getFileDFS{}},
		},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{PermAuthKeyId: 99},
	}
}

func TestUploadGetFileValidatesRequestBeforeDFS(t *testing.T) {
	core := newGetFileCore()
	validLocation := mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
		Id: 1,
	}).To_InputFileLocation()
	tests := map[string]*mtproto.TLUploadGetFile{
		"nil request":            nil,
		"nil location":           {Limit: 1},
		"negative offset":        {Location: validLocation, Offset_INT64: -1, Limit: 1},
		"negative legacy offset": {Location: validLocation, Offset_INT32: -1, Limit: 1},
		"zero limit":             {Location: validLocation, Limit: 0},
		"negative limit":         {Location: validLocation, Limit: -1},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := core.UploadGetFile(in)
			if got != nil || err == nil {
				t.Fatalf("UploadGetFile() = (%v, %v), want validation error", got, err)
			}
		})
	}
}

func TestUploadGetFileRejectsMissingAuthAndDependencies(t *testing.T) {
	if got, err := (&FilesCore{}).UploadGetFile(nil); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("UploadGetFile() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
	core := newGetFileCore()
	core.MD.PermAuthKeyId = 0
	if got, err := core.UploadGetFile(&mtproto.TLUploadGetFile{}); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("UploadGetFile() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
