package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/users/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type resolvePhoneUserClientStub struct {
	userclient.UserClient
	phone string
}

func (s *resolvePhoneUserClientStub) UserGetUserIdByPhone(_ context.Context, in *userpb.TLUserGetUserIdByPhone) (*mtproto.Int64, error) {
	s.phone = in.GetPhone()
	return &mtproto.Int64{V: 2}, nil
}

func (*resolvePhoneUserClientStub) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 1, FirstName: "Alice"}},
		{User: &mtproto.UserData{Id: 2, FirstName: "Carol"}},
	}}, nil
}

func (*resolvePhoneUserClientStub) UserGetPrivacy(context.Context, *userpb.TLUserGetPrivacy) (*userpb.Vector_PrivacyRule, error) {
	return &userpb.Vector_PrivacyRule{Datas: []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule(),
	}}, nil
}

func TestContactsResolvePhoneNormalizesE164(t *testing.T) {
	ctx := context.Background()
	userClient := &resolvePhoneUserClientStub{}
	core := &UsersCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}

	got, err := core.ContactsResolvePhone(&mtproto.TLContactsResolvePhone{Phone: "+12025550103"})
	if err != nil {
		t.Fatalf("ContactsResolvePhone() error = %v", err)
	}
	if userClient.phone != "12025550103" {
		t.Fatalf("phone lookup = %q, want normalized E.164 digits", userClient.phone)
	}
	if got.GetPeer().GetUserId() != 2 {
		t.Fatalf("resolved peer = %+v, want user 2", got.GetPeer())
	}
}
