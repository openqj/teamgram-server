package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/files/internal/svc"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/zeromicro/go-zero/core/logx"
)

type fileHashesDfsClient struct {
	dfs_client.DfsClient
	data  []byte
	calls []int64
}

func (c *fileHashesDfsClient) DfsDownloadFile(_ context.Context, in *dfs.TLDfsDownloadFile) (*mtproto.Upload_File, error) {
	c.calls = append(c.calls, in.GetOffset())
	offset := in.GetOffset()
	if offset >= int64(len(c.data)) {
		return &mtproto.Upload_File{}, nil
	}
	end := offset + int64(in.GetLimit())
	if end > int64(len(c.data)) {
		end = int64(len(c.data))
	}
	return &mtproto.Upload_File{Bytes: c.data[offset:end]}, nil
}

func fileHashesCore(client dfs_client.DfsClient) *FilesCore {
	return &FilesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{DfsClient: client}},
		Logger: logx.WithContext(context.Background()),
	}
}

func TestUploadGetFileHashesUsesDFSChunks(t *testing.T) {
	data := make([]byte, fileHashPartSize+3)
	for i := range data {
		data[i] = byte(i % 251)
	}
	client := &fileHashesDfsClient{data: data}
	got, err := fileHashesCore(client).UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{
		Location: &mtproto.InputFileLocation{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetDatas()) != 2 {
		t.Fatalf("hash count = %d, want 2", len(got.GetDatas()))
	}
	if len(client.calls) != 2 || client.calls[0] != 0 || client.calls[1] != fileHashPartSize {
		t.Fatalf("DFS offsets = %v, want [0 %d]", client.calls, fileHashPartSize)
	}
	for i, want := range [][]byte{data[:fileHashPartSize], data[fileHashPartSize:]} {
		sum := sha256.Sum256(want)
		hash := got.GetDatas()[i]
		offset := hash.GetOffset_INT64()
		if offset == 0 {
			offset = int64(hash.GetOffset_INT32())
		}
		if offset != int64(i*fileHashPartSize) || hash.GetLimit() != int32(len(want)) {
			t.Fatalf("hash[%d] range = (%d, %d), want (%d, %d)", i, offset, hash.GetLimit(), i*fileHashPartSize, len(want))
		}
		if string(hash.GetHash()) != string(sum[:]) {
			t.Fatalf("hash[%d] digest mismatch", i)
		}
	}
}

func TestUploadGetFileHashesRejectsInvalidLocationAndOffset(t *testing.T) {
	c := fileHashesCore(&fileHashesDfsClient{})
	if got, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{}); got != nil || !errors.Is(err, mtproto.ErrLocationInvalid) {
		t.Fatalf("empty location result=(%#v, %v), want LOCATION_INVALID", got, err)
	}
	if got, err := c.UploadGetFileHashes(&mtproto.TLUploadGetFileHashes{Location: &mtproto.InputFileLocation{}, Offset_INT64: -1}); got != nil || !errors.Is(err, mtproto.ErrOffsetInvalid) {
		t.Fatalf("negative offset result=(%#v, %v), want OFFSET_INVALID", got, err)
	}
}
