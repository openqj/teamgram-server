package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	dfs "github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

type channelPhotoDfsClient struct {
	dfs_client.DfsClient
	photo   *mtproto.Photo
	request *dfs.TLDfsUploadProfilePhotoFileV2
}

func (c *channelPhotoDfsClient) DfsUploadProfilePhotoFileV2(_ context.Context, in *dfs.TLDfsUploadProfilePhotoFileV2) (*mtproto.Photo, error) {
	c.request = in
	return c.photo, nil
}

func TestChannelsEditPhotoPersistsUploadedPhotoAndClear(t *testing.T) {
	owner := time.Now().UnixNano()%1_000_000_000 + 7_100_000_000
	channelID := time.Now().UnixNano()
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "channel-photo-test", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })

	dfsClient := &channelPhotoDfsClient{photo: mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         88001,
		DcId:       4,
		Sizes:      []*mtproto.PhotoSize{{Type: "x", W: 100, H: 100, Size2: 4}},
		VideoSizes: []*mtproto.VideoSize{{Type: "v", W: 100, H: 100, Size2: 8}},
	}).To_Photo()}
	core := &ApiFullCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{UserId: owner},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{DfsClient: dfsClient}},
	}
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	inputPhoto := mtproto.MakeTLInputChatUploadedPhoto(&mtproto.InputChatPhoto{
		File: &mtproto.InputFile{Id_INT64: 11, Parts: 1, Name: "channel.jpg"},
	}).To_InputChatPhoto()
	updates, err := core.ChannelsEditPhoto(&mtproto.TLChannelsEditPhoto{Channel: inputChannel, Photo: inputPhoto})
	if err != nil || updates == nil || len(updates.GetChats()) != 1 {
		t.Fatalf("upload photo: updates=%+v err=%v", updates, err)
	}
	if dfsClient.request == nil || dfsClient.request.GetCreator() != owner || dfsClient.request.GetFile().GetName() != "channel.jpg" {
		t.Fatalf("DFS request=%+v", dfsClient.request)
	}
	photo := updates.GetChats()[0].GetPhoto()
	if photo == nil || photo.GetPredicateName() != mtproto.Predicate_chatPhoto || photo.GetPhotoId() != 88001 || photo.GetDcId() != 4 || !photo.GetHasVideo() {
		t.Fatalf("uploaded photo in Updates=%+v", photo)
	}
	stored, ok, err := domain.LoadChannel(channelID)
	if err != nil || !ok || stored.PhotoID != 88001 || stored.PhotoDCID != 4 || !stored.PhotoHasVideo {
		t.Fatalf("stored photo=%+v ok=%v err=%v", stored, ok, err)
	}

	empty := mtproto.MakeTLInputChatPhotoEmpty(nil).To_InputChatPhoto()
	updates, err = core.ChannelsEditPhoto(&mtproto.TLChannelsEditPhoto{Channel: inputChannel, Photo: empty})
	if err != nil || updates == nil || len(updates.GetChats()) != 1 {
		t.Fatalf("clear photo: updates=%+v err=%v", updates, err)
	}
	photo = updates.GetChats()[0].GetPhoto()
	if photo == nil || photo.GetPredicateName() != mtproto.Predicate_chatPhotoEmpty || photo.GetPhotoId() != 0 || photo.GetDcId() != 0 {
		t.Fatalf("cleared photo in Updates=%+v", photo)
	}
	stored, ok, err = domain.LoadChannel(channelID)
	if err != nil || !ok || stored.PhotoID != 0 || stored.PhotoDCID != 0 || stored.PhotoHasVideo {
		t.Fatalf("stored cleared photo=%+v ok=%v err=%v", stored, ok, err)
	}
}
