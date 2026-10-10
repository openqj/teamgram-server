package core

import (
	"context"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type attachMenuUserClient struct {
	user_client.UserClient
	users *mtproto.MutableUsers
}

func (c *attachMenuUserClient) UserGetMutableUsersV2(context.Context, *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return c.users, nil
}

func TestAttachMenuUsesValidatedUserAndAtomicState(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	// TestMain owns the process-wide PostgreSQL store. Closing it here makes
	// later database-backed tests depend on execution order.
	uid, botID := int64(101), int64(202)
	users := mtproto.MakeTLMutableUsers(&mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id: uid,
		}).To_UserData()}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id:         botID,
			AccessHash: 303,
			Username:   "attachbot",
			UserType:   botUserType,
			Bot: mtproto.MakeTLBotData(&mtproto.BotData{
				Id:                botID,
				BotAttachMenu:     true,
				AttachMenuEnabled: true,
			}).To_BotData(),
		}).To_UserData()}).To_ImmutableUser(),
	}}).To_MutableUsers()
	client := &attachMenuUserClient{users: users}
	c := &ApiFullCore{svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}}, MD: &metadata.RpcMetadata{UserId: uid}}
	bot := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: botID, AccessHash: 303}).To_InputUser()
	if got, err := c.MessagesToggleBotInAttachMenu(&mtproto.TLMessagesToggleBotInAttachMenu{Bot: bot, WriteAllowed: true}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("enable attach bot = (%v, %v)", got, err)
	}
	list, err := c.MessagesGetAttachMenuBots(&mtproto.TLMessagesGetAttachMenuBots{})
	if err != nil || list == nil || len(list.GetBots()) != 1 || list.GetBots()[0].GetBotId() != botID {
		t.Fatalf("attach bot list = (%v, %v)", list, err)
	}
	if _, err := c.MessagesGetAttachMenuBots(&mtproto.TLMessagesGetAttachMenuBots{Hash: list.GetHash()}); err != nil {
		t.Fatalf("hash re-read = %v", err)
	}
	item, err := c.MessagesGetAttachMenuBot(&mtproto.TLMessagesGetAttachMenuBot{Bot: bot})
	if err != nil || item == nil || item.GetBot() == nil || !item.GetBot().GetRequestWriteAccess() {
		t.Fatalf("attach bot item = (%v, %v)", item, err)
	}
	if _, err := c.MessagesToggleBotInAttachMenu(&mtproto.TLMessagesToggleBotInAttachMenu{Bot: bot, Enabled: mtproto.BoolFalse}); err != nil {
		t.Fatalf("disable attach bot = %v", err)
	}
	list, err = c.MessagesGetAttachMenuBots(&mtproto.TLMessagesGetAttachMenuBots{})
	if err != nil || list == nil || len(list.GetBots()) != 0 {
		t.Fatalf("attach bot list after disable = (%v, %v)", list, err)
	}
}

const botUserType = botUserTypeValue

const botUserTypeValue = 2
