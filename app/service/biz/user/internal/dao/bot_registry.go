// Copyright 2026 Teamgram Authors
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (d *Dao) CreateBot(ctx context.Context, creatorUserId, managerBotId, managerAccessHash int64, name, username string) (*mtproto.ImmutableUser, error) {
	if managerBotId < 0 || (managerBotId == 0 && managerAccessHash != 0) || (managerBotId > 0 && managerAccessHash == 0) {
		return nil, mtproto.ErrUserIdInvalid
	}
	var accessHashBytes [8]byte
	if _, err := cryptorand.Read(accessHashBytes[:]); err != nil {
		return nil, err
	}
	accessHash := int64(binary.LittleEndian.Uint64(accessHashBytes[:]) & 0x7fffffffffffffff)
	if accessHash == 0 {
		accessHash = 1
	}
	var phoneBytes [12]byte
	if _, err := cryptorand.Read(phoneBytes[:]); err != nil {
		return nil, err
	}
	phone := "bot-" + hex.EncodeToString(phoneBytes[:])

	var botId int64
	txResult := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		var creator struct {
			UserType int32 `db:"user_type"`
			Deleted  bool  `db:"deleted"`
		}
		result.Err = tx.QueryRowPartial(&creator,
			"SELECT user_type, deleted FROM users WHERE id = ? FOR UPDATE", creatorUserId)
		if errors.Is(result.Err, sqlx.ErrNotFound) {
			result.Err = mtproto.ErrUserIdInvalid
			return
		}
		if result.Err != nil {
			return
		}
			if creator.Deleted || creator.UserType != user.UserTypeRegular {
			result.Err = mtproto.ErrForbiddenUserBotInvalid
			return
		}
		if managerBotId > 0 {
			var manager struct {
				BotId            int64 `db:"bot_id"`
				AccessHash       int64 `db:"access_hash"`
				UserType         int32 `db:"user_type"`
				Deleted          bool  `db:"deleted"`
				BotCanManageBots bool  `db:"bot_can_manage_bots"`
			}
			result.Err = tx.QueryRowPartial(&manager,
				"SELECT b.bot_id, u.access_hash, u.user_type, u.deleted, b.bot_can_manage_bots FROM bots b JOIN users u ON u.id = b.bot_id WHERE b.bot_id = ? FOR UPDATE", managerBotId)
			if errors.Is(result.Err, sqlx.ErrNotFound) {
				result.Err = mtproto.ErrForbiddenUserBotInvalid
				return
			}
			if result.Err != nil {
				return
			}
			if manager.BotId != managerBotId || manager.AccessHash != managerAccessHash || manager.Deleted || manager.UserType != user.UserTypeBot {
				result.Err = mtproto.ErrUserIdInvalid
				return
			}
			if !manager.BotCanManageBots {
				result.Err = mtproto.ErrForbiddenUserBotInvalid
				return
			}
		}

		var existing dataobject.UsernameDO
		err := tx.QueryRowPartial(&existing,
			"SELECT username, peer_type, peer_id FROM username WHERE username = ? FOR UPDATE", username)
		if err == nil {
			result.Err = mtproto.ErrUsernameOccupied
			return
		}
		if !errors.Is(err, sqlx.ErrNotFound) {
			result.Err = err
			return
		}

		userDO := &dataobject.UsersDO{
			UserType:    user.UserTypeBot,
			AccessHash:  accessHash,
			FirstName:   name,
			Username:    username,
			Phone:       phone,
			CountryCode: "BOT",
			IsBot:       true,
		}
		botId, _, result.Err = d.UsersDAO.InsertTx(tx, userDO)
		if result.Err != nil {
			return
		}

		_, _, result.Err = d.UsernameDAO.InsertTx(tx, &dataobject.UsernameDO{
			Username: username,
			PeerType: mtproto.PEER_USER,
			PeerId:   botId,
			Editable: true,
			Active:   true,
			Order2:   time.Now().Unix() << 32,
		})
		if sqlx.IsDuplicate(result.Err) {
			result.Err = mtproto.ErrUsernameOccupied
		}
		if result.Err != nil {
			return
		}

		token, err := newBotToken(botId)
		if err != nil {
			result.Err = err
			return
		}
		result.Err = d.BotsDAO.InsertRegistryTx(tx, botId, creatorUserId, managerBotId, token)
	})
	if txResult.Err != nil {
		return nil, txResult.Err
	}

	return d.GetImmutableUser(ctx, botId, false)
}

func (d *Dao) ExportBotToken(ctx context.Context, botId, creatorUserId int64, revoke bool) (string, error) {
	var token string
	txResult := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		var bot struct {
			CreatorUserId int64  `db:"creator_user_id"`
			Token         string `db:"token"`
		}
		result.Err = tx.QueryRowPartial(&bot,
			"SELECT creator_user_id, token FROM bots WHERE bot_id = ? FOR UPDATE", botId)
		if errors.Is(result.Err, sqlx.ErrNotFound) {
			result.Err = mtproto.ErrBotInvalid
			return
		}
		if result.Err != nil {
			return
		}
		if bot.CreatorUserId != creatorUserId {
			result.Err = mtproto.ErrForbiddenUserBotInvalid
			return
		}

		if revoke {
			token, result.Err = newBotToken(botId)
			if result.Err != nil {
				return
			}
			updateResult, err := tx.Exec("UPDATE bots SET token = ? WHERE bot_id = ?", token, botId)
			if err != nil {
				result.Err = err
				return
			}
			rowsAffected, err := updateResult.RowsAffected()
			if err != nil {
				result.Err = err
				return
			}
			if rowsAffected != 1 {
				result.Err = mtproto.ErrBotInvalid
			}
			return
		}

		token = bot.Token
		if token == "" {
			result.Err = mtproto.ErrTokenInvalid
		}
	})
	if txResult.Err != nil {
		return "", txResult.Err
	}
	return token, nil
}

func newBotToken(botId int64) (string, error) {
	var secret [32]byte
	if _, err := cryptorand.Read(secret[:]); err != nil {
		return "", err
	}
	return strconv.FormatInt(botId, 10) + ":" + base64.RawURLEncoding.EncodeToString(secret[:]), nil
}
