// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"

	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
)

func (d *Dao) updateStoreAvailable() error {
	if d == nil || d.Postgres == nil || d.Pool == nil {
		return errors.New("sync: PostgreSQL update store is not configured")
	}
	return nil
}

func addSeqOn(ctx context.Context, tx pgx.Tx, authID, userID int64, updateType int32, updateData []byte) (int32, int64, error) {
	key := counter.SeqKey(authID)
	// Serialize the sequence counter and its journal row on the same
	// transaction-scoped advisory lock. The lock also orders writers that have
	// not initialized the idgen_counters row yet.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		return 0, 0, err
	}
	value, err := counter.NextOn(ctx, tx, key, 1)
	if err != nil {
		return 0, 0, err
	}
	var date int64
	err = tx.QueryRow(ctx, `INSERT INTO auth_seq_updates
 (auth_id,user_id,seq,update_type,update_data,date2)
 VALUES ($1,$2,$3,$4,$5,GREATEST(floor(extract(epoch FROM clock_timestamp()))::bigint,
 COALESCE((SELECT date2 FROM auth_seq_updates WHERE auth_id=$1 ORDER BY seq DESC LIMIT 1),0)))
 RETURNING date2`, authID, userID, int32(value), updateType, string(updateData)).Scan(&date)
	if err != nil {
		return 0, 0, err
	}
	return int32(value), date, nil
}

