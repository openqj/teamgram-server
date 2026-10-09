// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserEditCloseFriends
// user.editCloseFriends user_id:long id:Vector<long> = Bool;
func (c *UserCore) UserEditCloseFriends(in *user.TLUserEditCloseFriends) (*mtproto.Bool, error) {
	err := c.svcCtx.Dao.Postgres.InTx(c.ctx, func(tx pgx.Tx) error {
		var userID int64
		if err := tx.QueryRow(c.ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, in.UserId).Scan(&userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		if _, err := tx.Exec(c.ctx, `UPDATE user_contacts SET close_friend=FALSE WHERE owner_user_id=$1 AND close_friend=TRUE`, in.UserId); err != nil {
			return err
		}
		if len(in.Id) > 0 {
			_, err := c.svcCtx.Dao.Postgres.Store.Contacts.UpdateCloseFriendTx(c.ctx, tx, true, in.UserId, in.Id)
			return err
		}
		return nil
	})
	if err != nil {
		return mtproto.BoolFalse, err
	}

	return mtproto.BoolTrue, nil
}
