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
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

type storiesMediaDfsClient struct {
	dfs_client.DfsClient
	photo    *mtproto.Photo
	document *mtproto.Document
}

func (c *storiesMediaDfsClient) DfsUploadPhotoFileV2(_ context.Context, _ *dfs.TLDfsUploadPhotoFileV2) (*mtproto.Photo, error) {
	return c.photo, nil
}

func (c *storiesMediaDfsClient) DfsUploadDocumentFileV2(_ context.Context, _ *dfs.TLDfsUploadDocumentFileV2) (*mtproto.Document, error) {
	return c.document, nil
}

func TestUploadStoryMediaRejectsNonPositiveProviderIDs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  *mtproto.InputMedia
		client *storiesMediaDfsClient
	}{
		{
			name: "photo",
			input: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
				File: &mtproto.InputFile{Id_INT64: 51, Parts: 1, Name: "story.jpg"},
			}).To_InputMedia(),
			client: &storiesMediaDfsClient{
				photo: mtproto.MakeTLPhoto(&mtproto.Photo{Id: -8001, AccessHash: 9001}).To_Photo(),
			},
		},
		{
			name: "document",
			input: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
				File:     &mtproto.InputFile{Id_INT64: 52, Parts: 1, Name: "story.bin"},
				MimeType: "application/octet-stream",
			}).To_InputMedia(),
			client: &storiesMediaDfsClient{
				document: mtproto.MakeTLDocument(&mtproto.Document{Id: -8002, AccessHash: 9002}).To_Document(),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := &ApiFullCore{
				ctx:    context.Background(),
				MD:     &metadata.RpcMetadata{UserId: 81030},
				svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{DfsClient: tc.client}},
			}
			got, err := core.uploadStoryMedia(81030, tc.input)
			if got != nil || !errors.Is(err, mtproto.ErrMediaInvalid) {
				t.Fatalf("result=(%#v, %v), want MEDIA_INVALID", got, err)
			}
		})
	}
}
