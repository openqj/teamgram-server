// Copyright 2022 Teamgram Authors
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

package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

var (
	contactsBlockPeerPrefix = "user_block_peer"
)

type CachedPeerBlocked struct {
	PeerBlocked *mtproto.PeerBlocked `json:"peer_blocked"`
}

func (c *CachedPeerBlocked) IsEmpty() bool {
	if c == nil {
		return true
	}

	return c.PeerBlocked == nil
}

func genContactsBlockPeerCacheKey(id, blockedId int64) string {
	return fmt.Sprintf("%s_%d_%d", contactsBlockPeerPrefix, id, blockedId)
}

func (d *Dao) CheckBlocked(ctx context.Context, id, blockedId int64) (bool, error) {
	if d.Postgres != nil {
		do, err := d.Postgres.Store.PeerBlocks.Select(ctx, id, mtproto.PEER_USER, blockedId)
		if err != nil {
			return false, err
		}
		return do != nil && !do.Deleted, nil
	}
	var (
		blocked = new(CachedPeerBlocked)
	)
	err := d.CachedConn.QueryRow(
		ctx,
		blocked,
		genContactsBlockPeerCacheKey(id, blockedId),
		func(ctx context.Context, conn *sqlx.DB, v interface{}) error {
			do, err := d.UserPeerBlocksDAO.Select(ctx, id, mtproto.PEER_USER, blockedId)
			if err != nil {
				return err
			}
			if do != nil {
				v.(*CachedPeerBlocked).PeerBlocked = mtproto.MakeTLPeerBlocked(&mtproto.PeerBlocked{
					PeerId: mtproto.MakePeerUser(do.PeerId),
					Date:   int32(do.Date),
				}).To_PeerBlocked()
			} else {
				return sqlc.ErrNotFound
			}

			return nil
		},
	)

	if errors.Is(err, sqlc.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !blocked.IsEmpty(), nil
}

func (d *Dao) BlockUser(ctx context.Context, id, blockId int64) error {
	if d.Postgres != nil {
		return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			if err := lockUserForBlock(ctx, tx, id); err != nil {
				return err
			}
			_, _, err := d.Postgres.Store.PeerBlocks.InsertOrUpdateTx(ctx, tx, &dataobject.UserPeerBlocksDO{UserId: id, PeerType: mtproto.PEER_USER, PeerId: blockId, Date: time.Now().Unix()})
			return err
		})
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			return d.UserPeerBlocksDAO.InsertOrUpdate(ctx, &dataobject.UserPeerBlocksDO{
				UserId:   id,
				PeerType: mtproto.PEER_USER,
				PeerId:   blockId,
				Date:     time.Now().Unix(),
			})
		},
		genContactsBlockPeerCacheKey(id, blockId))

	return err
}

func (d *Dao) UnBlockUser(ctx context.Context, id, unblockId int64) error {
	if d.Postgres != nil {
		return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			if err := lockUserForBlock(ctx, tx, id); err != nil {
				return err
			}
			_, err := d.Postgres.Store.PeerBlocks.DeleteTx(ctx, tx, id, mtproto.PEER_USER, unblockId)
			return err
		})
	}
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			affected, err := d.UserPeerBlocksDAO.Delete(
				ctx,
				id,
				mtproto.PEER_USER,
				unblockId)
			return 0, affected, err
		},
		genContactsBlockPeerCacheKey(id, unblockId))

	return err
}

func lockUserForBlock(ctx context.Context, tx pgx.Tx, id int64) error {
	if id <= 0 {
		return mtproto.ErrUserIdInvalid
	}
	var userID int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 AND deleted=FALSE AND user_type NOT IN(0,1) FOR UPDATE`, id).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return mtproto.ErrUserIdInvalid
	}
	return err
}
