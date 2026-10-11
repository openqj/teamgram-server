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
