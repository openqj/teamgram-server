package core

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *UserCore) UserSetBotInfo(in *user.BotRegistrySetBotInfoRequest) (*mtproto.Bool, error) {
	if in == nil || in.GetBotId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Users == nil || c.svcCtx.Dao.Postgres.Store.Bots == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.MD.GetUserId() != in.GetBotId() {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetName() != nil && (strings.TrimSpace(in.GetName().GetValue()) == "" || !validBotProfileText(in.GetName().GetValue(), 64, false)) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetAbout() != nil && !validBotProfileText(in.GetAbout().GetValue(), 128, true) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetDescription() != nil && !validBotProfileText(in.GetDescription().GetValue(), 10240, true) {
		return nil, mtproto.ErrInputRequestInvalid
	}

	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(c.ctx) }()
	bot, err := c.svcCtx.Dao.Postgres.Store.Bots.Select(c.ctx, in.GetBotId())
	if err != nil {
		return nil, err
	}
	if bot == nil || bot.BotId != in.GetBotId() {
		return nil, mtproto.ErrBotInvalid
	}
	userDO, err := c.svcCtx.Dao.Postgres.Store.Users.SelectByIDOn(c.ctx, tx, in.GetBotId())
	if err != nil {
		return nil, err
	}
	if userDO == nil || userDO.UserType != user.UserTypeBot || userDO.Deleted {
		return nil, mtproto.ErrBotInvalid
	}
	userChanges := make(map[string]any, 2)
	if in.GetName() != nil {
		userChanges["first_name"] = in.GetName().GetValue()
	}
	if in.GetAbout() != nil {
		userChanges["about"] = in.GetAbout().GetValue()
	}
	if len(userChanges) > 0 {
		if _, err = c.svcCtx.Dao.Postgres.Store.Users.UpdateTx(c.ctx, tx, userChanges, in.GetBotId()); err != nil {
			return nil, err
		}
	}
	if in.GetDescription() != nil {
		if _, err = c.svcCtx.Dao.Postgres.Store.Bots.UpdateTx(c.ctx, tx, map[string]any{"description": in.GetDescription().GetValue()}, in.GetBotId()); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(c.ctx); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func validBotProfileText(value string, maxRunes int, allowLineBreaks bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(allowLineBreaks && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
