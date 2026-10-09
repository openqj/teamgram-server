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
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func makeMessageBoxByDO(boxDO *dataobject.MessagesDO) *mtproto.MessageBox {
	// TODO(@benqi): check boxDO and dataDO
	// message, _ := mtproto.DecodeMessage(int(boxDO.MessageType), hack.Bytes(boxDO.MessageData))

	box := &mtproto.MessageBox{
		UserId:            boxDO.UserId,
		SenderUserId:      boxDO.SenderUserId,
		PeerType:          boxDO.PeerType,
		PeerId:            boxDO.PeerId,
		MessageId:         boxDO.UserMessageBoxId,
		DialogId1:         boxDO.DialogId1,
		DialogId2:         boxDO.DialogId2,
		DialogMessageId:   boxDO.DialogMessageId,
		RandomId:          boxDO.RandomId,
		Pts:               0,
		PtsCount:          0,
		MessageFilterType: boxDO.MessageFilterType,
		Message:           nil,
	}
	jsonx.UnmarshalFromString(boxDO.MessageData, &box.Message)
	box.Message = box.Message.FixData()
	return box
}

func (d *Dao) sendMessageToOutbox(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, outboxMessage *msg.OutboxMessage) (*mtproto.MessageBox, bool, error) {
	if d.Postgres != nil && d.Postgres.Store != nil {
		return d.sendMessageToOutboxPostgres(ctx, fromId, peer, outboxMessage)
	}
	var (
		dialogId = mtproto.MakeDialogId(fromId, peer.PeerType, peer.PeerId)
		err      error
		message  = outboxMessage.Message
	)

	idList := d.IDGenClient2.GetNextIdList(
		ctx,
		idgen_client.MakeIDTypeNextId(),
		idgen_client.MakeIDTypeNgen(idgen_client.IDTypeMessageBox, fromId),
		idgen_client.MakeIDTypeNgen(idgen_client.IDTypePts, fromId))
	if len(idList) != 3 {
		err = mtproto.ErrInternalServerError
		return nil, false, err
	}

	dialogMessageId := idList[0].Id
	outBoxMsgId := int32(idList[1].Id)
	pts := int32(idList[2].Id)

	if dialogMessageId == 0 || outBoxMsgId == 0 || pts == 0 {
		err = mtproto.ErrInternalServerError
		return nil, false, err
	}

	var (
		dialogDO *dataobject.DialogsDO
	)

	// A, B
	tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		message.Out = true
		message.Id = outBoxMsgId
		// message.Mentioned = model.CheckHasMention(message.Entities, fromId)
		//if message.Mentioned {
		//	message.MediaUnread = true
		//}
		//if !message.MediaUnread {
		message.MediaUnread = mtproto.CheckHasMediaUnread(message)
		//}
		// Pts:              pts,
		// PtsCount:         ptsCount,
		// mType, mData := mtproto.EncodeMessage(message)
		mData, _ := jsonx.Marshal(message)
		outMsgBox := &mtproto.MessageBox{
			UserId:            fromId,
			SenderUserId:      fromId,
			PeerType:          peer.PeerType,
			PeerId:            peer.PeerId,
			MessageId:         outBoxMsgId,
			DialogId1:         dialogId.A,
			DialogId2:         dialogId.B,
			DialogMessageId:   dialogMessageId,
			RandomId:          outboxMessage.RandomId,
			Pts:               0,
			PtsCount:          0,
			MessageFilterType: mtproto.GetMediaType(message),
			Message:           message,
		}

		var (
			savedPeerUtil *mtproto.PeerUtil
		)
		if message.GetSavedPeerId() != nil {
			savedPeerUtil = mtproto.FromPeer(message.GetSavedPeerId())
		} else {
			savedPeerUtil = &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY, PeerId: 0}
		}

		outBoxDO := &dataobject.MessagesDO{
			UserId:            outMsgBox.UserId,
			UserMessageBoxId:  outMsgBox.MessageId,
			DialogId1:         outMsgBox.DialogId1,
			DialogId2:         outMsgBox.DialogId2,
			DialogMessageId:   outMsgBox.DialogMessageId,
			SenderUserId:      outMsgBox.UserId,
			PeerType:          peer.PeerType,
			PeerId:            peer.PeerId,
			RandomId:          outMsgBox.RandomId,
			MessageFilterType: outMsgBox.MessageFilterType,
			MessageData:       hack.String(mData),
			Message:           message.Message,
			Mentioned:         false,
			MediaUnread:       message.MediaUnread,
			Date2:             int64(outMsgBox.Message.Date),
			SavedPeerType:     savedPeerUtil.PeerType,
			SavedPeerId:       savedPeerUtil.PeerId,
			Deleted:           false,
		}

		lastInsertId, rowsAffected, err := d.MessagesDAO.InsertOrReturnIdTx(tx, outBoxDO)
		if err != nil {
			result.Err = err
			return
		}

		if rowsAffected == 0 {
			// TODO(@benqi): random_id已经存在
			if lastInsertId > 0 {
				result.Data = lastInsertId
				return
			} else {
				result.Err = errors.New("insert error")
				return
			}
		}

		//outMsgBox.Pts = d.IDGenClient2.NextPtsId(ctx, fromId)
		//outMsgBox.PtsCount = 1
		// logx.WithContext(ctx).Infof("sendMessage - (pts: %d, pts_count: %d)", outMsgBox.Pts, outMsgBox.PtsCount)

		switch peer.PeerType {
		case mtproto.PEER_USER:
			dialogDO = &dataobject.DialogsDO{
				UserId:           fromId,
				PeerType:         peer.PeerType,
				PeerId:           peer.PeerId,
				PeerDialogId:     mtproto.MakePeerDialogId(mtproto.PEER_USER, peer.PeerId),
				TopMessage:       outBoxMsgId,
				UnreadCount:      0,
				DraftMessageData: "null",
				Date2:            int64(outMsgBox.Message.Date),
			}

			result.Data = outMsgBox
		case mtproto.PEER_CHAT:
			dialogDO = &dataobject.DialogsDO{
				UserId:           fromId,
				PeerType:         peer.PeerType,
				PeerId:           peer.PeerId,
				PeerDialogId:     mtproto.MakePeerDialogId(mtproto.PEER_CHAT, peer.PeerId),
				TopMessage:       outBoxMsgId,
				UnreadCount:      0,
				DraftMessageData: "null",
				Date2:            int64(outMsgBox.Message.Date),
			}

			result.Data = outMsgBox
		default:
			result.Err = fmt.Errorf("fatal error - invalid peer_type: %v", peer)
			logx.WithContext(ctx).Errorf("%v", result)
			return
		}

		for _, entity := range message.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag {
				if entity.GetUrl() != "" {
					d.HashTagsDAO.InsertOrUpdateTx(tx, &dataobject.HashTagsDO{
						UserId:           outMsgBox.UserId,
						PeerType:         peer.PeerType,
						PeerId:           peer.PeerId,
						HashTag:          entity.GetUrl(),
						HashTagMessageId: outMsgBox.MessageId,
					})
				}
			}
		}

		_, err = d.AddToPtsQueueTx(tx, fromId, pts, 1, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: outMsgBox.Message,
			Pts_INT32:       pts,
			PtsCount:        1,
		}).To_Update())
		if err != nil {
			result.Err = err
			return
		}
	})

	if tR.Err != nil {
		return nil, false, tR.Err
	}

	var outBox *mtproto.MessageBox

	switch tR.Data.(type) {
	case *mtproto.MessageBox:
		// TODO: use rpc
		d.CachedConn.Exec(
			ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				rowsAffected, err2 := d.DialogsDAO.UpdateOutboxDialog(
					ctx,
					dialogDO.TopMessage,
					dialogDO.Date2,
					fromId,
					peer.PeerType,
					peer.PeerId)
				if err2 != nil {
					return 0, 0, err2
				}
				// again handle rowsAffected == 0
				if rowsAffected == 0 {
					_, _, err2 = d.InsertIgnoreDialog(ctx, dialogDO)
				}

				return 0, 0, nil
			},
			dialog.GetDialogCacheKeyByPeer(fromId, peer.PeerType, peer.PeerId))

		outBox = tR.Data.(*mtproto.MessageBox)
		outBox.Pts = pts // d.IDGenClient2.NextPtsId(ctx, fromId)
		outBox.PtsCount = 1

	case int64:
		if tR.Data.(int64) <= 0 {
			logx.WithContext(ctx).Error("unknown error")
			return nil, false, errors.New("fatal unknown error")
		}

		// dup, we'll recreate box
		do, err := d.SelectMessageByRandomID(ctx, fromId, outboxMessage.RandomId)
		if err != nil {
			return nil, false, err
		}
		if do != nil {
			outBox = makeMessageBoxByDO(do)
			outBox.Pts = d.IDGenClient2.CurrentPtsId(ctx, outBox.UserId)
			outBox.PtsCount = 1
			return outBox, false, nil
		} else {
			logx.WithContext(ctx).Error("unknown error")
			return nil, false, errors.New("fatal unknown error")
		}
	default:
		logx.WithContext(ctx).Error("unknown error")
		return nil, false, errors.New("fatal unknown error")
	}

	return outBox, true, nil
}

