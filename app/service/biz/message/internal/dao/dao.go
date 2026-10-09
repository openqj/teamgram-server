/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"context"
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/plugin"
)

// Dao dao.
type Dao struct {
	*Mysql
	*Postgres
	sqlc.CachedConn
	Plugin plugin.MessagePlugin
}

// HasLegacyStore reports whether a test or migration-boundary caller supplied
// the old MySQL message DAO. Production constructors leave this nil because
// PostgreSQL is authoritative.
func (d *Dao) HasLegacyStore() bool {
	return d != nil && d.Mysql != nil && d.MessagesDAO != nil && d.CommonDAO != nil
}

// New new a dao and return.
func New(c config.Config, plugin plugin.MessagePlugin) *Dao {
	if c.Postgres.DSN == "" {
		panic(errors.New("biz/message: Postgres.DSN is required"))
	}
	dao := &Dao{Plugin: plugin}
	pg, err := NewPostgres(c.Postgres)
	if err != nil {
		panic(err)
	}
	dao.Postgres = pg
	return dao
}

func (d *Dao) SelectByMessageIDListForSearch(ctx context.Context, userID int64, ids []int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageIdList(ctx, userID, ids)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SearchMessagesForSearch(ctx context.Context, userID, dialogID1, dialogID2 int64, offset int32, q string, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.Search(ctx, userID, dialogID1, dialogID2, offset, q, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SearchGlobalMessagesForSearch(ctx context.Context, userID int64, offset int32, q string, limit int32) ([]dataobject.MessagesDO, error) {
	return d.searchGlobalMessages(ctx, userID, offset, q, limit)
}

func (d *Dao) SelectPeerHashTagList(ctx context.Context, userID int64, peerType int32, peerID int64, hashTag string) ([]int32, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.HashTags != nil {
		return d.Postgres.Store.HashTags.SelectPeerHashTagList(ctx, userID, peerType, peerID, hashTag)
	}
	return nil, errors.New("biz/message: postgres hash-tag store is not configured")
}

func (d *Dao) searchGlobalMessages(ctx context.Context, userID int64, offset int32, q string, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SearchGlobal(ctx, userID, offset, q, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) selectMediaMessages(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMediaType(ctx, userID, dialogID1, dialogID2, mediaType, offset, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMediaMessages(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32, offset, limit int32) ([]dataobject.MessagesDO, error) {
	return d.selectMediaMessages(ctx, userID, dialogID1, dialogID2, mediaType, offset, limit)
}

func (d *Dao) selectPhotoVideoMessages(ctx context.Context, userID, dialogID1, dialogID2 int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByPhotoVideoMediaType(ctx, userID, dialogID1, dialogID2, offset, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectPhotoVideoMessages(ctx context.Context, userID, dialogID1, dialogID2 int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	return d.selectPhotoVideoMessages(ctx, userID, dialogID1, dialogID2, offset, limit)
}

func (d *Dao) selectSentMediaMessages(ctx context.Context, userID int64, mediaType int32, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectSentByMediaType(ctx, userID, mediaType, offset, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectSentMediaMessages(ctx context.Context, userID int64, mediaType int32, offset, limit int32) ([]dataobject.MessagesDO, error) {
	return d.selectSentMediaMessages(ctx, userID, mediaType, offset, limit)
}

func (d *Dao) selectPhoneCallMessages(ctx context.Context, userID int64, mediaType int32, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectPhoneCallList(ctx, userID, mediaType, offset, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectPhoneCallMessages(ctx context.Context, userID int64, mediaType int32, offset, limit int32) ([]dataobject.MessagesDO, error) {
	return d.selectPhoneCallMessages(ctx, userID, mediaType, offset, limit)
}

func (d *Dao) selectBackwardSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardSavedByOffsetIdLimit(ctx, userID, peerType, peerID, offset, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) selectForwardSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardSavedByOffsetIdLimit(ctx, userID, peerType, peerID, offset, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) countSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountSaved(ctx, userID, peerType, peerID)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectBackwardSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.selectBackwardSavedMessages(ctx, userID, peerType, peerID, offset, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectBackwardSavedByOffsetIdLimit(ctx, userID, peerType, peerID, offset, limit)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) SelectForwardSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.selectForwardSavedMessages(ctx, userID, peerType, peerID, offset, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectForwardSavedByOffsetIdLimit(ctx, userID, peerType, peerID, offset, limit)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) CountSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.countSavedMessages(ctx, userID, peerType, peerID)
	}
	if d != nil && d.CommonDAO != nil && d.MessagesDAO != nil {
		return int64(d.CommonDAO.CalcSize(ctx, d.MessagesDAO.CalcTableName(userID), map[string]interface{}{"user_id": userID, "saved_peer_type": peerType, "saved_peer_id": peerID, "deleted": 0})), nil
	}
	return 0, errors.New("biz/message: message store is not configured")
}

// PostgreSQL message accessors. These keep service handlers independent from
// the legacy sharded DAO while preserving the generated method contracts.
func (d *Dao) SelectMessageById(ctx context.Context, userID int64, id int32) (*dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageId(ctx, userID, id)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectByMessageId(ctx, userID, id)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageByIdList(ctx context.Context, userID int64, ids []int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageIdList(ctx, userID, ids)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectByMessageIdList(ctx, userID, ids)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageByDataIdList(ctx context.Context, userID int64, ids []int64) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageDataIdList(ctx, userID, ids)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectByMessageDataIdList(ctx, d.MessagesDAO.CalcTableName(userID), ids)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageByDataIdUserIdList(ctx context.Context, dialogMessageID int64, ids []int64) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageDataIdUserIdList(ctx, dialogMessageID, ids)
	}
	if d != nil && d.MessagesDAO != nil {
		if len(ids) == 0 {
			return []dataobject.MessagesDO{}, nil
		}
		byTable := make(map[string][]int64)
		for _, id := range ids {
			table := d.MessagesDAO.CalcTableName(id)
			byTable[table] = append(byTable[table], id)
		}
		result := make([]dataobject.MessagesDO, 0)
		for table, userIDs := range byTable {
			rows, err := d.MessagesDAO.SelectByMessageDataIdUserIdList(ctx, table, dialogMessageID, userIDs)
			if err != nil {
				return nil, err
			}
			result = append(result, rows...)
		}
		return result, nil
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

// SelectMessageByDataIdUserIdListForUsers returns the message view for each
// requested user. PostgreSQL keeps all message views in one relation, so no
// physical shard/table name is needed at the service boundary.
func (d *Dao) SelectMessageByDataIdUserIdListForUsers(ctx context.Context, dialogMessageID int64, userIDs []int64) ([]dataobject.MessagesDO, error) {
	return d.SelectMessageByDataIdUserIdList(ctx, dialogMessageID, userIDs)
}

func (d *Dao) SelectPeerUserMessage(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectPeerUserMessage(ctx, peerID, userID, messageID)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectPeerUserMessage(ctx, peerID, userID, messageID)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectPeerUserMessageId(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	return d.SelectPeerUserMessage(ctx, peerID, userID, messageID)
}

func (d *Dao) CountMessageHistory(ctx context.Context, userID, dialogID1, dialogID2 int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountHistory(ctx, userID, dialogID1, dialogID2)
	}
	if d != nil && d.CommonDAO != nil && d.MessagesDAO != nil {
		return int64(d.CommonDAO.CalcSize(ctx, d.MessagesDAO.CalcTableName(userID), map[string]interface{}{"user_id": userID, "dialog_id1": dialogID1, "dialog_id2": dialogID2, "deleted": 0})), nil
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) CountMessageMedia(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountByMediaType(ctx, userID, dialogID1, dialogID2, mediaType)
	}
	if d != nil && d.MessagesDAO != nil {
		count, err := d.MessagesDAO.CountByMediaType(ctx, userID, dialogID1, dialogID2, mediaType)
		return int64(count), err
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) CountUnreadMentions(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountUnreadMentions(ctx, userID, peerType, peerID)
	}
	if d != nil && d.CommonDAO != nil && d.MessagesDAO != nil {
		return int64(d.CommonDAO.CalcSize(ctx, d.MessagesDAO.CalcTableName(userID), map[string]interface{}{"user_id": userID, "peer_type": peerType, "peer_id": peerID, "mentioned": 1, "media_unread": 1, "deleted": 0})), nil
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectBackwardHistory(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectBackwardByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectForwardHistory(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectForwardByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectBackwardHistoryByDate(ctx context.Context, userID, dialogID1, dialogID2 int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, date2, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectBackwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, date2, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectForwardHistoryByDate(ctx context.Context, userID, dialogID1, dialogID2 int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, date2, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectForwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, date2, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectHistoryBySender(ctx context.Context, userID, dialogID1, dialogID2, senderUserID int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardBySendUserIdOffsetIdLimit(ctx, userID, dialogID1, dialogID2, senderUserID, offsetID, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectBackwardBySendUserIdOffsetIdLimit(ctx, userID, dialogID1, dialogID2, senderUserID, offsetID, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) CountHistoryBySender(ctx context.Context, userID, dialogID1, dialogID2, senderUserID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountHistoryBySender(ctx, userID, dialogID1, dialogID2, senderUserID)
	}
	if d != nil && d.CommonDAO != nil && d.MessagesDAO != nil {
		return int64(d.CommonDAO.CalcSize(ctx, d.MessagesDAO.CalcTableName(userID), map[string]interface{}{"user_id": userID, "dialog_id1": dialogID1, "dialog_id2": dialogID2, "sender_user_id": senderUserID, "deleted": 0})), nil
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectPinnedMessageIDs(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]int32, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectPinnedMessageIdList(ctx, userID, dialogID1, dialogID2)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectPinnedMessageIdList(ctx, userID, dialogID1, dialogID2)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) UpdatePinnedMessage(ctx context.Context, pinned bool, userID int64, messageID int32) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Pool != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		tx, err := d.Postgres.Pool.Begin(ctx)
		if err != nil {
			return 0, err
		}
		rows, err := d.Postgres.Store.Messages.UpdatePinnedOn(ctx, tx, pinned, userID, messageID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return rows, nil
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.UpdatePinned(ctx, pinned, userID, messageID)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) UnpinMessages(ctx context.Context, userID int64, ids []int32) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Pool != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		tx, err := d.Postgres.Pool.Begin(ctx)
		if err != nil {
			return 0, err
		}
		rows, err := d.Postgres.Store.Messages.UpdateUnPinnedByIdListOn(ctx, tx, userID, ids)
		if err != nil {
			_ = tx.Rollback(ctx)
			return 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return rows, nil
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.UpdateUnPinnedByIdList(ctx, userID, ids)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageReadOutbox(ctx context.Context, userID, readUserID int64, maxID int32) ([]dataobject.MessageReadOutboxDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.MessageReadOutbox != nil {
		return d.Postgres.Store.MessageReadOutbox.SelectList(ctx, userID, readUserID, maxID)
	}
	if d != nil && d.MessageReadOutboxDAO != nil {
		return d.MessageReadOutboxDAO.SelectList(ctx, userID, readUserID, maxID)
	}
	return nil, errors.New("biz/message: postgres message read-outbox store is not configured")
}

func (d *Dao) SelectGroupMessageReadOutbox(ctx context.Context, userID, peerDialogID int64, maxID int32) ([]dataobject.MessageReadOutboxDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.MessageReadOutbox != nil {
		return d.Postgres.Store.MessageReadOutbox.SelectGroupList(ctx, userID, peerDialogID, maxID)
	}
	return nil, errors.New("biz/message: postgres group read-outbox store is not configured")
}

func (d *Dao) SelectPinnedList(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectPinnedList(ctx, userID, dialogID1, dialogID2)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectPinnedList(ctx, userID, dialogID1, dialogID2)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) SelectLastTwoPinned(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]int32, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectLastTwoPinnedList(ctx, userID, dialogID1, dialogID2)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectLastTwoPinnedList(ctx, userID, dialogID1, dialogID2)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) SelectUnreadMentionsBackward(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, minID, maxID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, minID, maxID, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectBackwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) SelectUnreadMentionsForward(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, minID, maxID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, minID, maxID, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectForwardUnreadMentionsByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) SelectSavedBackwardByDate(ctx context.Context, userID int64, peerType int32, peerID int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardSavedByOffsetDateLimit(ctx, userID, peerType, peerID, date2, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectBackwardSavedByOffsetDateLimit(ctx, userID, peerType, peerID, date2, limit)
	}
	return nil, errors.New("biz/message: message store is not configured")
}

func (d *Dao) SelectSavedForwardByDate(ctx context.Context, userID int64, peerType int32, peerID int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardSavedByOffsetDateLimit(ctx, userID, peerType, peerID, date2, limit)
	}
	if d != nil && d.MessagesDAO != nil {
		return d.MessagesDAO.SelectForwardSavedByOffsetDateLimit(ctx, userID, peerType, peerID, date2, limit)
	}
	return nil, errors.New("biz/message: message store is not configured")
}
