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
	profile *mtproto.ImmutableUser
	info    *mtproto.BotInfo
}

func (c *botInfoUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.profile, nil
}

func (c *botInfoUserClient) UserGetBotInfo(context.Context, *userpb.TLUserGetBotInfo) (*mtproto.BotInfo, error) {
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
