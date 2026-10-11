package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type paidMessagePrivacyUserClient struct {
	userclient.UserClient
	rules   []*mtproto.PrivacyRule
	saved   *userpb.TLUserSetPrivacy
	result  *mtproto.Bool
	profile *mtproto.ImmutableUser
}

func (c *paidMessagePrivacyUserClient) UserGetPrivacy(context.Context, *userpb.TLUserGetPrivacy) (*userpb.Vector_PrivacyRule, error) {
	return &userpb.Vector_PrivacyRule{Datas: c.rules}, nil
}

func (c *paidMessagePrivacyUserClient) UserSetPrivacy(_ context.Context, in *userpb.TLUserSetPrivacy) (*mtproto.Bool, error) {
	c.saved = in
	return c.result, nil
}

func (c *paidMessagePrivacyUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.profile, nil
}

func TestAccountAddNoPaidMessagesExceptionPersistsCallerPrivacyRule(t *testing.T) {
	const callerID int64 = 990001
	const targetID int64 = 990002
	client := &paidMessagePrivacyUserClient{
		rules: []*mtproto.PrivacyRule{
			mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		},
		result: mtproto.BoolTrue,
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	got, err := c.AccountAddNoPaidMessagesException(&mtproto.TLAccountAddNoPaidMessagesException{
		UserId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
	})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("add exception = (%v, %v), want BoolTrue", got, err)
	}
	if client.saved == nil || client.saved.GetUserId() != callerID || client.saved.GetKeyType() != mtproto.NO_PAID_MESSAGES {
		t.Fatalf("saved privacy request = %+v", client.saved)
	}
	if len(client.saved.GetRules()) != 2 {
		t.Fatalf("saved privacy rules = %+v, want existing deny plus allow-users", client.saved.GetRules())
	}
	allow := client.saved.GetRules()[1]
	if allow.GetPredicateName() != mtproto.Predicate_privacyValueAllowUsers || len(allow.GetUsers()) != 1 || allow.GetUsers()[0] != callerID {
		t.Fatalf("allow rule = %+v", allow)
	}
}

func TestAccountToggleNoPaidMessagesExceptionPersistsCallerPrivacyRule(t *testing.T) {
	const callerID int64 = 990008
	client := &paidMessagePrivacyUserClient{
		rules: []*mtproto.PrivacyRule{
			mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		},
		result: mtproto.BoolTrue,
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	got, err := c.AccountToggleNoPaidMessagesException(&mtproto.TLAccountToggleNoPaidMessagesException{
		UserId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
	})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("toggle = (%v, %v), want BoolTrue", got, err)
	}
	if client.saved == nil || client.saved.GetUserId() != callerID || client.saved.GetKeyType() != mtproto.NO_PAID_MESSAGES {
		t.Fatalf("saved privacy request = %+v", client.saved)
	}
	if len(client.saved.GetRules()) != 2 {
		t.Fatalf("saved privacy rules = %+v, want existing deny plus allow-users", client.saved.GetRules())
	}
	allow := client.saved.GetRules()[1]
	if allow.GetPredicateName() != mtproto.Predicate_privacyValueAllowUsers || len(allow.GetUsers()) != 1 || allow.GetUsers()[0] != callerID {
		t.Fatalf("allow rule = %+v", allow)
	}
}

func TestAccountToggleNoPaidMessagesExceptionDoesNotMutateReadRules(t *testing.T) {
	const callerID int64 = 990010
	const existingID int64 = 990011
	client := &paidMessagePrivacyUserClient{
		rules: []*mtproto.PrivacyRule{
			mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{existingID}}).To_PrivacyRule(),
		},
		result: mtproto.BoolTrue,
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	if got, err := c.AccountToggleNoPaidMessagesException(&mtproto.TLAccountToggleNoPaidMessagesException{
		UserId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
	}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("toggle = (%v, %v), want BoolTrue", got, err)
	}
	if len(client.rules[0].GetUsers()) != 1 || client.rules[0].GetUsers()[0] != existingID {
		t.Fatalf("read privacy rules mutated: %+v", client.rules)
	}
	if len(client.saved.GetRules()[0].GetUsers()) != 2 {
		t.Fatalf("saved allowlist = %+v, want existing plus caller", client.saved.GetRules()[0].GetUsers())
	}
}