// sendMessageToOutboxPostgres persists the message view, dialog projection,
// hashtag rows, and pts update atomically. It is the runtime path used by the
// PostgreSQL-only service constructor; the generated MySQL implementation
// above remains only as a migration-boundary fallback for tests.
func (d *Dao) sendMessageToOutboxPostgres(ctx context.Context, fromID int64, peer *mtproto.PeerUtil, outboxMessage *msg.OutboxMessage) (*mtproto.MessageBox, bool, error) {
	var box *mtproto.MessageBox
	var inserted bool
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		box, inserted, err = d.sendMessageToOutboxPostgresOn(ctx, tx, fromID, peer, outboxMessage)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return box, inserted, nil
}

func (d *Dao) sendMessageToOutboxPostgresOn(ctx context.Context, tx pgx.Tx, fromID int64, peer *mtproto.PeerUtil, outboxMessage *msg.OutboxMessage) (*mtproto.MessageBox, bool, error) {
	if outboxMessage == nil || outboxMessage.Message == nil || peer == nil {
		return nil, false, mtproto.ErrMessageEmpty
	}
	if peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_CHAT {
		return nil, false, mtproto.ErrPeerIdInvalid
	}
	// Serialize a sender's message IDs, dialog projections and update writes.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('messenger-user:' || $1::bigint::text, 0))`, fromID); err != nil {
		return nil, false, err
	}
	if outboxMessage.RandomId != 0 {
		existing, err := d.Postgres.Store.Messages.SelectByRandomIdOn(ctx, tx, fromID, fromID, outboxMessage.RandomId)
		if err != nil {
			return nil, false, err
		}
		if existing != nil {
			box := makeMessageBoxByDO(existing)
			if err := d.loadMessagePtsPostgres(ctx, tx, box); err != nil {
				return nil, false, err
			}
			return box, false, nil
		}
	}
	idList := d.IDGenClient2.GetNextIdList(ctx, idgen_client.MakeIDTypeNextId())
	if len(idList) != 1 || idList[0].Id <= 0 {
		return nil, false, mtproto.ErrInternalServerError
	}
	messageIDValue, err := counter.NextOn(ctx, tx, counter.MessageBoxKey(fromID), 1)
	if err != nil {
		return nil, false, err
	}
	dialogMessageID, messageID := idList[0].Id, int32(messageIDValue)
	dialogID := mtproto.MakeDialogId(fromID, peer.PeerType, peer.PeerId)
	message := proto.Clone(outboxMessage.Message).(*mtproto.Message)
	message.Out = true
	message.Id = messageID
	message.MediaUnread = mtproto.CheckHasMediaUnread(message)
	mData, err := jsonx.Marshal(message)
	if err != nil {
		return nil, false, err
	}
	savedPeer := &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY}
	if message.GetSavedPeerId() != nil {
		savedPeer = mtproto.FromPeer(message.GetSavedPeerId())
	}
	box := &mtproto.MessageBox{UserId: fromID, SenderUserId: fromID,
		PeerType: peer.PeerType, PeerId: peer.PeerId, MessageId: messageID,
		DialogId1: dialogID.A, DialogId2: dialogID.B, DialogMessageId: dialogMessageID,
		RandomId: outboxMessage.RandomId, MessageFilterType: mtproto.GetMediaType(message), Message: message,
		PtsCount: 1}
	dialogDO := &dataobject.DialogsDO{UserId: fromID, PeerType: peer.PeerType, PeerId: peer.PeerId,
		PeerDialogId: mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId), TopMessage: messageID,
		DraftMessageData: "null", Date2: int64(message.Date)}
	var duplicate *mtproto.MessageBox
	err = func() error {
		messageDO := &dataobject.MessagesDO{
			UserId: fromID, UserMessageBoxId: messageID, DialogId1: dialogID.A, DialogId2: dialogID.B,
			DialogMessageId: dialogMessageID, SenderUserId: fromID, PeerType: peer.PeerType, PeerId: peer.PeerId,
			RandomId: outboxMessage.RandomId, MessageFilterType: box.MessageFilterType, MessageData: string(mData),
			Message: message.Message, MediaUnread: message.MediaUnread, Date2: int64(message.Date),
			SavedPeerType: savedPeer.PeerType, SavedPeerId: savedPeer.PeerId,
		}
		storageID, rowsAffected, err := d.Postgres.Store.Messages.InsertOrReturnIdOn(ctx, tx, messageDO)
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			existing, err := d.Postgres.Store.Messages.SelectByStorageIDOn(ctx, tx, fromID, storageID)
			if err != nil {
				return err
			}
			if existing == nil {
				return errors.New("conflicting message has no stored record")
			}
			duplicate = makeMessageBoxByDO(existing)
			return nil
		}
		if _, err := d.Postgres.Store.Dialogs.UpdateMessageTopOn(ctx, tx, dialogDO.TopMessage, dialogDO.Date2, fromID, peer.PeerType, peer.PeerId); err != nil {
			return err
		}
		// A dialog may not exist yet for a first outbound message.
		if dlg, err := d.Postgres.Store.Dialogs.SelectDialogOn(ctx, tx, fromID, peer.PeerType, peer.PeerId); err != nil {
			return err
		} else if dlg == nil {
			if _, _, err = d.Postgres.Store.Dialogs.InsertIgnoreOn(ctx, tx, dialogDO); err != nil {
				return err
			}
		}
		for _, entity := range message.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag && entity.GetUrl() != "" {
				if _, _, err := d.Postgres.Store.HashTags.InsertOrUpdateOn(ctx, tx, &dataobject.HashTagsDO{
					UserId: fromID, PeerType: peer.PeerType, PeerId: peer.PeerId,
					HashTag: entity.GetUrl(), HashTagMessageId: messageID,
				}); err != nil {
					return err
				}
			}
		}
		pts, err := counter.NextOn(ctx, tx, counter.PtsKey(fromID), 1)
		if err != nil {
			return err
		}
		box.Pts = int32(pts)
		_, err = d.AddToPtsQueueOn(ctx, tx, fromID, box.Pts, 1, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: message, Pts_INT32: box.Pts, PtsCount: 1,
		}).To_Update())
		return err
	}()
	if err != nil {
		return nil, false, err
	}
	if duplicate != nil {
		if err := d.loadMessagePtsPostgres(ctx, tx, duplicate); err != nil {
			return nil, false, err
		}
		return duplicate, false, nil
	}
	return box, true, nil
}

