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

type personalChannelUserClient struct {
	userclient.UserClient
	result  *mtproto.Bool
	err     error
	called  bool
	request *userpb.TLUserUpdatePersonalChannel
}

func (c *personalChannelUserClient) UserUpdatePersonalChannel(_ context.Context, in *userpb.TLUserUpdatePersonalChannel) (*mtproto.Bool, error) {
	c.called = true
	c.request = in
	return c.result, c.err
}

func TestAccountUpdatePersonalChannelValidatesInput(t *testing.T) {
	users := &personalChannelUserClient{result: mtproto.BoolTrue}
	core := newPersonalChannelCore(users, 42)

	for name, in := range map[string]*mtproto.TLAccountUpdatePersonalChannel{
		"nil request":   nil,
		"nil channel":   {},
		"empty channel": {Channel: mtproto.MakeTLInputChannelEmpty(nil).To_InputChannel()},
		"zero channel":  {Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 0}).To_InputChannel()},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := core.AccountUpdatePersonalChannel(in)
			if got != nil || !errors.Is(err, mtproto.ErrChannelInvalid) {
				t.Fatalf("result = (%v, %v), want CHANNEL_INVALID", got, err)
			}
		})
	}
	if users.called {
		t.Fatal("user service called for invalid request")
	}
}

func TestAccountUpdatePersonalChannelCallsUserService(t *testing.T) {
	users := &personalChannelUserClient{result: mtproto.BoolTrue}
	core := newPersonalChannelCore(users, 42)
	got, err := core.AccountUpdatePersonalChannel(&mtproto.TLAccountUpdatePersonalChannel{
		Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 99}).To_InputChannel(),
	})
	if err != nil || got != mtproto.BoolTrue {
		t.Fatalf("result = (%v, %v), want BoolTrue", got, err)
	}
	if !users.called || users.request.GetUserId() != 42 || users.request.GetChannelId() != 99 {
		t.Fatalf("user request = %v", users.request)
	}
}

func TestAccountUpdatePersonalChannelPropagatesUserError(t *testing.T) {
	want := errors.New("user update failed")
	users := &personalChannelUserClient{err: want}
	core := newPersonalChannelCore(users, 42)
	got, err := core.AccountUpdatePersonalChannel(&mtproto.TLAccountUpdatePersonalChannel{
		Channel: mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 99}).To_InputChannel(),
	})
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("result = (%v, %v), want propagated error", got, err)
	}
}

func newPersonalChannelCore(users userclient.UserClient, userID int64) *UserChannelProfilesCore {
	ctx := context.Background()
	return &UserChannelProfilesCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: users}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}
