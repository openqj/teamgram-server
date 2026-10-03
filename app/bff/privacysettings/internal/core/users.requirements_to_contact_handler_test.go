package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/privacysettings/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type requirementsUserClientStub struct {
	userclient.UserClient
	me          *mtproto.UserData
	meErr       error
	rules       *userpb.Vector_PrivacyRule
	rulesErr    error
	settings    *mtproto.GlobalPrivacySettings
	settingsErr error
	contact     *mtproto.Bool
	contactErr  error
}

func (s *requirementsUserClientStub) UserGetUserDataById(context.Context, *userpb.TLUserGetUserDataById) (*mtproto.UserData, error) {
	return s.me, s.meErr
}

func (s *requirementsUserClientStub) UserGetPrivacy(context.Context, *userpb.TLUserGetPrivacy) (*userpb.Vector_PrivacyRule, error) {
	return s.rules, s.rulesErr
}

func (s *requirementsUserClientStub) UserGetGlobalPrivacySettings(context.Context, *userpb.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	return s.settings, s.settingsErr
}

func (s *requirementsUserClientStub) UserCheckContact(context.Context, *userpb.TLUserCheckContact) (*mtproto.Bool, error) {
	return s.contact, s.contactErr
}

func newRequirementsTestCore(stub *requirementsUserClientStub) *PrivacySettingsCore {
	ctx := context.Background()
	return &PrivacySettingsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: stub}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func requirementInputUser(id int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: id + 100}).To_InputUser()
}

func TestUsersGetRequirementsToContactReturnsPaidRequirement(t *testing.T) {
	core := newRequirementsTestCore(&requirementsUserClientStub{
		me:       &mtproto.UserData{Id: 42},
		rules:    &userpb.Vector_PrivacyRule{},
		settings: mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{NoncontactPeersPaidStars: wrapperspb.Int64(50)}).To_GlobalPrivacySettings(),
		contact:  mtproto.BoolFalse,
	})
	result, err := core.UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: []*mtproto.InputUser{requirementInputUser(43)}})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.GetDatas()) != 1 || result.GetDatas()[0].GetPredicateName() != mtproto.Predicate_requirementToContactPaidMessages || result.GetDatas()[0].GetStarsAmount() != 50 {
		t.Fatalf("requirements = %v, want paid-messages requirement for 50 Stars", result)
	}
}

func TestUsersGetRequirementsToContactPropagatesProviderFailure(t *testing.T) {
	wantErr := errors.New("privacy provider unavailable")
	core := newRequirementsTestCore(&requirementsUserClientStub{
		me:       &mtproto.UserData{Id: 42},
		rulesErr: wantErr,
		settings: mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{NoncontactPeersPaidStars: wrapperspb.Int64(50)}).To_GlobalPrivacySettings(),
		contact:  mtproto.BoolFalse,
	})
	result, err := core.UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: []*mtproto.InputUser{requirementInputUser(43)}})
	if result != nil || !errors.Is(err, wantErr) {
		t.Fatalf("provider failure = (%v, %v), want nil and %v", result, err, wantErr)
	}
}

func TestUsersGetIsPremiumRequiredToContactPropagatesMissingSelf(t *testing.T) {
	core := newRequirementsTestCore(&requirementsUserClientStub{})
	result, err := core.UsersGetIsPremiumRequiredToContact(&mtproto.TLUsersGetIsPremiumRequiredToContact{Id: []*mtproto.InputUser{requirementInputUser(43)}})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing self = (%v, %v), want INTERNAL_SERVER_ERROR", result, err)
	}
}
