package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestBotEmojiStatusPermissionAndUpdatePostgres(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	ownerID := time.Now().UnixNano()
	botID := ownerID + 1
	ownerHash := ownerID + 10
	botHash := botID + 10
	client := &botEmojiUserClient{users: map[int64]*mtproto.ImmutableUser{
		ownerID: botEmojiProfile(ownerID, ownerHash, false),
		botID:   botEmojiProfile(botID, botHash, true),
	}}
	svcCtx := &svc.ServiceContext{Dao: &apifullDao.Dao{UserClient: client}}
	owner := &ApiFullCore{ctx: context.Background(), svcCtx: svcCtx, MD: &metadata.RpcMetadata{UserId: ownerID}}
	bot := &ApiFullCore{ctx: context.Background(), svcCtx: svcCtx, MD: &metadata.RpcMetadata{UserId: botID}}
	botInput := &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: botID, AccessHash: botHash}
	ownerInput := &mtproto.InputUser{PredicateName: mtproto.Predicate_inputUser, UserId: ownerID, AccessHash: ownerHash}

	if result, err := owner.BotsToggleUserEmojiStatusPermission(&mtproto.TLBotsToggleUserEmojiStatusPermission{Bot: botInput, Enabled: mtproto.BoolTrue}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("grant permission = %v, %v", result, err)
	}
	if result, err := bot.BotsUpdateUserEmojiStatus(&mtproto.TLBotsUpdateUserEmojiStatus{UserId: ownerInput, EmojiStatus: mtproto.MakeTLEmojiStatusEmpty(nil).To_EmojiStatus()}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("bot update = %v, %v", result, err)
	}
	if client.request == nil || client.request.GetUserId() != ownerID || client.request.GetEmojiStatusDocumentId() != 0 {
		t.Fatalf("user update request = %+v", client.request)
	}
	if result, err := owner.BotsToggleUserEmojiStatusPermission(&mtproto.TLBotsToggleUserEmojiStatusPermission{Bot: botInput, Enabled: mtproto.BoolFalse}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("revoke permission = %v, %v", result, err)
	}
	if result, err := bot.BotsUpdateUserEmojiStatus(&mtproto.TLBotsUpdateUserEmojiStatus{UserId: ownerInput, EmojiStatus: mtproto.MakeTLEmojiStatusEmpty(nil).To_EmojiStatus()}); result != nil || err == nil {
		t.Fatalf("revoked update = %v, %v", result, err)
	}
}

type botEmojiUserClient struct {
	user_client.UserClient
	users   map[int64]*mtproto.ImmutableUser
	request *userpb.TLUserUpdateEmojiStatus
}

func (c *botEmojiUserClient) UserGetImmutableUser(_ context.Context, in *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	if user := c.users[in.GetId()]; user != nil {
		return user, nil
	}
	return nil, mtproto.ErrUserIdInvalid
}

func (c *botEmojiUserClient) UserUpdateEmojiStatus(_ context.Context, in *userpb.TLUserUpdateEmojiStatus) (*mtproto.Bool, error) {
	c.request = in
	return mtproto.BoolTrue, nil
}

func botEmojiProfile(id, accessHash int64, bot bool) *mtproto.ImmutableUser {
	data := &mtproto.UserData{Id: id, AccessHash: accessHash}
	if bot {
		data.Bot = &mtproto.BotData{}
	}
	return &mtproto.ImmutableUser{User: data}
}
