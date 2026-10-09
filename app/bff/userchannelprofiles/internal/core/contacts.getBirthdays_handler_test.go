package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type birthdaysUserClientStub struct {
	userclient.UserClient
	err     error
	result  *userpb.Vector_ContactBirthday
	request *userpb.TLUserGetMutableUsersV2
}

func (s *birthdaysUserClientStub) UserGetBirthdays(context.Context, *userpb.TLUserGetBirthdays) (*userpb.Vector_ContactBirthday, error) {
	return s.result, s.err
}

func (s *birthdaysUserClientStub) UserGetMutableUsersV2(_ context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	s.request = in
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{{User: &mtproto.UserData{Id: 7}}, {User: &mtproto.UserData{Id: 42}}}}, nil
}

func TestContactsGetBirthdaysHydratesPrivacyForViewer(t *testing.T) {
	stub := &birthdaysUserClientStub{result: &userpb.Vector_ContactBirthday{Datas: []*mtproto.ContactBirthday{{ContactId: 7, Birthday: &mtproto.Birthday{Day: 1, Month: 1}}}}}
	ctx := context.Background()
	core := &UserChannelProfilesCore{ctx: ctx, MD: &metadata.RpcMetadata{UserId: 42}, Logger: logx.WithContext(ctx), svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}}}
	got, err := core.ContactsGetBirthdays(&mtproto.TLContactsGetBirthdays{})
	if err != nil || len(got.GetContacts()) != 1 || len(got.GetUsers()) != 1 {
		t.Fatalf("birthdays = (%v, %v), want hydrated contact", got, err)
	}
	if in := stub.request; !in.GetPrivacy() || !in.GetHasTo() || len(in.GetTo()) != 1 || in.GetTo()[0] != 42 {
		t.Fatalf("birthday users request = %v, want privacy for actual viewer", in)
	}
}

func TestContactsGetBirthdaysRejectsUnauthenticatedAndNilRequest(t *testing.T) {
	core := &UserChannelProfilesCore{}
	if _, err := core.ContactsGetBirthdays(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("nil core metadata error = %v, want AUTH_KEY_UNREGISTERED", err)
	}

	core.MD = &metadata.RpcMetadata{UserId: 972341}
	if _, err := core.ContactsGetBirthdays(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request error = %v, want INPUT_REQUEST_INVALID", err)
	}
}

func TestContactsGetBirthdaysPropagatesProviderError(t *testing.T) {
	want := errors.New("birthdays unavailable")
	core := &UserChannelProfilesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: &birthdaysUserClientStub{err: want}}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 972341},
	}
	_, err := core.ContactsGetBirthdays(&mtproto.TLContactsGetBirthdays{})
	if !errors.Is(err, want) {
		t.Fatalf("provider error = %v, want %v", err, want)
	}
}
