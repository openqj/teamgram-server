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

	"github.com/teamgram/marmota/pkg/stores/sqlx"
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

	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Mysql == nil ||
		c.svcCtx.Dao.Mysql.DB == nil || c.svcCtx.Dao.Mysql.BotsDAO == nil ||
		c.svcCtx.Dao.Mysql.BotCommandsDAO == nil {
		if c != nil && c.Logger != nil {
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

	botDO, err := c.svcCtx.Dao.BotsDAO.Select(c.ctx, in.GetBotId())
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - select bot(%d) error: %v", in.GetBotId(), err)
		}
		return nil, err
	}
	if botDO == nil || botDO.BotId != in.GetBotId() {
		return nil, mtproto.ErrBotInvalid
	}
	if botDO.CreatorUserId <= 0 {
		if c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - bot(%d) has no authoritative creator", in.GetBotId())
		}
		return nil, mtproto.ErrMethodNotImpl
	}
	if botDO.CreatorUserId != in.GetUserId() {
		if c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - user(%d) is not creator of bot(%d)", in.GetUserId(), in.GetBotId())
		}
		return nil, mtproto.ErrForbiddenUserBotInvalid
	}

	result := sqlx.TxWrapper(c.ctx, c.svcCtx.Dao.DB, func(tx *sqlx.Tx, storeResult *sqlx.StoreResult) {
		if _, storeResult.Err = c.svcCtx.Dao.BotCommandsDAO.DeleteTx(tx, in.GetBotId()); storeResult.Err != nil {
			return
		}
		if len(doList) > 0 {
			_, _, storeResult.Err = c.svcCtx.Dao.BotCommandsDAO.InsertBulkTx(tx, doList)
		}
	})
	if result.Err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("user.setBotCommands - replace bot(%d) commands error: %v", in.GetBotId(), result.Err)
		}
		return nil, result.Err
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
