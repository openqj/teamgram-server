/*
 * WARNING! All changes made in this file will be lost!
 *   Created from by 'dalgen'
 *
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package mysql_dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// CountByMediaType counts exactly the same stored media rows used by
// MessageSearchByMediaType. Photo/video spans its combined and individual
// stored media classifications.
func (dao *MessagesDAO) CountByMediaType(ctx context.Context, userID, dialogID1, dialogID2 int64, mediaType int32) (int32, error) {
	var (
		count int32
		query string
		args  []interface{}
	)

	if mediaType == mtproto.MEDIA_PHOTOVIDEO {
		query = "select count(id) from " + dao.CalcTableName(userID) + " where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and message_filter_type in (?, ?, ?) and deleted = 0"
		args = []interface{}{userID, dialogID1, dialogID2, mtproto.MEDIA_PHOTOVIDEO, mtproto.MEDIA_PHOTOS_ONLY, mtproto.MEDIA_VIDEOS_ONLY}
	} else {
		query = "select count(id) from " + dao.CalcTableName(userID) + " where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and message_filter_type = ? and deleted = 0"
		args = []interface{}{userID, dialogID1, dialogID2, mediaType}
	}

	if err := dao.db.QueryRow(ctx, &count, query, args...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in CountByMediaType(_), error: %v", err)
		return 0, err
	}
	return count, nil
}

// SelectByPhotoVideoMediaType
// select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from messages where user_id = :user_id and (dialog_id1 = :dialog_id1 and dialog_id2 = :dialog_id2) and message_filter_type in (0, 7, 8) and user_message_box_id < :user_message_box_id and deleted = 0 order by user_message_box_id desc limit :limit
func (dao *MessagesDAO) SelectByPhotoVideoMediaType(ctx context.Context, userId int64, dialogId1 int64, dialogId2 int64, userMessageBoxId int32, limit int32) (rList []dataobject.MessagesDO, err error) {
	var (
		query  = "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from " + dao.CalcTableName(userId) + " where user_id = ? and (dialog_id1 = ? and dialog_id2 = ?) and message_filter_type in (0, 7, 8) and user_message_box_id < ? and deleted = 0 order by user_message_box_id desc limit ?"
		values []dataobject.MessagesDO
	)
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId, dialogId1, dialogId2, userMessageBoxId, limit)

	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectByMediaType(_), error: %v", err)
		return
	}

	rList = values

	return
}

// SelectByPhotoVideoMediaTypeWithCB
// select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from messages where user_id = :user_id and (dialog_id1 = :dialog_id1 and dialog_id2 = :dialog_id2) and message_filter_type in (0, 7, 8) and user_message_box_id < :user_message_box_id and deleted = 0 order by user_message_box_id desc limit :limit
func (dao *MessagesDAO) SelectByPhotoVideoMediaTypeWithCB(ctx context.Context, userId int64, dialogId1 int64, dialogId2 int64, messageFilterType int32, userMessageBoxId int32, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	var (
		query  = "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from " + dao.CalcTableName(userId) + " where user_id = ? and (dialog_id1 = ? and dialog_id2 = ?) and message_filter_type in (0, 7, 8) and user_message_box_id < ? and deleted = 0 order by user_message_box_id desc limit ?"
		values []dataobject.MessagesDO
	)
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId, dialogId1, dialogId2, userMessageBoxId, limit)

	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectByMediaType(_), error: %v", err)
		return
	}

	rList = values

	if cb != nil {
		sz := len(rList)
		for i := range sz {
			cb(sz, i, &rList[i])
		}
	}

	return
}
