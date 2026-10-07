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

package mysql_dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/zeromicro/go-zero/core/logx"
)

func (dao *BotsDAO) InsertRegistryTx(tx *sqlx.Tx, botId, creatorUserId, managerBotId int64, token string) error {
	_, err := tx.Exec(
		"insert into bots(bot_id, creator_user_id, manager_bot_id, token) values (?, ?, ?, ?)",
		botId,
		creatorUserId,
		managerBotId,
		token,
	)
	return err
}

func (dao *BotsDAO) SelectBotIdsByCreatorUserId(ctx context.Context, creatorUserId int64) ([]int64, error) {
	var rows []struct {
		BotId int64 `db:"bot_id"`
	}
	if err := dao.db.QueryRowsPartial(ctx, &rows,
		"SELECT bot_id FROM bots WHERE creator_user_id = ? ORDER BY bot_id", creatorUserId); err != nil {
		logx.WithContext(ctx).Errorf("select bot ids by creator user id, error: %v", err)
		return nil, err
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.BotId)
	}
	return ids, nil
}
