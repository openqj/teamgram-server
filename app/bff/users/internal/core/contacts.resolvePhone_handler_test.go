package core

import (
	"context"
	"errors"
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
	phone                             string
	allowed                           *mtproto.Bool
	privacyRequest                    *userpb.TLUserCheckPrivacy
	privacyErr, lookupErr, mutableErr error
}

func (s *resolvePhoneUserClientStub) UserGetUserIdByPhone(_ context.Context, in *userpb.TLUserGetUserIdByPhone) (*mtproto.Int64, error) {
	s.phone = in.GetPhone()
	return &mtproto.Int64{V: 2}, s.lookupErr
}

func (s *resolvePhoneUserClientStub) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		{User: &mtproto.UserData{Id: 1, FirstName: "Alice"}},
		{User: &mtproto.UserData{Id: 2, FirstName: "Carol"}},
	}}, s.mutableErr
}

func (s *resolvePhoneUserClientStub) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	s.privacyRequest = in
	return s.allowed, s.privacyErr
}

func TestContactsResolvePhoneNormalizesE164(t *testing.T) {
	ctx := context.Background()
	userClient := &resolvePhoneUserClientStub{allowed: mtproto.BoolTrue}
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
	if request := userClient.privacyRequest; request.GetUserId() != 2 || request.GetPeerId() != 1 || request.GetKeyType() != mtproto.ADDED_BY_PHONE {
		t.Fatalf("privacy request = %v, want resolved owner 2/viewer 1", request)
	}
}

func TestContactsResolvePhonePreservesPrivacyDenialAndDatabaseErrors(t *testing.T) {
	wantErr := errors.New("postgres privacy unavailable")
	tests := []struct {
		name   string
		client *resolvePhoneUserClientStub
		want   error
	}{
		{name: "privacy denied", client: &resolvePhoneUserClientStub{allowed: mtproto.BoolFalse}, want: mtproto.ErrPhoneNotOccupied},
		{name: "privacy query", client: &resolvePhoneUserClientStub{privacyErr: wantErr}, want: wantErr},
		{name: "user batch query", client: &resolvePhoneUserClientStub{mutableErr: wantErr}, want: wantErr},
		{name: "phone query", client: &resolvePhoneUserClientStub{lookupErr: wantErr}, want: wantErr},
		{name: "nil privacy", client: &resolvePhoneUserClientStub{}, want: mtproto.ErrInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := &UsersCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: tc.client}}, Logger: logx.WithContext(ctx), MD: &metadata.RpcMetadata{UserId: 1}}
			if got, err := c.ContactsResolvePhone(&mtproto.TLContactsResolvePhone{Phone: "+12025550103"}); got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("resolve = (%v, %v), want %v", got, err, tc.want)
			}
		})
	}
}
