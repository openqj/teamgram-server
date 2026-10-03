package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	dfs "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

type ringtoneUploadDfsClient struct {
	dfs_client.DfsClient
	doc *mtproto.Document
	err error
	got *dfs.TLDfsUploadRingtoneFile
}

func (c *ringtoneUploadDfsClient) DfsUploadRingtoneFile(_ context.Context, in *dfs.TLDfsUploadRingtoneFile) (*mtproto.Document, error) {
	c.got = in
	return c.doc, c.err
}

func ringtoneUploadCore(client dfs_client.DfsClient) *ApiFullCore {
	return &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: 7001},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			DfsClient: client,
		}},
	}
}

func ringtoneUploadRequest() *mtproto.TLAccountUploadRingtone {
	return &mtproto.TLAccountUploadRingtone{
		File:     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 11, Parts: 1, Name: "tone.mp3"}).To_InputFile(),
		FileName: "tone.mp3",
		MimeType: "",
	}
}

func TestAccountUploadRingtoneUsesDFS(t *testing.T) {
	dfsClient := &ringtoneUploadDfsClient{
		doc: mtproto.MakeTLDocument(&mtproto.Document{Id: 42, AccessHash: 99}).To_Document(),
	}
	got, err := ringtoneUploadCore(dfsClient).AccountUploadRingtone(ringtoneUploadRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.GetId() != 42 {
		t.Fatalf("document = %#v", got)
	}
	if dfsClient.got == nil || dfsClient.got.GetCreator() != 7001 || dfsClient.got.GetFileName() != "tone.mp3" || dfsClient.got.GetMimeType() != "audio/mpeg" {
		t.Fatalf("DFS request = %#v", dfsClient.got)
	}
}

func TestAccountUploadRingtonePropagatesDFSError(t *testing.T) {
	want := errors.New("dfs unavailable")
	dfsClient := &ringtoneUploadDfsClient{err: want}
	got, err := ringtoneUploadCore(dfsClient).AccountUploadRingtone(ringtoneUploadRequest())
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("result=(%#v, %v), want DFS error", got, err)
	}
}

func TestAccountUploadRingtoneFailsClosedWithoutDFS(t *testing.T) {
	got, err := ringtoneUploadCore(nil).AccountUploadRingtone(ringtoneUploadRequest())
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("result=(%#v, %v), want METHOD_NOT_IMPL", got, err)
	}
}

func TestAccountUploadRingtoneRejectsInvalidInput(t *testing.T) {
	dfsClient := &ringtoneUploadDfsClient{}
	for _, in := range []*mtproto.TLAccountUploadRingtone{nil, {}, {FileName: "tone.mp3"}} {
		got, err := ringtoneUploadCore(dfsClient).AccountUploadRingtone(in)
		if got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
			t.Fatalf("input=%#v result=(%#v, %v), want input error", in, got, err)
		}
	}
	if dfsClient.got != nil {
		t.Fatalf("invalid request reached DFS: %#v", dfsClient.got)
	}
}
