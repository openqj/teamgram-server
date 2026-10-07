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
	err error
}

func (s *birthdaysUserClientStub) UserGetBirthdays(context.Context, *userpb.TLUserGetBirthdays) (*userpb.Vector_ContactBirthday, error) {
	return nil, s.err
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
