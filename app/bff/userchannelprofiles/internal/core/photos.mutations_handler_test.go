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
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
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
	nilReply bool
	requests int
}

func (c *photoMutationSyncClient) SyncPushUpdates(_ context.Context, _ *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	c.requests++
	if c.nilReply {
		return nil, nil
	}
	return mtproto.EmptyVoid, c.err
}

func (c *profilePhotosMediaClient) MediaUploadProfilePhotoFile(_ context.Context, _ *mediapb.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error) {
	if c.err != nil {
		return nil, c.err
	}
	for _, photo := range c.photos {
		return photo, nil
	}
	return nil, nil
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

func TestPhotosMutationsFailClosedWhenProviderMissing(t *testing.T) {
	ctx := context.Background()
	logger := logx.WithContext(ctx)
	validPhoto := mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 88, AccessHash: 9}).To_InputPhoto()
	validFile := mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 7, Parts: 1, Name: "photo.jpg"}).To_InputFile()
	cases := []struct {
		name string
		call func(*UserChannelProfilesCore) error
		core *UserChannelProfilesCore
	}{
		{
			name: "update missing sync",
			call: func(c *UserChannelProfilesCore) error {
				_, err := c.PhotosUpdateProfilePhoto(&mtproto.TLPhotosUpdateProfilePhoto{Id: validPhoto})
				return err
			},
			core: &UserChannelProfilesCore{
				ctx: ctx, Logger: logger, MD: &metadata.RpcMetadata{UserId: 42},
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
					UserClient: newPhotoMutationUserClient(),
					MediaClient: &profilePhotosMediaClient{photos: map[int64]*mtproto.Photo{
						88: mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: 88}).To_Photo(),
					}},
				}},
			},
		},
		{
			name: "upload missing services",
			call: func(c *UserChannelProfilesCore) error {
				_, err := c.PhotosUploadProfilePhoto(&mtproto.TLPhotosUploadProfilePhoto{File: validFile})
				return err
			},
			core: &UserChannelProfilesCore{
				ctx: ctx, Logger: logger, MD: &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
			},
		},
		{
			name: "delete missing sync",
			call: func(c *UserChannelProfilesCore) error {
				_, err := c.PhotosDeletePhotos(&mtproto.TLPhotosDeletePhotos{Id: []*mtproto.InputPhoto{validPhoto}})
				return err
			},
			core: &UserChannelProfilesCore{
				ctx: ctx, Logger: logger, MD: &metadata.RpcMetadata{UserId: 42},
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
					UserClient:  newPhotoMutationUserClient(),
					MediaClient: &profilePhotosMediaClient{},
				}},
			},
		},
		{
			name: "contact upload missing media",
			call: func(c *UserChannelProfilesCore) error {
				_, err := c.PhotosUploadContactProfilePhoto(&mtproto.TLPhotosUploadContactProfilePhoto{
					UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 43, AccessHash: 9}).To_InputUser(),
					File:   validFile,
				})
				return err
			},
			core: &UserChannelProfilesCore{
				ctx: ctx, Logger: logger, MD: &metadata.RpcMetadata{UserId: 42, PermAuthKeyId: 99},
				svcCtx: &svc.ServiceContext{Dao: &dao.Dao{}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(tc.core)
			if status.Code(err) != status.Code(mtproto.ErrInternalServerError) {
				t.Fatalf("photo mutation error = %v, want INTERNAL_SERVER_ERROR", err)
			}
		})
	}
}

func TestPhotosUpdateProfilePhotoFailsClosedOnNilSyncReply(t *testing.T) {
	users := newPhotoMutationUserClient()
	media := &profilePhotosMediaClient{photos: map[int64]*mtproto.Photo{
		88: mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: 88}).To_Photo(),
	}}
	core := newPhotoMutationCore(users, media, &photoMutationSyncClient{nilReply: true})
	got, err := core.PhotosUpdateProfilePhoto(&mtproto.TLPhotosUpdateProfilePhoto{
		Id: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 88, AccessHash: 9}).To_InputPhoto(),
	})
	if got != nil && got.GetPhoto() != nil {
		t.Fatalf("PhotosUpdateProfilePhoto() = %v, want nil", got)
	}
	if status.Code(err) != status.Code(mtproto.ErrInternalServerError) {
		t.Fatalf("PhotosUpdateProfilePhoto() error = %v, want INTERNAL_SERVER_ERROR", err)
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
