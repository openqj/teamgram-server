package core

import (
	"context"
	"errors"
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

type botAccessSettingsUserClient struct {
	user_client.UserClient
	users map[int64]*mtproto.ImmutableUser
}

func (c *botAccessSettingsUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if user := c.users[in.GetId()]; user != nil {
		return user, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

func botAccessSettingsProfile(id, accessHash int64, bot bool) *mtproto.ImmutableUser {
	data := &mtproto.UserData{Id: id, AccessHash: accessHash}
	if bot {
		data.Bot = mtproto.MakeTLBotData(&mtproto.BotData{Id: id}).To_BotData()
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: data}).To_ImmutableUser()
}

func TestBotAccessSettingsPostgresRoundTrip(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !persist.PostgresEnabled() {
		t.Skip("APIFULL_POSTGRES_DSN with APIFull PostgreSQL schema is required")
	}
	const (
		botID      = int64(992001)
		botHash    = int64(992002)
		memberID   = int64(992003)
		memberHash = int64(992004)
	)
	client := &botAccessSettingsUserClient{users: map[int64]*mtproto.ImmutableUser{
		botID:    botAccessSettingsProfile(botID, botHash, true),
		memberID: botAccessSettingsProfile(memberID, memberHash, false),
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
		MD:     &metadata.RpcMetadata{UserId: botID},
	}
	db, err := persist.OpenPostgresDB(os.Getenv("APIFULL_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_bot_access_settings WHERE bot_user_id = $1`, botID)
		_ = db.Close()
	})
	bot := &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: botID, AccessHash: botHash}
	member := &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: memberID, AccessHash: memberHash}
	if got, err := c.BotsEditAccessSettings(&mtproto.TLBotsEditAccessSettings{Restricted: true, Bot: bot, AddUsers: []*mtproto.InputUser{member}}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("edit access settings = (%v, %v)", got, err)
	}
	settings, err := c.BotsGetAccessSettings(&mtproto.TLBotsGetAccessSettings{Bot: bot})
	if err != nil || settings == nil || !settings.GetRestricted() || len(settings.GetAddUsers()) != 1 || settings.GetAddUsers()[0].GetId() != memberID {
		t.Fatalf("read access settings = (%v, %v)", settings, err)
	}
	self := mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser()
	settings, err = c.BotsGetAccessSettings(&mtproto.TLBotsGetAccessSettings{Bot: self})
	if err != nil || settings == nil || !settings.GetRestricted() || len(settings.GetAddUsers()) != 1 {
		t.Fatalf("read access settings through inputUserSelf = (%v, %v)", settings, err)
	}
	if got, err := c.BotsEditAccessSettings(&mtproto.TLBotsEditAccessSettings{Bot: self, Restricted: false}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("edit access settings through inputUserSelf = (%v, %v)", got, err)
	}
	if got, err := c.BotsEditAccessSettings(&mtproto.TLBotsEditAccessSettings{Bot: bot}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("preserve access users = (%v, %v)", got, err)
	}
	settings, err = c.BotsGetAccessSettings(&mtproto.TLBotsGetAccessSettings{Bot: bot})
	if err != nil || len(settings.GetAddUsers()) != 1 {
		t.Fatalf("preserved access users = (%v, %v)", settings, err)
	}
	if got, err := c.BotsEditAccessSettings(&mtproto.TLBotsEditAccessSettings{Bot: bot, AddUsers: []*mtproto.InputUser{}}); err != nil || !mtproto.FromBool(got) {
		t.Fatalf("clear access users = (%v, %v)", got, err)
	}
	if got, err := c.BotsGetAccessSettings(&mtproto.TLBotsGetAccessSettings{Bot: bot}); err != nil || len(got.GetAddUsers()) != 0 {
		t.Fatalf("cleared access users = (%v, %v)", got, err)
	}
	if _, err := c.BotsGetAccessSettings(&mtproto.TLBotsGetAccessSettings{Bot: &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: botID, AccessHash: botHash + 1}}); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("wrong bot access hash = %v", err)
	}
}

func TestValidateBotAccessUsersRejectsMismatchedProfileID(t *testing.T) {
	const (
		requestedID = int64(993001)
		returnedID  = int64(993002)
		accessHash  = int64(993003)
	)
	client := &botAccessSettingsUserClient{users: map[int64]*mtproto.ImmutableUser{
		requestedID: botAccessSettingsProfile(returnedID, accessHash, false),
	}}
	c := &ApiFullCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}},
	}
	_, err := c.validateBotAccessUsers(990001, []*mtproto.InputUser{{
		PredicateName: mtproto.Predicate_inputUser,
		UserId:        requestedID,
		AccessHash:    accessHash,
	}})
	if !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("mismatched profile ID: got %v, want USER_ID_INVALID", err)
	}
}