func (d *Dao) SendUserMessage(ctx context.Context, fromId, toId int64, outBox *msg.OutboxMessage) (*mtproto.MessageBox, bool, error) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: toId}
	return d.sendMessageToOutbox(ctx, fromId, peer, outBox)
}

func (d *Dao) SendUserMessageV2(ctx context.Context, fromId, toId int64, outBox *msg.OutboxMessage, out bool) (*mtproto.MessageBox, error) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: toId}
	return d.sendMessageToOutboxV2(ctx, fromId, peer, outBox, out)
}

func (d *Dao) SendUserMessageV3(ctx context.Context, fromId, toId int64, outBox *msg.OutboxMessage, out bool) (*mtproto.MessageBox, error) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: toId}
	return d.sendMessageToOutboxV3(ctx, fromId, peer, outBox, out)
}

func (d *Dao) SendUserMultiMessage(ctx context.Context, fromId, toId int64, outBoxList []*msg.OutboxMessage) ([]*mtproto.MessageBox, error) {
	var (
		boxList []*mtproto.MessageBox
	)

	for _, msg := range outBoxList {
		peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: toId}
		outBox, _, err := d.sendMessageToOutbox(ctx, fromId, peer, msg)
		if err != nil {
			return nil, err
		}
		boxList = append(boxList, outBox)
	}

	return boxList, nil
}

