package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type botAdminRightsUserClient struct {
	user_client.UserClient
	profile *mtproto.ImmutableUser
}

func (c *botAdminRightsUserClient) UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return c.profile, nil
}

func TestBotDefaultAdminRightsPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is required")
	}
	botID := int64(970000000 + os.Getpid()%100000)
	profile := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: botID, Bot: &mtproto.BotData{}}}
	c := &ApiFullCore{
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: &botAdminRightsUserClient{profile: profile}}},
		MD:     &metadata.RpcMetadata{UserId: botID},
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{InviteUsers: true, ManageTopics: true}).To_ChatAdminRights()
	if got, err := c.BotsSetBotGroupDefaultAdminRights(&mtproto.TLBotsSetBotGroupDefaultAdminRights{AdminRights: rights}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set group rights: got=%v err=%v", got, err)
	}
	rights = mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PostMessages: true, EditMessages: true}).To_ChatAdminRights()
	if got, err := c.BotsSetBotBroadcastDefaultAdminRights(&mtproto.TLBotsSetBotBroadcastDefaultAdminRights{AdminRights: rights}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("set broadcast rights: got=%v err=%v", got, err)
	}
	groupRaw, found, err := domain.GetBotDefaultAdminRights(botID, true)
	if err != nil || !found {
		t.Fatalf("load group rights: found=%v err=%v", found, err)
	}
	var gotGroup mtproto.ChatAdminRights
	if err = json.Unmarshal(groupRaw, &gotGroup); err != nil || !gotGroup.InviteUsers || !gotGroup.ManageTopics {
		t.Fatalf("group rights: %#v err=%v", gotGroup, err)
	}
	broadcastRaw, found, err := domain.GetBotDefaultAdminRights(botID, false)
	if err != nil || !found {
		t.Fatalf("load broadcast rights: found=%v err=%v", found, err)
	}
	var gotBroadcast mtproto.ChatAdminRights
	if err = json.Unmarshal(broadcastRaw, &gotBroadcast); err != nil || !gotBroadcast.PostMessages || !gotBroadcast.EditMessages {
		t.Fatalf("broadcast rights: %#v err=%v", gotBroadcast, err)
	}
}
