package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type photoMutationUserClient struct {
	userclient.UserClient
	immutable *mtproto.ImmutableUser
}

func (c *photoMutationUserClient) UserUpdateProfilePhoto(_ context.Context, in *userpb.TLUserUpdateProfilePhoto) (*mtproto.Int64, error) {
	return &mtproto.Int64{V: in.GetId()}, nil
}

func (c *photoMutationUserClient) UserDeleteProfilePhotos(_ context.Context, _ *userpb.TLUserDeleteProfilePhotos) (*mtproto.Int64, error) {
	return &mtproto.Int64{}, nil
}

func (c *photoMutationUserClient) UserGetImmutableUser(_ context.Context, _ *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.immutable, nil
}

type photoMutationSyncClient struct {
	syncclient.SyncClient
	err      error
	requests int
}

func (c *photoMutationSyncClient) SyncPushUpdates(_ context.Context, _ *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	c.requests++
	return mtproto.EmptyVoid, c.err
}

func TestPhotosUpdateProfilePhotoReturnsSyncFailure(t *testing.T) {
	syncErr := errors.New("sync publish failed")
	users := newPhotoMutationUserClient()
	media := &profilePhotosMediaClient{photos: map[int64]*mtproto.Photo{
		88: mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: 88}).To_Photo(),
	}}
	syncer := &photoMutationSyncClient{err: syncErr}
	core := newPhotoMutationCore(users, media, syncer)

	got, err := core.PhotosUpdateProfilePhoto(&mtproto.TLPhotosUpdateProfilePhoto{
		Id: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 88}).To_InputPhoto(),
	})
	if got != nil || !errors.Is(err, syncErr) {
		t.Fatalf("PhotosUpdateProfilePhoto() = (%v, %v), want propagated sync error", got, err)
	}
	if syncer.requests != 1 {
		t.Fatalf("sync requests = %d, want 1", syncer.requests)
	}
}

func TestPhotosDeletePhotosReturnsSyncFailure(t *testing.T) {
	syncErr := errors.New("sync publish failed")
	users := newPhotoMutationUserClient()
	syncer := &photoMutationSyncClient{err: syncErr}
	core := newPhotoMutationCore(users, &profilePhotosMediaClient{}, syncer)

	got, err := core.PhotosDeletePhotos(&mtproto.TLPhotosDeletePhotos{
		Id: []*mtproto.InputPhoto{mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 88}).To_InputPhoto()},
	})
	if got != nil || !errors.Is(err, syncErr) {
		t.Fatalf("PhotosDeletePhotos() = (%v, %v), want propagated sync error", got, err)
	}
	if syncer.requests != 1 {
		t.Fatalf("sync requests = %d, want 1", syncer.requests)
	}
}

func newPhotoMutationUserClient() *photoMutationUserClient {
	return &photoMutationUserClient{immutable: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{Id: 42}).To_UserData(),
	}).To_ImmutableUser()}
}

func newPhotoMutationCore(users *photoMutationUserClient, media *profilePhotosMediaClient, syncer *photoMutationSyncClient) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient:  users,
			MediaClient: media,
			SyncClient:  syncer,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}
