package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

func TestMessagesGetOutboxReadDateFailsClosedWithoutProviders(t *testing.T) {
	core := &MessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	request := &mtproto.TLMessagesGetOutboxReadDate{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 11}).To_InputPeer(),
		MsgId: 9,
	}

	got, err := core.MessagesGetOutboxReadDate(request)
	if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("MessagesGetOutboxReadDate() = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", got, err)
	}
}

type readDateUserClient struct {
	userclient.UserClient
	selfPremium          bool
	selfExpire           int64
	allowSelf, allowPeer bool
	privacyErr           error
	nilPrivacy           bool
	showSelf, showPeer   bool
	settingsErr          error
	nilSettings          bool
	checks               []*userpb.TLUserCheckPrivacy
}

func (c *readDateUserClient) UserGetGlobalPrivacySettings(_ context.Context, in *userpb.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	if c.settingsErr != nil || c.nilSettings {
		return nil, c.settingsErr
	}
	hide := !c.showPeer
	if in.GetUserId() == 42 {
		hide = !c.showSelf
	}
	return &mtproto.GlobalPrivacySettings{HideReadMarks: hide}, nil
}

func (c *readDateUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	data := &mtproto.UserData{Id: in.GetId(), AccessHash: 11}
	if in.GetId() == 42 {
		data.Premium, data.PremiumExpireDate = c.selfPremium, mtproto.MakeFlagsInt64(c.selfExpire)
	}
	return &mtproto.ImmutableUser{User: data}, nil
}

func (c *readDateUserClient) UserCheckPrivacy(_ context.Context, in *userpb.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	c.checks = append(c.checks, in)
	if c.privacyErr != nil || c.nilPrivacy {
		return nil, c.privacyErr
	}
	if in.GetUserId() == 42 {
		return mtproto.ToBool(c.allowSelf), nil
	}
	return mtproto.ToBool(c.allowPeer), nil
}

type readDateMessageClient struct {
	messageclient.MessageClient
	reads int
}

func (*readDateMessageClient) MessageGetUserMessage(context.Context, *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	return &mtproto.MessageBox{UserId: 42, MessageId: 9, SenderUserId: 42, PeerType: mtproto.PEER_USER, PeerId: 7,
		Message: &mtproto.Message{Id: 9, Out: true, Date: int32(time.Now().Unix())}}, nil
}

func (c *readDateMessageClient) MessageGetOutboxReadDate(context.Context, *messagepb.TLMessageGetOutboxReadDate) (*messagepb.Vector_ReadParticipantDate, error) {
	c.reads++
	return &messagepb.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{{UserId: 7, Date: 123}}}, nil
}

func TestMessagesGetOutboxReadDatePrivacyAndPremium(t *testing.T) {
	wantErr := errors.New("postgres privacy unavailable")
	for _, tc := range []struct {
		name    string
		users   *readDateUserClient
		wantErr error
		message string
	}{
		{name: "both allow", users: &readDateUserClient{allowSelf: true, allowPeer: true}},
		{name: "read dates public", users: &readDateUserClient{showSelf: true, showPeer: true}},
		{name: "peer denies", users: &readDateUserClient{allowSelf: true}, wantErr: mtproto.ErrUserPrivacyRestricted},
		{name: "self denies", users: &readDateUserClient{allowPeer: true}, message: "YOUR_PRIVACY_RESTRICTED"},
		{name: "premium self", users: &readDateUserClient{allowPeer: true, selfPremium: true, selfExpire: time.Now().Add(time.Hour).Unix()}},
		{name: "expired premium self", users: &readDateUserClient{allowPeer: true, selfPremium: true, selfExpire: 1}, message: "YOUR_PRIVACY_RESTRICTED"},
		{name: "query failure", users: &readDateUserClient{privacyErr: wantErr}, wantErr: wantErr},
		{name: "nil response", users: &readDateUserClient{nilPrivacy: true}, wantErr: mtproto.ErrInternalServerError},
		{name: "global privacy failure", users: &readDateUserClient{settingsErr: wantErr}, wantErr: wantErr},
		{name: "nil global privacy", users: &readDateUserClient{nilSettings: true}, wantErr: mtproto.ErrInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			messages := &readDateMessageClient{}
			c := &MessagesCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: tc.users, MessageClient: messages}}, MD: &metadata.RpcMetadata{UserId: 42}, Logger: logx.WithContext(ctx)}
			got, err := c.MessagesGetOutboxReadDate(&mtproto.TLMessagesGetOutboxReadDate{Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7, AccessHash: 11}).To_InputPeer(), MsgId: 9})
			if tc.message != "" {
				if got != nil || status.Convert(err).Message() != tc.message {
					t.Fatalf("read date = (%v, %v), want %s", got, err, tc.message)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("read date = (%v, %v), want %v", got, err, tc.wantErr)
			}
			if err == nil {
				wantChecks := 0
				if !tc.users.showPeer {
					wantChecks++
				}
				if !tc.users.showSelf && !tc.users.selfPremium {
					wantChecks++
				}
				if got.GetDate() != 123 || messages.reads != 1 || len(tc.users.checks) != wantChecks {
					t.Fatalf("read date result/checks = (%v, %d, %v)", got, messages.reads, tc.users.checks)
				}
			} else if messages.reads != 0 {
				t.Fatal("read timestamps queried after privacy rejection")
			}
			for _, in := range tc.users.checks {
				if in.GetKeyType() != mtproto.STATUS_TIMESTAMP || in.GetUserId() == 7 && in.GetPeerId() != 42 || in.GetUserId() == 42 && in.GetPeerId() != 7 {
					t.Fatalf("read date privacy request = %v", in)
				}
			}
		})
	}
}