// AddSeqToUpdatesQueue advances the auth counter only with its committed event.
func (d *Dao) AddSeqToUpdatesQueue(ctx context.Context, authID, userID int64, updateType int32, updateData []byte) (int32, error) {
	if err := d.updateStoreAvailable(); err != nil {
		return 0, err
	}
	if authID == 0 || userID <= 0 || len(updateData) == 0 {
		return 0, mtproto.ErrInputRequestInvalid
	}
	var seq int32
	err := postgres.WithTx(ctx, d.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var err error
		seq, _, err = addSeqOn(ctx, tx, authID, userID, updateType, updateData)
		return err
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}

func addPtsOn(ctx context.Context, tx pgx.Tx, userID int64, pts, ptsCount int32, update *mtproto.Update) (int32, error) {
	if userID <= 0 || pts <= 0 || ptsCount <= 0 || update == nil {
		return 0, mtproto.ErrInputRequestInvalid
	}
	data, err := jsonx.Marshal(update)
	if err != nil {
		return 0, err
	}
	var date int64
	if err := tx.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp()))::bigint`).Scan(&date); err != nil {
		return 0, err
	}
	id, _, err := postgres_dao.NewUserPtsUpdatesDAO(tx).Insert(ctx, &dataobject.UserPtsUpdatesDO{
		UserId: userID, Pts: pts, PtsCount: ptsCount, UpdateType: mtproto.GetUpdateType(update), UpdateData: string(data), Date2: date,
	})
	return int32(id), err
}

func (d *Dao) AddToPtsQueue(ctx context.Context, userID int64, pts, ptsCount int32, update *mtproto.Update) (int32, error) {
	if err := d.updateStoreAvailable(); err != nil {
		return 0, err
	}
	var id int32
	err := postgres.WithTx(ctx, d.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var err error
		id, err = addPtsOn(ctx, tx, userID, pts, ptsCount, update)
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

type PreparedUpdates struct {
	AuthUpdates map[int64]*mtproto.Updates `json:"auth_updates"`
	Completed   bool                       `json:"-"`
}

func updateEvents(userID int64, updates *mtproto.Updates) ([]*mtproto.Update, error) {
	if userID <= 0 || updates == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	switch updates.GetPredicateName() {
	case mtproto.Predicate_updates, mtproto.Predicate_updatesCombined:
		for _, update := range updates.GetUpdates() {
			if update == nil {
				return nil, mtproto.ErrInputRequestInvalid
			}
		}
		return updates.GetUpdates(), nil
	case mtproto.Predicate_updateShort:
		if updates.GetUpdate() == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return []*mtproto.Update{updates.GetUpdate()}, nil
	case mtproto.Predicate_updateShortMessage, mtproto.Predicate_updateShortChatMessage:
		var events []*mtproto.Update
		mtproto.VisitUpdates(userID, updates, map[string]mtproto.UpdateVisitedFunc{
			mtproto.Predicate_updateNewMessage: func(_ int64, update *mtproto.Update, _ []*mtproto.User, _ []*mtproto.Chat, _ int32) {
				events = append(events, update)
			},
		})
		return events, nil
	case mtproto.Predicate_updatesTooLong, mtproto.Predicate_updateShortSentMessage, mtproto.Predicate_updateAccountResetAuthorization:
		return nil, nil
	default:
		return nil, mtproto.ErrInputRequestInvalid
	}
}

func commonPtsEvent(update *mtproto.Update) bool {
	switch mtproto.GetUpdateType(update) {
	case mtproto.PTS_UPDATE_NEW_MESSAGE, mtproto.PTS_UPDATE_DELETE_MESSAGES,
		mtproto.PTS_UPDATE_READ_HISTORY_OUTBOX, mtproto.PTS_UPDATE_READ_HISTORY_INBOX,
		mtproto.PTS_UPDATE_WEBPAGE, mtproto.PTS_UPDATE_READ_MESSAGE_CONENTS,
		mtproto.PTS_UPDATE_EDIT_MESSAGE, mtproto.PTS_UPDATE_FOLDER_PEERS, mtproto.PTS_UPDATE_PINNED_MESSAGES:
		return true
	}
	return false
}

func seqEvent(update *mtproto.Update) bool {
	if commonPtsEvent(update) || update.GetPts_INT32() != 0 || update.GetPts_FLAGINT32() != nil || update.GetQts() != 0 {
		return false
	}
	switch update.GetPredicateName() {
	case mtproto.Predicate_updateUserTyping, mtproto.Predicate_updateChatUserTyping,
		mtproto.Predicate_updateChannelUserTyping, mtproto.Predicate_updateEncryptedChatTyping,
		mtproto.Predicate_updateUserStatus, mtproto.Predicate_updateChannelTooLong:
		return false
	}
	return true
}

// PrepareUpdates journals every active permanent authorization, including
// offline devices. Network delivery starts only after this transaction commits.
func (d *Dao) PrepareUpdates(ctx context.Context, userID int64, updates *mtproto.Updates, onlyAuthID int64, excludes []int64) (*PreparedUpdates, error) {
	if err := d.updateStoreAvailable(); err != nil {
		return nil, err
	}
	events, err := updateEvents(userID, updates)
	if err != nil {
		return nil, err
	}
	if excludes == nil {
		excludes = []int64{}
	}
	prepared := &PreparedUpdates{AuthUpdates: make(map[int64]*mtproto.Updates)}
	err = postgres.WithTx(ctx, d.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if existing, err := loadDeliveryReceipt(ctx, tx, userID); err != nil {
			return err
		} else if existing != nil {
			prepared = existing
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT au.auth_key_id FROM auth_users au
 JOIN auth_key_infos ai ON ai.auth_key_id=au.auth_key_id AND ai.auth_key_type=$4 AND NOT ai.deleted
 JOIN auth_keys ak ON ak.auth_key_id=au.auth_key_id AND NOT ak.deleted
 WHERE au.user_id=$1 AND NOT au.deleted AND ($2::bigint=0 OR au.auth_key_id=$2)
 AND NOT (au.auth_key_id=ANY($3::bigint[])) ORDER BY au.auth_key_id FOR SHARE OF au,ai,ak`, userID, onlyAuthID, excludes, mtproto.AuthKeyTypePerm)
		if err != nil {
			return err
		}
		var authIDs []int64
		for rows.Next() {
			var authID int64
			if err := rows.Scan(&authID); err != nil {
				rows.Close()
				return err
			}
			authIDs = append(authIDs, authID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if onlyAuthID != 0 && len(authIDs) == 0 {
			return mtproto.ErrAuthKeyUnregistered
		}
		var seqEvents []*mtproto.Update
		for _, update := range events {
			if commonPtsEvent(update) && update.GetPtsCount() != 0 {
				if _, err := addPtsOn(ctx, tx, userID, update.GetPts_INT32(), update.GetPtsCount(), update); err != nil {
					return err
				}
			} else if seqEvent(update) {
				seqEvents = append(seqEvents, update)
			}
		}
		forwardAuthID := updates.GetAuthKeyId()
		if forwardAuthID != 0 {
			if len(authIDs) != 1 || authIDs[0] != forwardAuthID {
				return mtproto.ErrAuthKeyUnregistered
			}
			if len(seqEvents) > 0 {
				start := updates.GetSeqStart()
				if start <= 0 || updates.GetSeq()-start+1 != int32(len(seqEvents)) {
					return mtproto.ErrInputRequestInvalid
				}
				for i, update := range seqEvents {
					data, err := jsonx.Marshal(update)
					if err != nil {
						return err
					}
					var matches bool
					if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_seq_updates
 WHERE auth_id=$1 AND user_id=$2 AND seq=$3 AND update_type=$4 AND update_data::jsonb=$5::jsonb)`,
						forwardAuthID, userID, start+int32(i), mtproto.GetUpdateType(update), string(data)).Scan(&matches); err != nil {
						return err
					}
					if !matches {
						return errors.New("sync: forwarded seq update is not committed")
					}
				}
			}
			prepared.AuthUpdates[forwardAuthID] = proto.Clone(updates).(*mtproto.Updates)
			return saveDeliveryReceipt(ctx, tx, userID, prepared)
		}
		for _, authID := range authIDs {
			out := proto.Clone(updates).(*mtproto.Updates)
			out.Seq, out.SeqStart = 0, 0
			for i, update := range seqEvents {
				data, err := jsonx.Marshal(update)
				if err != nil {
					return err
				}
				seq, date, err := addSeqOn(ctx, tx, authID, userID, mtproto.GetUpdateType(update), data)
				if err != nil {
					return err
				}
				if i == 0 {
					out.SeqStart = seq
				}
				out.Seq, out.Date = seq, int32(date)
			}
			if len(seqEvents) > 0 {
				if out.GetPredicateName() == mtproto.Predicate_updateShort {
					out.Update = nil
					out.Updates = []*mtproto.Update{proto.Clone(updates.GetUpdate()).(*mtproto.Update)}
					out = mtproto.MakeTLUpdates(out).To_Updates()
				}
				if len(seqEvents) > 1 {
					out = mtproto.MakeTLUpdatesCombined(out).To_Updates()
				}
			}
			prepared.AuthUpdates[authID] = out
		}
		if err := saveDeliveryReceipt(ctx, tx, userID, prepared); err != nil {
			return fmt.Errorf("sync: persist delivery receipt: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return prepared, nil
}