func TestAccountToggleNoPaidMessagesExceptionPersistsValidatedUserTarget(t *testing.T) {
	const callerID int64 = 990012
	const targetID int64 = 990013
	const accessHash int64 = 770013
	client := &paidMessagePrivacyUserClient{
		rules:  []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()},
		result: mtproto.BoolTrue,
		profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: targetID, AccessHash: accessHash},
		}).To_ImmutableUser(),
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	target := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID, AccessHash: accessHash}).To_InputUser()
	if got, err := c.AccountToggleNoPaidMessagesException(&mtproto.TLAccountToggleNoPaidMessagesException{UserId: target}); err != nil || got != mtproto.BoolTrue {
		t.Fatalf("toggle = (%v, %v), want BoolTrue", got, err)
	}
	if client.saved == nil || len(client.saved.GetRules()) != 2 || len(client.saved.GetRules()[1].GetUsers()) != 1 || client.saved.GetRules()[1].GetUsers()[0] != targetID {
		t.Fatalf("saved target exception = %+v, want target %d", client.saved, targetID)
	}
}

func TestAccountToggleNoPaidMessagesExceptionRejectsUnsupportedLocalFlags(t *testing.T) {
	const callerID int64 = 990009
	client := &paidMessagePrivacyUserClient{result: mtproto.BoolTrue}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	target := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	for name, in := range map[string]*mtproto.TLAccountToggleNoPaidMessagesException{
		"refund":  {RefundCharged: true, UserId: target},
		"require": {RequirePayment: true, UserId: target},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := c.AccountToggleNoPaidMessagesException(in)
			if got != nil || !errors.Is(err, mtproto.ErrPaymentUnsupported) {
				t.Fatalf("toggle = (%v, %v), want PAYMENT_UNSUPPORTED", got, err)
			}
			if client.saved != nil {
				t.Fatalf("unsupported flag wrote privacy request: %+v", client.saved)
			}
		})
	}
}

func TestAccountAddNoPaidMessagesExceptionValidatesTargetAccessHash(t *testing.T) {
	const callerID int64 = 990006
	const targetID int64 = 990007
	client := &paidMessagePrivacyUserClient{
		profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
			User: &mtproto.UserData{Id: targetID, AccessHash: 77},
		}).To_ImmutableUser(),
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}
	got, err := c.AccountAddNoPaidMessagesException(&mtproto.TLAccountAddNoPaidMessagesException{
		RefundCharged: true,
		UserId:        mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID, AccessHash: 77}).To_InputUser(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentUnsupported) {
		t.Fatalf("valid target with unavailable refund provider = (%v, %v), want PAYMENT_UNSUPPORTED after access validation", got, err)
	}
	client.profile.GetUser().AccessHash = 78
	client.saved = nil
	got, err = c.AccountAddNoPaidMessagesException(&mtproto.TLAccountAddNoPaidMessagesException{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID, AccessHash: 77}).To_InputUser(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) || client.saved != nil {
		t.Fatalf("stale target access hash = (%v, %v), want USER_ID_INVALID and no write", got, err)
	}
}

func TestAccountAddNoPaidMessagesExceptionRejectsInvalidTargetBeforeRead(t *testing.T) {
	client := &paidMessagePrivacyUserClient{result: mtproto.BoolTrue}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: 990003},
	}
	got, err := c.AccountAddNoPaidMessagesException(&mtproto.TLAccountAddNoPaidMessagesException{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 990004}).To_InputUser(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("invalid target = (%v, %v), want USER_ID_INVALID", got, err)
	}
}

func TestAccountAddNoPaidMessagesExceptionRejectsUnverifiableRefund(t *testing.T) {
	client := &paidMessagePrivacyUserClient{result: mtproto.BoolTrue}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: 990005},
	}
	got, err := c.AccountAddNoPaidMessagesException(&mtproto.TLAccountAddNoPaidMessagesException{
		RefundCharged: true,
		UserId:        mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrPaymentUnsupported) {
		t.Fatalf("refund request = (%v, %v), want PAYMENT_UNSUPPORTED", got, err)
	}
}