func (d *Dao) SendChatMessage(ctx context.Context, fromId, chatId int64, outBox *msg.OutboxMessage) (*mtproto.MessageBox, bool, error) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: chatId}
	return d.sendMessageToOutbox(ctx, fromId, peer, outBox)
}

func (d *Dao) SendChatMessageV2(ctx context.Context, fromId, chatId int64, outBox *msg.OutboxMessage) (*mtproto.MessageBox, error) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: chatId}
	return d.sendMessageToOutboxV2(ctx, fromId, peer, outBox, true)
}

func (d *Dao) SendChatMultiMessage(ctx context.Context, fromId, chatId int64, outBoxList []*msg.OutboxMessage) ([]*mtproto.MessageBox, error) {
	var (
		boxList []*mtproto.MessageBox
	)

	for _, msg := range outBoxList {
		peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: chatId}
		box, _, err := d.sendMessageToOutbox(ctx, fromId, peer, msg)
		if err != nil {
			return nil, err
		}
		boxList = append(boxList, box)
	}

	return boxList, nil
}

func (d *Dao) DeleteMessages(ctx context.Context, userId int64, msgIds []int32) (*mtproto.PeerUtil, []int64, error) {

	var (
		topMessageIndex      int32
		dialogId             *mtproto.DialogID
		deletedMsgDataIdList = make([]int64, 0, len(msgIds))
	)

	msgDOList, err := d.SelectMessageByIDListWithCB(
		ctx,
		userId,
		msgIds,
		func(sz, i int, v *dataobject.MessagesDO) {
			if dialogId == nil {
				dialogId = &mtproto.DialogID{
					A: v.DialogId1,
					B: v.DialogId2,
				}
			}

			if dialogId.A != v.DialogId1 || dialogId.B != v.DialogId2 {
				dialogId.A = 0
				dialogId.B = 0
			}
		})
	if err != nil {
		// mtproto.ErrMsgIdInvalid
		return nil, nil, err
	} else if dialogId.IsZero() {
		return mtproto.MakePeerUtil(mtproto.PEER_EMPTY, 0), []int64{}, nil
	}

	// 会话里最后n条消息，检查是否需要修改会话信息
	topMessageDOList, err := d.SelectDialogLastMessages(ctx, userId, dialogId.A, dialogId.B, int32(len(msgIds)+1))
	if err != nil {
		return nil, nil, err
	} else if len(topMessageDOList) == 0 {
		// return []int64{}, nil
	} else {
		topMessageIndex = math.MaxInt32
	}

	getLastTopMessage := func(topMessage2 int32) (int32, int64) {
		for i := 0; i < len(topMessageDOList); i++ {
			if topMessageDOList[i].UserMessageBoxId >= topMessage2 {
				continue
			}
			return topMessageDOList[i].UserMessageBoxId, topMessageDOList[i].Date2
		}
		return 0, 0
	}

	//_, err = d.DialogsDAO.SelectDialogsWithCB().SelectByPeer(ctx, userId, model.GetPeerIdByDialogId(userId, dialogId))
	//if err != nil {
	//	return nil, err
	//}

	for i := 0; i < len(msgDOList); i++ {
		topMessage, _ := getLastTopMessage(topMessageIndex)
		if topMessage == msgDOList[i].UserMessageBoxId {
			topMessageIndex = topMessage
		}
	}

	peer := dialogId.ToPeerUtil(userId)
	if d.Postgres != nil && d.Postgres.Store != nil {
		topMessage, _ := getLastTopMessage(topMessageIndex)
		err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
			if _, err := d.Postgres.Store.Messages.DeleteMessagesByMessageIdListOn(ctx, tx, userId, msgIds); err != nil {
				return err
			}
			_, err := d.Postgres.Store.Dialogs.UpdateOutboxDialogOn(ctx, tx, topMessage, time.Now().Unix(), userId, peer.PeerType, peer.PeerId)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		for i := range msgDOList {
			deletedMsgDataIdList = append(deletedMsgDataIdList, msgDOList[i].DialogMessageId)
		}
		return peer, deletedMsgDataIdList, nil
	}

	tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		_, result.Err = d.MessagesDAO.DeleteMessagesByMessageIdListTx(tx, userId, msgIds)
		if result.Err != nil {
			return
		}
	})
	if tR.Err != nil {
		return nil, nil, tR.Err
	}

	topMessage, _ := getLastTopMessage(topMessageIndex)
	d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, err2 := d.DialogsDAO.UpdateOutboxDialog(
				ctx,
				topMessage,
				time.Now().Unix(),
				userId,
				peer.PeerType,
				peer.PeerId)

			return 0, 0, err2
		},
		dialog.GetDialogCacheKeyByPeer(userId, peer.PeerType, peer.PeerId))

	for i := 0; i < len(msgDOList); i++ {
		deletedMsgDataIdList = append(deletedMsgDataIdList, msgDOList[i].DialogMessageId)
	}
	return peer, deletedMsgDataIdList, nil
}

//func (d *Dao) editOutboxMessage(ctx context.Context, fromId int32, peer *model.PeerUtil, toId int32, message *mtproto.Message) (box *model.MessageBox, err error) {
//	mType, mData := model.EncodeMessage(message)
//	if _, err = m.MessagesDAO.UpdateEditMessage(ctx, int8(mType), hack.String(mData), message.Message, fromId, message.Id); err != nil {
//		return
//	}
//	box = &model.MessageBox{
//		SelfUserId:        fromId,
//		SendUserId:        fromId,
//		PeerType:          peer.PeerType,
//		PeerId:            peer.PeerId,
//		MessageId:         message.Id,
//		DialogId:          model.MakeDialogId(fromId, peer.PeerType, peer.PeerId),
//		DialogMessageId:   0,
//		RandomId:          0,
//		Pts:               int32(idgen.NextPtsId(ctx, fromId)),
//		PtsCount:          1,
//		MessageFilterType: 0,
//		MessageBoxType:    0,
//		MessageType:       int8(mType),
//		Message:           message,
//	}
//	return
//}

