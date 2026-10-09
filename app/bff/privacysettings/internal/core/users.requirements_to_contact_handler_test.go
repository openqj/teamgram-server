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
	me             *mtproto.UserData
	meErr          error
	allowed        *mtproto.Bool
	privacyErr     error
	privacyRequest *userpb.TLUserCheckPrivacy
	owner          *mtproto.UserData
	ownerErr       error
	settingsCalls  int
	settings       *mtproto.GlobalPrivacySettings
	settingsErr    error
	contact        *mtproto.Bool
	contactErr     error
}

func (s *requirementsUserClientStub) UserGetUserDataById(_ context.Context, in *userpb.TLUserGetUserDataById) (*mtproto.UserData, error) {
	if in.GetUserId() != 42 {
		if s.owner != nil || s.ownerErr != nil {
			return s.owner, s.ownerErr
		}
		return &mtproto.UserData{Id: in.GetUserId(), AccessHash: in.GetUserId() + 100}, nil
	}
	return s.me, s.meErr
}

func (s *requirementsUserClientStub) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	s.privacyRequest = in
	return s.allowed, s.privacyErr
}

func (s *requirementsUserClientStub) UserGetGlobalPrivacySettings(context.Context, *userpb.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	s.settingsCalls++
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
		allowed:  mtproto.BoolFalse,
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
		me:         &mtproto.UserData{Id: 42},
		privacyErr: wantErr,
		settings:   mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{NoncontactPeersPaidStars: wrapperspb.Int64(50)}).To_GlobalPrivacySettings(),
		contact:    mtproto.BoolFalse,
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

func TestRequirementsToContactUsesAuthoritativeExemption(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		stub := &requirementsUserClientStub{
			me: &mtproto.UserData{Id: 42}, allowed: mtproto.ToBool(allowed), contact: mtproto.BoolFalse,
			settings: &mtproto.GlobalPrivacySettings{NoncontactPeersPaidStars: wrapperspb.Int64(50)},
		}
		got, err := newRequirementsTestCore(stub).UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: []*mtproto.InputUser{requirementInputUser(43)}})
		if err != nil || len(got.GetDatas()) != 1 || (got.GetDatas()[0].GetPredicateName() == mtproto.Predicate_requirementToContactEmpty) != allowed {
			t.Fatalf("requirement for allowed=%v = (%v, %v)", allowed, got, err)
		}
		if in := stub.privacyRequest; in.GetUserId() != 43 || in.GetPeerId() != 42 || in.GetKeyType() != mtproto.NO_PAID_MESSAGES {
			t.Fatalf("paid exemption request = %v", in)
		}
	}
}

func TestRequirementsToContactRejectsInvalidHashBeforeSettingsRead(t *testing.T) {
	stub := &requirementsUserClientStub{me: &mtproto.UserData{Id: 42}, owner: &mtproto.UserData{Id: 43, AccessHash: 999}}
	got, err := newRequirementsTestCore(stub).UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: []*mtproto.InputUser{requirementInputUser(43)}})
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) || stub.settingsCalls != 0 || stub.privacyRequest != nil {
		t.Fatalf("requirement = (%v, %v), settings reads=%d, privacy=%v", got, err, stub.settingsCalls, stub.privacyRequest)
	}
}

func TestRequirementsToContactPreservesOrderAndPremiumExpiration(t *testing.T) {
	stub := &requirementsUserClientStub{me: &mtproto.UserData{Id: 42, Premium: true, PremiumExpireDate: wrapperspb.Int64(1)},
		settings: &mtproto.GlobalPrivacySettings{NewNoncontactPeersRequirePremium: true}, contact: mtproto.BoolFalse}
	inputs := []*mtproto.InputUser{requirementInputUser(44), mtproto.MakeTLInputUserSelf(nil).To_InputUser(), requirementInputUser(43)}
	got, err := newRequirementsTestCore(stub).UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: inputs})
	if err != nil || len(got.GetDatas()) != 3 || got.GetDatas()[0].GetPredicateName() != mtproto.Predicate_requirementToContactPremium || got.GetDatas()[1].GetPredicateName() != mtproto.Predicate_requirementToContactEmpty || got.GetDatas()[2].GetPredicateName() != mtproto.Predicate_requirementToContactPremium {
		t.Fatalf("ordered requirements = (%v, %v)", got, err)
	}
	stub.me.PremiumExpireDate = nil
	got, err = newRequirementsTestCore(stub).UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: inputs})
	if err != nil || got.GetDatas()[0].GetPredicateName() != mtproto.Predicate_requirementToContactEmpty || got.GetDatas()[2].GetPredicateName() != mtproto.Predicate_requirementToContactEmpty {
		t.Fatalf("premium requirements = (%v, %v)", got, err)
	}
}

func TestRequirementsToContactPropagatesNilAndSettingsErrors(t *testing.T) {
	wantErr := errors.New("postgres settings unavailable")
	for _, tc := range []struct {
		name        string
		settingsErr error
		want        error
	}{
		{name: "nil privacy", want: mtproto.ErrInternalServerError},
		{name: "settings query", settingsErr: wantErr, want: wantErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &requirementsUserClientStub{me: &mtproto.UserData{Id: 42}, settingsErr: tc.settingsErr,
				settings: &mtproto.GlobalPrivacySettings{NoncontactPeersPaidStars: wrapperspb.Int64(50)}, contact: mtproto.BoolFalse}
			got, err := newRequirementsTestCore(stub).UsersGetRequirementsToContact(&mtproto.TLUsersGetRequirementsToContact{Id: []*mtproto.InputUser{requirementInputUser(43)}})
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("requirement = (%v, %v), want %v", got, err, tc.want)
			}
		})
	}
}
