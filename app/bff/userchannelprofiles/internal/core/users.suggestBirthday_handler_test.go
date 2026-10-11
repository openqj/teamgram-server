package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/internal/svc"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type birthdaySuggestionUserClient struct {
	userclient.UserClient
	user *mtproto.ImmutableUser
}

func (c *birthdaySuggestionUserClient) UserGetImmutableUser(_ context.Context, _ *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.user, nil
}

type birthdaySuggestionMsgClient struct {
	msgclient.MsgClient
	request *msgpb.TLMsgSendMessageV2
	result  *mtproto.Updates
}

func (c *birthdaySuggestionMsgClient) MsgSendMessageV2(_ context.Context, in *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	c.request = in
	return c.result, nil
}

func TestUsersSuggestBirthdayFailsClosedWithoutRecipientWorkflow(t *testing.T) {
	ctx := context.Background()
	c := &UserChannelProfilesCore{
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}

	got, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 99}).To_InputUser(),
		Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3}).To_Birthday(),
	})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("UsersSuggestBirthday() = (%+v, %v), want (nil, METHOD_NOT_IMPL)", got, err)
	}
}

func TestUsersSuggestBirthdayKeepsInputValidation(t *testing.T) {
	ctx := context.Background()
	c := &UserChannelProfilesCore{
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
	validRecipient := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 7, AccessHash: 99}).To_InputUser()

	for _, tc := range []struct {
		name string
		in   *mtproto.TLUsersSuggestBirthday
		want error
	}{
		{
			name: "self recipient",
			in: &mtproto.TLUsersSuggestBirthday{
				Id:       mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
				Birthday: mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3}).To_Birthday(),
			},
			want: mtproto.ErrUserIdInvalid,
		},
		{
			name: "missing birthday",
			in: &mtproto.TLUsersSuggestBirthday{
				Id: validRecipient,
			},
			want: mtproto.ErrInputRequestInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.UsersSuggestBirthday(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("UsersSuggestBirthday() = (%+v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestUsersSuggestBirthdayUsesDurableMessageServiceWhenProviderIsUnset(t *testing.T) {
	const callerID int64 = 42
	const targetID int64 = 7
	const accessHash int64 = 700
	birthday := mtproto.MakeTLBirthday(&mtproto.Birthday{Day: 12, Month: 3}).To_Birthday()
	users := &birthdaySuggestionUserClient{user: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{Id: targetID, AccessHash: accessHash}).To_UserData(),
	}).To_ImmutableUser()}
	sender := &birthdaySuggestionMsgClient{result: mtproto.MakeEmptyUpdates()}
	c := &UserChannelProfilesCore{
		ctx: context.Background(), Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: callerID, PermAuthKeyId: 9001},
		svcCtx: &svc.ServiceContext{Config: config.Config{}, Dao: &dao.Dao{UserClient: users, MsgClient: sender}},
	}

	got, err := c.UsersSuggestBirthday(&mtproto.TLUsersSuggestBirthday{
		Id:       mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: targetID, AccessHash: accessHash}).To_InputUser(),
		Birthday: birthday,
	})
	if err != nil || got != sender.result {
		t.Fatalf("UsersSuggestBirthday() = (%v, %v), want durable message result", got, err)
	}
	if sender.request == nil || sender.request.GetUserId() != callerID || sender.request.GetAuthKeyId() != 9001 || sender.request.GetPeerType() != mtproto.PEER_USER || sender.request.GetPeerId() != targetID || len(sender.request.GetMessage()) != 1 {
		t.Fatalf("message request = %v", sender.request)
	}
	message := sender.request.GetMessage()[0].GetMessage()
	if message == nil || message.GetPredicateName() != mtproto.Predicate_messageService || message.GetPeerId().GetUserId() != targetID || message.GetFromId().GetUserId() != callerID {
		t.Fatalf("service message = %v", message)
	}
	action := message.GetAction()
	if action == nil || action.GetPredicateName() != mtproto.Predicate_messageActionSuggestBirthday || action.GetBirthday().GetDay() != 12 || action.GetBirthday().GetMonth() != 3 {
		t.Fatalf("service action = %v", action)
	}
}