func (d *Dao) EditUserOutboxMessage(ctx context.Context, fromId, toId int64, message *mtproto.Message) (*mtproto.MessageBox, error) {
	return d.editOutboxMessage(ctx, fromId, mtproto.PEER_USER, toId, message)
}

func (d *Dao) EditChatOutboxMessage(ctx context.Context, fromId, toId int64, message *mtproto.Message) (*mtproto.MessageBox, error) {
	return d.editOutboxMessage(ctx, fromId, mtproto.PEER_CHAT, toId, message)
}

func (d *Dao) editOutboxMessage(ctx context.Context, fromId int64, peerType int32, peerId int64, message *mtproto.Message) (*mtproto.MessageBox, error) {
	var (
		mData, _ = jsonx.Marshal(message)
		did      = mtproto.MakeDialogId(fromId, peerType, peerId)
	)

	if _, err := d.UpdateMessageEdit(ctx, string(mData), message.Message, fromId, message.Id); err != nil {
		return nil, err
	}

	d.DeleteHashTagMessageID(ctx, fromId, message.Id)
	for _, entity := range message.GetEntities() {
		if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag {
			if entity.GetUrl() != "" {
				d.InsertOrUpdateHashTag(ctx, &dataobject.HashTagsDO{
					UserId:           fromId,
					PeerType:         peerType,
					PeerId:           peerId,
					HashTag:          entity.GetUrl(),
					HashTagMessageId: message.Id,
				})
			}
		}
	}

	return &mtproto.MessageBox{
		UserId:            fromId,
		MessageId:         message.Id,
		SenderUserId:      fromId,
		PeerType:          peerType,
		PeerId:            peerId,
		RandomId:          0,
		DialogId1:         did.A,
		DialogId2:         did.B,
		DialogMessageId:   0,
		MessageFilterType: 0,
		Message:           message,
		Pts:               d.IDGenClient2.NextPtsId(ctx, fromId),
		PtsCount:          1,
		Mentioned:         false,
		MediaUnread:       false,
	}, nil
}

func (d *Dao) DeletePhoneCallHistory(ctx context.Context, userId int64) ([]int32, []int64, error) {
	var (
		dialogIdMap          = make(map[mtproto.DialogID][]int32)
		deletedIdList        = make([]int32, 0)
		deletedMsgDataIdList = make([]int64, 0)
	)

	_, err := d.SelectPhoneCallMessages(
		ctx,
		userId,
		mtproto.MEDIA_PHONE_CALL,
		math.MaxInt32,
		math.MaxInt32,
		func(sz, i int, v *dataobject.MessagesDO) {
			did := mtproto.DialogID{A: v.DialogId1, B: v.DialogId2}
			if v2, ok := dialogIdMap[did]; !ok {
				dialogIdMap[did] = []int32{v.UserMessageBoxId}
			} else {
				dialogIdMap[did] = append(v2, v.UserMessageBoxId)
			}
			deletedIdList = append(deletedIdList, v.UserMessageBoxId)
			deletedMsgDataIdList = append(deletedMsgDataIdList, v.DialogMessageId)
		},
	)
	if err != nil {
		logx.WithContext(ctx).Errorf("error: %v", err)
		return nil, nil, err
	}

	if len(dialogIdMap) == 0 {
		return []int32{}, []int64{}, nil
	}

	logx.WithContext(ctx).Infof("phone: %v", dialogIdMap)

	for dialogId, msgIds := range dialogIdMap {
		_, err = d.DeleteMessageByIDList(ctx, userId, msgIds)
		if err != nil {
			return nil, nil, err
		}

		// TODO: performance optimization
		lastMessageId, err := d.SelectDialogLastMessageID(ctx, userId, dialogId.A, dialogId.B)
		if err != nil {
			return nil, nil, err
		}
		if d.Postgres != nil && d.Postgres.Store != nil {
			if _, err = d.Postgres.Store.Dialogs.UpdateCustomMap(ctx, map[string]any{"top_message": lastMessageId}, userId, mtproto.PEER_USER, mtproto.GetPeerIdByDialogId(userId, dialogId)); err != nil {
				return nil, nil, err
			}
			continue
		}
		_, _, err = d.CachedConn.Exec(
			ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				_, err2 := d.UpdateDialogCustomMap(
					ctx,
					map[string]interface{}{
						"top_message": lastMessageId,
					},
					userId,
					mtproto.PEER_USER, mtproto.GetPeerIdByDialogId(userId, dialogId))
				return 0, 0, err2
			},
			dialog.GetDialogCacheKeyByPeer(userId, mtproto.PEER_USER, mtproto.GetPeerIdByDialogId(userId, dialogId)))
		if err != nil {
			return nil, nil, err
		}
	}

	return deletedIdList, deletedMsgDataIdList, nil
}

func (d *Dao) SendMessageToOutboxV1(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, outMsgBox *mtproto.MessageBox) (bool, error) {
	return d.sendMessageToOutboxV1(ctx, fromId, peer, outMsgBox, true)
}

// SendMessageToOutboxV1NoPts writes the outbox message to DB without writing
// user_pts_updates (caller is responsible for pts persistence).
func (d *Dao) SendMessageToOutboxV1NoPts(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, outMsgBox *mtproto.MessageBox) (bool, error) {
	return d.sendMessageToOutboxV1(ctx, fromId, peer, outMsgBox, false)
}

