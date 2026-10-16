package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type botInfoUserClient struct {
	user_client.UserClient
	profile   *mtproto.ImmutableUser
	info      *mtproto.BotInfo
	lastBotID int64
}

func (c *botInfoUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.profile, nil
}

func (c *botInfoUserClient) UserGetBotInfo(_ context.Context, in *userpb.TLUserGetBotInfo) (*mtproto.BotInfo, error) {
	c.lastBotID = in.GetBotId()
	return c.info, nil
}

func TestBotsGetBotInfoForwardsCanonicalUserProvider(t *testing.T) {
	const (
		callerID = int64(7001)
		botID    = int64(7002)
		hash     = int64(9002)
	)
	client := &botInfoUserClient{
		profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
			Id: botID, AccessHash: hash, FirstName: "Relay", LastName: "Bot",
			About: wrapperspb.String("Bot about"), Bot: &mtproto.BotData{},
		}}).To_ImmutableUser(),
		info: mtproto.MakeTLBotInfo(&mtproto.BotInfo{
			Description_STRING: "Canonical description",
		}).To_BotInfo(),
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}

	got, err := c.BotsGetBotInfoDCD914FD(&mtproto.TLBotsGetBotInfoDCD914FD{
		Bot: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: hash}).To_InputUser(),
	})
	if err != nil {
		t.Fatalf("bots.getBotInfo: %v", err)
	}
	if got == nil || got.GetName() != "Relay Bot" || got.GetAbout() != "Bot about" || got.GetDescription() != "Canonical description" {
		t.Fatalf("bots.getBotInfo = %#v", got)
	}
}

func TestBotsGetBotInfoDefaultsToCallerBot(t *testing.T) {
	const callerID = int64(7001)
	client := &botInfoUserClient{
		profile: mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{
			Id: callerID, AccessHash: 9001, FirstName: "Caller", LastName: "Bot",
			About: wrapperspb.String("Caller about"), Bot: &mtproto.BotData{},
		}}).To_ImmutableUser(),
		info: mtproto.MakeTLBotInfo(&mtproto.BotInfo{
			Description_STRING: "Caller description",
		}).To_BotInfo(),
	}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: callerID},
	}

	got, err := c.BotsGetBotInfoDCD914FD(&mtproto.TLBotsGetBotInfoDCD914FD{})
	if err != nil {
		t.Fatalf("bots.getBotInfo without bot: %v", err)
	}
	if client.lastBotID != callerID {
		t.Fatalf("provider bot id = %d, want caller id %d", client.lastBotID, callerID)
	}
	if got == nil || got.GetName() != "Caller Bot" || got.GetAbout() != "Caller about" || got.GetDescription() != "Caller description" {
		t.Fatalf("bots.getBotInfo without bot = %#v", got)
	}
}
