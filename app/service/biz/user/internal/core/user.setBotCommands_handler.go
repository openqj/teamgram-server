/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	maxBotCommands           = 100
	maxBotCommandNameLength  = 32
	maxBotCommandDescription = 256
)

// UserSetBotCommands
// user.setBotCommands user_id:long bot_id:long commands:Vector<BotCommand> = Bool;
func (c *UserCore) UserSetBotCommands(in *user.TLUserSetBotCommands) (*mtproto.Bool, error) {
	doList, err := makeBotCommandsDOList(in)
	if err != nil {
		return nil, err
	}

	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		if c != nil && c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - error: bot command storage is not configured")
		}
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil ||
		c.svcCtx.Dao.Postgres.Store.Bots == nil || c.svcCtx.Dao.Postgres.Store.BotCommands == nil {
		if c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - error: bot command storage is not configured")
		}
		return nil, mtproto.ErrMethodNotImpl
	}

	if c.MD != nil {
		if c.MD.GetUserId() <= 0 {
			if c.Logger != nil {
				c.Logger.Errorf("user.setBotCommands - error: caller metadata has no user id")
			}
			return nil, mtproto.ErrAuthKeyUnregistered
		}
		if c.MD.GetUserId() != in.GetUserId() {
			if c.Logger != nil {
				c.Logger.Errorf("user.setBotCommands - error: caller(%d) cannot update user(%d)", c.MD.GetUserId(), in.GetUserId())
			}
			return nil, mtproto.ErrUserIdInvalid
		}
	}

	var txErr error
	tx, err := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(c.ctx) }()
		var botDO *dataobject.BotsDO
		botDO, txErr = c.svcCtx.Dao.Postgres.Store.Bots.SelectForUpdateTx(c.ctx, tx, in.GetBotId())
		if txErr == nil && (botDO == nil || botDO.BotId != in.GetBotId()) {
			txErr = mtproto.ErrBotInvalid
		}
		if txErr == nil && botDO.BotId != in.GetUserId() {
			if botDO.CreatorUserId <= 0 {
				if c.Logger != nil {
					c.Logger.Errorf("user.setBotCommands - bot(%d) has no authoritative creator", in.GetBotId())
				}
				txErr = mtproto.ErrMethodNotImpl
			} else if botDO.CreatorUserId != in.GetUserId() {
				if c.Logger != nil {
					c.Logger.Errorf("user.setBotCommands - user(%d) is not creator or bot owner of bot(%d)", in.GetUserId(), in.GetBotId())
				}
				txErr = mtproto.ErrForbiddenUserBotInvalid
			}
		}
		if txErr == nil {
			_, txErr = c.svcCtx.Dao.Postgres.Store.BotCommands.DeleteTx(c.ctx, tx, in.GetBotId())
		}
		if txErr == nil && len(doList) > 0 {
			_, _, txErr = c.svcCtx.Dao.Postgres.Store.BotCommands.InsertBulkTx(c.ctx, tx, doList)
		}
		if txErr == nil {
			txErr = tx.Commit(c.ctx)
		}
	} else {
		txErr = err
	}
	if txErr != nil {
		if c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - replace bot(%d) commands error: %v", in.GetBotId(), txErr)
		}
		return nil, txErr
	}

	return mtproto.BoolTrue, nil
}

func makeBotCommandsDOList(in *user.TLUserSetBotCommands) ([]*dataobject.BotCommandsDO, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetBotId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	commands := in.GetCommands()
	if len(commands) > maxBotCommands {
		return nil, mtproto.ErrInputRequestInvalid
	}

	doList := make([]*dataobject.BotCommandsDO, 0, len(commands))
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		if command == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		name := command.GetCommand()
		if !isValidBotCommandName(name) {
			return nil, mtproto.ErrBotCommandInvalid
		}
		if _, ok := seen[name]; ok {
			return nil, mtproto.ErrBotCommandInvalid
		}
		seen[name] = struct{}{}

		description := command.GetDescription()
		if strings.TrimSpace(description) == "" || !utf8.ValidString(description) ||
			utf8.RuneCountInString(description) > maxBotCommandDescription {
			return nil, mtproto.ErrBotCommandDescriptionInvalid
		}
		if command.GetEphemeral() {
			// bot_commands has no ephemeral column; silently dropping it would
			// change the command semantics on the next read.
			return nil, mtproto.ErrBotCommandInvalid
		}

		doList = append(doList, &dataobject.BotCommandsDO{
			BotId:       in.GetBotId(),
			Command:     name,
			Description: description,
		})
	}

	return doList, nil
}

func isValidBotCommandName(command string) bool {
	if len(command) == 0 || len(command) > maxBotCommandNameLength {
		return false
	}
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '_' {
			return false
		}
	}
	return true
}
