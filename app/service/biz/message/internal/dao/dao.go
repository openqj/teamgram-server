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
	return d.selectBackwardSavedMessages(ctx, userID, peerType, peerID, offset, limit)
}

func (d *Dao) SelectForwardSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64, offset, limit int32) ([]dataobject.MessagesDO, error) {
	return d.selectForwardSavedMessages(ctx, userID, peerType, peerID, offset, limit)
}

func (d *Dao) CountSavedMessages(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	return d.countSavedMessages(ctx, userID, peerType, peerID)
}

// PostgreSQL message accessors. These keep service handlers independent from
// the legacy sharded DAO while preserving the generated method contracts.
func (d *Dao) SelectMessageById(ctx context.Context, userID int64, id int32) (*dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageId(ctx, userID, id)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageByIdList(ctx context.Context, userID int64, ids []int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageIdList(ctx, userID, ids)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageByDataIdList(ctx context.Context, userID int64, ids []int64) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageDataIdList(ctx, userID, ids)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageByDataIdUserIdList(ctx context.Context, dialogMessageID int64, ids []int64) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageDataIdUserIdList(ctx, dialogMessageID, ids)
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
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectPeerUserMessageId(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	return d.SelectPeerUserMessage(ctx, peerID, userID, messageID)
}

func (d *Dao) CountMessageHistory(ctx context.Context, userID, dialogID1, dialogID2 int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountHistory(ctx, userID, dialogID1, dialogID2)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) CountMessageMedia(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountByMediaType(ctx, userID, dialogID1, dialogID2, mediaType)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) CountUnreadMentions(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountUnreadMentions(ctx, userID, peerType, peerID)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectBackwardHistory(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectForwardHistory(ctx context.Context, userID, dialogID1, dialogID2 int64, offsetID, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardByOffsetIdLimit(ctx, userID, dialogID1, dialogID2, offsetID, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectBackwardHistoryByDate(ctx context.Context, userID, dialogID1, dialogID2 int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectBackwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, date2, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectForwardHistoryByDate(ctx context.Context, userID, dialogID1, dialogID2 int64, date2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectForwardByOffsetDateLimit(ctx, userID, dialogID1, dialogID2, date2, limit)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectPinnedMessageIDs(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]int32, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectPinnedMessageIdList(ctx, userID, dialogID1, dialogID2)
	}
	return nil, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) UpdatePinnedMessage(ctx context.Context, pinned bool, userID int64, messageID int32) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdatePinned(ctx, pinned, userID, messageID)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) UnpinMessages(ctx context.Context, userID int64, ids []int32) (int64, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateUnPinnedByIdList(ctx, userID, ids)
	}
	return 0, errors.New("biz/message: postgres message store is not configured")
}

func (d *Dao) SelectMessageReadOutbox(ctx context.Context, userID, readUserID int64, maxID int32) ([]dataobject.MessageReadOutboxDO, error) {
	if d != nil && d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.MessageReadOutbox != nil {
		return d.Postgres.Store.MessageReadOutbox.SelectList(ctx, userID, readUserID, maxID)
	}
	return nil, errors.New("biz/message: postgres message read-outbox store is not configured")
}
