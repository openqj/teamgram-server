package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediaclient "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/zeromicro/go-zero/core/logx"
)

type profilePhotosUserClient struct {
	userclient.UserClient
	response *userpb.Vector_Long
	err      error
	request  *userpb.TLUserGetProfilePhotos
}

func (c *profilePhotosUserClient) UserGetProfilePhotos(_ context.Context, in *userpb.TLUserGetProfilePhotos) (*userpb.Vector_Long, error) {
	c.request = in
	return c.response, c.err
}

type profilePhotosMediaClient struct {
	mediaclient.MediaClient
	photos   map[int64]*mtproto.Photo
	err      error
	nilPhoto bool
	ids      []int64
}

func (c *profilePhotosMediaClient) MediaGetPhoto(_ context.Context, in *mediapb.TLMediaGetPhoto) (*mtproto.Photo, error) {
	c.ids = append(c.ids, in.GetPhotoId())
	if c.err != nil {
		return nil, c.err
	}
	if c.nilPhoto {
		return nil, nil
	}
	return c.photos[in.GetPhotoId()], nil
}

func TestPhotosGetUserPhotosReturnsPhotosForResolvedUser(t *testing.T) {
	wantPhoto := mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: 19}).To_Photo()
	users := &profilePhotosUserClient{response: &userpb.Vector_Long{Datas: []int64{19}}}
	media := &profilePhotosMediaClient{photos: map[int64]*mtproto.Photo{19: wantPhoto}}
	core := newProfilePhotosCore(users, media)

	got, err := core.PhotosGetUserPhotos(&mtproto.TLPhotosGetUserPhotos{
		UserId: mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	})
	if err != nil {
		t.Fatalf("PhotosGetUserPhotos() error = %v", err)
	}
	if users.request == nil || users.request.GetUserId() != 42 {
		t.Fatalf("profile-photo owner = %v, want user 42", users.request)
	}
	if !reflect.DeepEqual(media.ids, []int64{19}) || len(got.GetPhotos()) != 1 || got.GetPhotos()[0] != wantPhoto {
		t.Fatalf("PhotosGetUserPhotos() = %v, media ids = %v", got, media.ids)
	}
}

func TestPhotosGetUserPhotosFailsClosedOnUserServiceFailures(t *testing.T) {
	serviceErr := errors.New("profile photos unavailable")
	for _, tc := range []struct {
		name  string
		users *profilePhotosUserClient
		want  error
	}{
		{name: "service error", users: &profilePhotosUserClient{err: serviceErr}, want: serviceErr},
		{name: "nil response", users: &profilePhotosUserClient{}, want: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := &profilePhotosMediaClient{}
			got, err := newProfilePhotosCore(tc.users, media).PhotosGetUserPhotos(profilePhotosSelfRequest())
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("PhotosGetUserPhotos() = (%v, %v), want error %v", got, err, tc.want)
			}
			if len(media.ids) != 0 {
				t.Fatalf("media calls after user-service failure = %v, want none", media.ids)
			}
		})
	}
}

func TestPhotosGetUserPhotosFailsClosedOnMediaFailures(t *testing.T) {
	mediaErr := errors.New("photo unavailable")
	for _, tc := range []struct {
		name  string
		media *profilePhotosMediaClient
		want  error
	}{
		{name: "service error", media: &profilePhotosMediaClient{err: mediaErr}, want: mediaErr},
		{name: "nil response", media: &profilePhotosMediaClient{nilPhoto: true}, want: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := &profilePhotosUserClient{response: &userpb.Vector_Long{Datas: []int64{19}}}
			got, err := newProfilePhotosCore(users, tc.media).PhotosGetUserPhotos(profilePhotosSelfRequest())
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("PhotosGetUserPhotos() = (%v, %v), want error %v", got, err, tc.want)
			}
			if !reflect.DeepEqual(tc.media.ids, []int64{19}) {
				t.Fatalf("media calls = %v, want [19]", tc.media.ids)
			}
		})
	}
}

func profilePhotosSelfRequest() *mtproto.TLPhotosGetUserPhotos {
	return &mtproto.TLPhotosGetUserPhotos{UserId: mtproto.MakeTLInputUserSelf(nil).To_InputUser()}
}

func newProfilePhotosCore(users *profilePhotosUserClient, media *profilePhotosMediaClient) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users, MediaClient: media}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}
