package core

import (
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *UserCore) UserCreateBot(in *user.TLUserCreateBot) (*mtproto.ImmutableUser, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	creatorUserId, err := c.requireBotCreator()
	if err != nil {
		return nil, err
	}
	return c.createBot(creatorUserId, 0, 0, in.GetName(), in.GetUsername())
}

func (c *UserCore) UserCreateManagedBot(in *user.BotRegistryCreateBotRequest) (*mtproto.ImmutableUser, error) {
	if in == nil || in.GetManagerBotId() <= 0 || in.GetManagerAccessHash() == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	creatorUserId, err := c.requireBotCreator()
	if err != nil {
		return nil, err
	}
	return c.createBot(creatorUserId, in.GetManagerBotId(), in.GetManagerAccessHash(), in.GetName(), in.GetUsername())
}

func (c *UserCore) requireBotCreator() (int64, error) {
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return 0, mtproto.ErrForbiddenUserBotInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return 0, mtproto.ErrMethodNotImpl
	}
	return c.MD.GetUserId(), nil
}

func (c *UserCore) createBot(creatorUserId, managerBotId, managerAccessHash int64, rawName, username string) (*mtproto.ImmutableUser, error) {
	name := strings.TrimSpace(rawName)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if !user.CheckUsernameInvalid(username) || !strings.HasSuffix(strings.ToLower(username), "bot") {
		return nil, mtproto.ErrUsernameInvalid
	}

	return c.svcCtx.Dao.CreateBot(c.ctx, creatorUserId, managerBotId, managerAccessHash, name, username)
}