func (d *Dao) sendMessageToOutboxV1(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, outMsgBox *mtproto.MessageBox, writePts bool) (bool, error) {
	if d.Postgres != nil && d.Postgres.Store != nil {
		return d.sendMessageToOutboxV1Postgres(ctx, fromId, peer, outMsgBox, writePts)
	}
	message := outMsgBox.Message
	mData, _ := jsonx.Marshal(outMsgBox.GetMessage())
	outBoxMsgId := outMsgBox.MessageId

	var (
		savedPeerUtil *mtproto.PeerUtil
	)

	if message.GetSavedPeerId() != nil {
		savedPeerUtil = mtproto.FromPeer(message.GetSavedPeerId())
	} else {
		savedPeerUtil = &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY, PeerId: 0}
	}

	inserted := false
	tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		_, rowsAffected, err := d.MessagesDAO.InsertOrReturnIdTx(
			tx,
			&dataobject.MessagesDO{
				UserId:            outMsgBox.UserId,
				UserMessageBoxId:  outMsgBox.MessageId,
				DialogId1:         outMsgBox.DialogId1,
				DialogId2:         outMsgBox.DialogId2,
				DialogMessageId:   outMsgBox.DialogMessageId,
				SenderUserId:      outMsgBox.UserId,
				PeerType:          peer.PeerType,
				PeerId:            peer.PeerId,
				RandomId:          outMsgBox.RandomId,
				MessageFilterType: outMsgBox.MessageFilterType,
				MessageData:       hack.String(mData),
				Message:           message.GetMessage(),
				Mentioned:         false,
				MediaUnread:       message.GetMediaUnread(),
				Date2:             int64(outMsgBox.Message.Date),
				SavedPeerType:     savedPeerUtil.PeerType,
				SavedPeerId:       savedPeerUtil.PeerId,
				Deleted:           false,
			})
		if err != nil {
			result.Err = err
			return
		}
		if rowsAffected == 0 {
			return
		}
		inserted = true

		for _, entity := range message.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag {
				if entity.GetUrl() != "" {
					_, _, err = d.HashTagsDAO.InsertOrUpdateTx(tx, &dataobject.HashTagsDO{
						UserId:           outMsgBox.UserId,
						PeerType:         peer.PeerType,
						PeerId:           peer.PeerId,
						HashTag:          entity.GetUrl(),
						HashTagMessageId: outMsgBox.MessageId,
					})
				}
			}
		}

		if writePts {
			_, err = d.AddToPtsQueueTx(tx, fromId, outMsgBox.Pts, outMsgBox.PtsCount, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
				Message_MESSAGE: outMsgBox.Message,
				Pts_INT32:       outMsgBox.Pts,
				PtsCount:        outMsgBox.PtsCount,
			}).To_Update())
			if err != nil {
				result.Err = err
				return
			}
		}
	})

	if tR.Err != nil {
		return false, tR.Err
	}
	if !inserted {
		return false, nil
	}
	_, err := d.DialogClient.DialogInsertOrUpdateDialog(
		ctx,
		&dialog.TLDialogInsertOrUpdateDialog{
			UserId:          fromId,
			PeerType:        peer.PeerType,
			PeerId:          peer.PeerId,
			TopMessage:      &wrapperspb.Int32Value{Value: outBoxMsgId},
			ReadOutboxMaxId: nil,
			ReadInboxMaxId:  nil,
			UnreadCount:     &wrapperspb.Int32Value{Value: 0},
			UnreadMark:      false,
			PinnedMsgId:     nil,
			Date2:           &wrapperspb.Int64Value{Value: int64(outMsgBox.Message.Date)},
		})
	if err != nil {
		// return i
	}

	return true, tR.Err
}

func (d *Dao) sendMessageToOutboxV1Postgres(ctx context.Context, fromID int64, peer *mtproto.PeerUtil, outMsgBox *mtproto.MessageBox, writePts bool) (bool, error) {
	if outMsgBox == nil || outMsgBox.Message == nil || peer == nil {
		return false, mtproto.ErrInputRequestInvalid
	}
	mData, err := jsonx.Marshal(outMsgBox.GetMessage())
	if err != nil {
		return false, err
	}
	savedPeer := &mtproto.PeerUtil{PeerType: mtproto.PEER_EMPTY}
	if outMsgBox.Message.GetSavedPeerId() != nil {
		savedPeer = mtproto.FromPeer(outMsgBox.Message.GetSavedPeerId())
	}
	inserted := false
	err = d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('messenger-user:' || $1::bigint::text, 0))`, fromID); err != nil {
			return err
		}
		storageID, rowsAffected, err := d.Postgres.Store.Messages.InsertOrReturnIdOn(ctx, tx, &dataobject.MessagesDO{
			UserId: outMsgBox.UserId, UserMessageBoxId: outMsgBox.MessageId, DialogId1: outMsgBox.DialogId1,
			DialogId2: outMsgBox.DialogId2, DialogMessageId: outMsgBox.DialogMessageId, SenderUserId: outMsgBox.UserId,
			PeerType: peer.PeerType, PeerId: peer.PeerId, RandomId: outMsgBox.RandomId,
			MessageFilterType: outMsgBox.MessageFilterType, MessageData: string(mData), Message: outMsgBox.Message.GetMessage(),
			MediaUnread: outMsgBox.Message.GetMediaUnread(), Date2: int64(outMsgBox.Message.Date),
			SavedPeerType: savedPeer.PeerType, SavedPeerId: savedPeer.PeerId,
		})
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			existing, err := d.Postgres.Store.Messages.SelectByStorageIDOn(ctx, tx, outMsgBox.UserId, storageID)
			if err != nil {
				return err
			}
			if existing == nil {
				return errors.New("conflicting outbox message has no stored record")
			}
			pts, ptsCount := outMsgBox.Pts, outMsgBox.PtsCount
			*outMsgBox = *makeMessageBoxByDO(existing)
			outMsgBox.Pts, outMsgBox.PtsCount = pts, ptsCount
			return nil
		}
		inserted = true
		for _, entity := range outMsgBox.Message.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag && entity.GetUrl() != "" {
				if _, _, err := d.Postgres.Store.HashTags.InsertOrUpdateOn(ctx, tx, &dataobject.HashTagsDO{UserId: outMsgBox.UserId, PeerType: peer.PeerType, PeerId: peer.PeerId, HashTag: entity.GetUrl(), HashTagMessageId: outMsgBox.MessageId}); err != nil {
					return err
				}
			}
		}
		if writePts {
			if outMsgBox.Pts <= 0 {
				pts, err := counter.NextOn(ctx, tx, counter.PtsKey(fromID), 1)
				if err != nil {
					return err
				}
				outMsgBox.Pts = int32(pts)
				outMsgBox.PtsCount = 1
			}
			if outMsgBox.Pts <= 0 || outMsgBox.PtsCount <= 0 {
				return mtproto.ErrInternalServerError
			}
			if _, err := d.AddToPtsQueueOn(ctx, tx, fromID, outMsgBox.Pts, outMsgBox.PtsCount, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: outMsgBox.Message, Pts_INT32: outMsgBox.Pts, PtsCount: outMsgBox.PtsCount}).To_Update()); err != nil {
				return err
			}
		}
		if _, err := d.Postgres.Store.Dialogs.UpdateMessageTopOn(ctx, tx, outMsgBox.MessageId, int64(outMsgBox.Message.Date), fromID, peer.PeerType, peer.PeerId); err != nil {
			return err
		}
		_, _, err = d.Postgres.Store.Dialogs.InsertIgnoreOn(ctx, tx, &dataobject.DialogsDO{UserId: fromID, PeerType: peer.PeerType, PeerId: peer.PeerId, PeerDialogId: mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId), TopMessage: outMsgBox.MessageId, DraftMessageData: "null", Date2: int64(outMsgBox.Message.Date)})
		return err
	})
	if err != nil {
		return false, err
	}
	return inserted, nil
}

func (d *Dao) sendMessageToOutboxV2(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, outboxMessage *msg.OutboxMessage, out bool) (*mtproto.MessageBox, error) {
	var (
		dialogId        = mtproto.MakeDialogId(fromId, peer.PeerType, peer.PeerId)
		err             error
		message         = outboxMessage.Message
		outBoxMsgId     int32
		dialogMessageId int64
		pts             int32
	)

	if out {
		idList := d.IDGenClient2.GetNextIdList(
			ctx,
			idgen_client.MakeIDTypeNextId(),
			idgen_client.MakeIDTypeNgen(idgen_client.IDTypeMessageBox, fromId),
			idgen_client.MakeIDTypeNgen(idgen_client.IDTypePts, fromId))
		if len(idList) != 3 {
			err = mtproto.ErrInternalServerError
			return nil, err
		}

		dialogMessageId = idList[0].Id
		outBoxMsgId = int32(idList[1].Id)
		pts = int32(idList[2].Id)

		if dialogMessageId == 0 || outBoxMsgId == 0 || pts == 0 {
			logx.WithContext(ctx).Errorf("GetNextIdList error: %v", idList)
			err = mtproto.ErrInternalServerError
			return nil, err
		}
	} else {
		dialogMessageId = d.IDGenClient2.NextId(ctx)
		if dialogMessageId == 0 {
			err = mtproto.ErrInternalServerError
			logx.WithContext(ctx).Errorf("NextId error: %v", dialogMessageId)
			return nil, err

		}
	}

	message.Out = out
	message.Id = outBoxMsgId
	message.MediaUnread = mtproto.CheckHasMediaUnread(message)
	outMsgBox := mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:            fromId,
		MessageId:         outBoxMsgId,
		SenderUserId:      fromId,
		PeerType:          peer.PeerType,
		PeerId:            peer.PeerId,
		RandomId:          outboxMessage.RandomId,
		DialogId1:         dialogId.A,
		DialogId2:         dialogId.B,
		DialogMessageId:   dialogMessageId,
		MessageFilterType: mtproto.GetMediaType(message),
		Message:           message,
		Mentioned:         false,
		MediaUnread:       false,
		Pinned:            false,
		Pts:               pts,
		PtsCount:          1,
		Views:             0,
		ReplyOwnerId:      0,
		Forwards:          0,
		Reaction:          "",
		CommentGroupId:    0,
		CommentGroupMsgId: 0,
		ReplyToMsgId:      0,
		ReplyToTopId:      0,
		TtlPeriod:         0,
		HasReaction:       false,
	}).To_MessageBox()

	// Write sender's user_pts_updates before RPC return to guarantee
	// getDifference completeness for the sender.
	if out && pts > 0 {
		if _, err = d.AddToPtsQueueE(ctx, fromId, pts, 1, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: message,
			Pts_INT32:       pts,
			PtsCount:        1,
		}).To_Update()); err != nil {
			logx.WithContext(ctx).Errorf("sendMessageToOutboxV2 - AddToPtsQueueE error, user_id: %d, pts: %d, err: %v", fromId, pts, err)
		}
	}

	return outMsgBox, nil
}

func (d *Dao) sendMessageToOutboxV3(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, outboxMessage *msg.OutboxMessage, out bool) (*mtproto.MessageBox, error) {
	var (
		dialogId        = mtproto.MakeDialogId(fromId, peer.PeerType, peer.PeerId)
		err             error
		message         = outboxMessage.Message
		outBoxMsgId     int32
		dialogMessageId int64
		pts             int32
	)

	if out {
		idList := d.IDGenClient2.GetNextIdList(
			ctx,
			idgen_client.MakeIDTypeNextId(),
			idgen_client.MakeIDTypeNgen(idgen_client.IDTypeMessageBox, fromId))
		// idgen_client.MakeIDTypeNgen(idgen_client.IDTypePts, fromId))
		if len(idList) != 2 {
			err = mtproto.ErrInternalServerError
			return nil, err
		}

		dialogMessageId = idList[0].Id
		outBoxMsgId = int32(idList[1].Id)
		// pts = int32(idList[2].Id)

		// if dialogMessageId == 0 || outBoxMsgId == 0 || pts == 0 {
		if dialogMessageId == 0 || outBoxMsgId == 0 {
			logx.WithContext(ctx).Errorf("GetNextIdList error: %v", idList)
			err = mtproto.ErrInternalServerError
			return nil, err
		}
	} else {
		dialogMessageId = d.IDGenClient2.NextId(ctx)
		if dialogMessageId == 0 {
			err = mtproto.ErrInternalServerError
			logx.WithContext(ctx).Errorf("NextId error: %v", dialogMessageId)
			return nil, err

		}
	}

	message.Out = out
	message.Id = outBoxMsgId
	message.MediaUnread = mtproto.CheckHasMediaUnread(message)
	outMsgBox := mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:            fromId,
		MessageId:         outBoxMsgId,
		SenderUserId:      fromId,
		PeerType:          peer.PeerType,
		PeerId:            peer.PeerId,
		RandomId:          outboxMessage.RandomId,
		DialogId1:         dialogId.A,
		DialogId2:         dialogId.B,
		DialogMessageId:   dialogMessageId,
		MessageFilterType: mtproto.GetMediaType(message),
		Message:           message,
		Mentioned:         false,
		MediaUnread:       false,
		Pinned:            false,
		Pts:               pts,
		PtsCount:          1,
		Views:             0,
		ReplyOwnerId:      0,
		Forwards:          0,
		Reaction:          "",
		CommentGroupId:    0,
		CommentGroupMsgId: 0,
		ReplyToMsgId:      0,
		ReplyToTopId:      0,
		TtlPeriod:         0,
		HasReaction:       false,
	}).To_MessageBox()

	return outMsgBox, nil
}

func (d *Dao) EditUserOutboxMessageV2(ctx context.Context, fromId, toId int64, newMessage *msg.OutboxMessage, dstMessage *mtproto.MessageBox) (*mtproto.MessageBox, error) {
	return d.editOutboxMessageV2(ctx, fromId, mtproto.PEER_USER, toId, newMessage, dstMessage)
}

func (d *Dao) EditChatOutboxMessageV2(ctx context.Context, fromId, toId int64, newMessage *msg.OutboxMessage, dstMessage *mtproto.MessageBox) (*mtproto.MessageBox, error) {
	return d.editOutboxMessageV2(ctx, fromId, mtproto.PEER_CHAT, toId, newMessage, dstMessage)
}

func (d *Dao) editOutboxMessageV2(ctx context.Context, fromId int64, peerType int32, peerId int64, newMessage *msg.OutboxMessage, dstMessage *mtproto.MessageBox) (*mtproto.MessageBox, error) {
	if d.Postgres != nil {
		return d.EditMessageState(ctx, fromId, mtproto.MakePeerUtil(peerType, peerId), dstMessage.MessageId, newMessage.Message)
	}
	var (
		pts            = d.IDGenClient2.NextPtsId(ctx, fromId)
		ptsCount int32 = 1
		message        = newMessage.Message
	)

	if pts == 0 {
		logx.WithContext(ctx).Errorf("NextPtsId error: %v", fromId)
		err := mtproto.ErrInternalServerError
		return nil, err
	}

	// Write sender's user_pts_updates before RPC return to guarantee
	// getDifference completeness for the sender.
	if _, err := d.AddToPtsQueueE(ctx, fromId, pts, ptsCount, mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
		Message_MESSAGE: message,
		Pts_INT32:       pts,
		PtsCount:        ptsCount,
	}).To_Update()); err != nil {
		logx.WithContext(ctx).Errorf("editOutboxMessageV2 - AddToPtsQueueE error, user_id: %d, pts: %d, err: %v", fromId, pts, err)
	}

	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:            dstMessage.UserId,
		MessageId:         dstMessage.MessageId,
		SenderUserId:      dstMessage.SenderUserId,
		PeerType:          dstMessage.PeerType,
		PeerId:            dstMessage.PeerId,
		RandomId:          dstMessage.RandomId,
		DialogId1:         dstMessage.DialogId1,
		DialogId2:         dstMessage.DialogId2,
		DialogMessageId:   dstMessage.DialogMessageId,
		MessageFilterType: dstMessage.MessageFilterType,
		Message:           message,
		Mentioned:         dstMessage.Mentioned,
		MediaUnread:       dstMessage.MediaUnread,
		Pinned:            dstMessage.Pinned,
		Pts:               pts,
		PtsCount:          ptsCount,
		Views:             dstMessage.Views,
		ReplyOwnerId:      dstMessage.ReplyOwnerId,
		Forwards:          dstMessage.Forwards,
		Reaction:          dstMessage.Reaction,
		CommentGroupId:    dstMessage.CommentGroupId,
		CommentGroupMsgId: dstMessage.CommentGroupMsgId,
		ReplyToMsgId:      dstMessage.ReplyToMsgId,
		ReplyToTopId:      dstMessage.ReplyToTopId,
		TtlPeriod:         dstMessage.TtlPeriod,
		HasReaction:       dstMessage.HasReaction,
	}).To_MessageBox(), nil
}
