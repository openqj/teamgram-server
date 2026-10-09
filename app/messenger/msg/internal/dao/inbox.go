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
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/marmota/pkg/container2"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/proto"
)

// from outBox --> make inBox
func (d *Dao) makeMessageInBox(fromId int64, peer *mtproto.PeerUtil, toUserId int64, clientRandomId int64, dialogMessageId int64, message *mtproto.Message) *mtproto.MessageBox {
	mentioned := mtproto.CheckHasMention(message.Entities, toUserId)
	logx.Infof("insert to inbox: %#v, message: {%#v}", mentioned, message)

	did := mtproto.MakeDialogId(fromId, peer.PeerType, peer.PeerId)
	// from outBox --> make inBox
	return &mtproto.MessageBox{
		UserId:            fromId,
		MessageId:         0,
		DialogId1:         did.A,
		DialogId2:         did.B,
		DialogMessageId:   dialogMessageId,
		RandomId:          clientRandomId,
		MessageFilterType: mtproto.GetMediaType(message),
	}
}

func replyPeerMatchesCurrentDialog(peer *mtproto.PeerUtil, replyPeer *mtproto.Peer) bool {
	if peer == nil || replyPeer == nil {
		return false
	}

	switch peer.PeerType {
	case mtproto.PEER_USER:
		return replyPeer.PredicateName == mtproto.Predicate_peerUser && replyPeer.UserId == peer.PeerId
	case mtproto.PEER_CHAT:
		return replyPeer.PredicateName == mtproto.Predicate_peerChat && replyPeer.ChatId == peer.PeerId
	case mtproto.PEER_CHANNEL:
		return replyPeer.PredicateName == mtproto.Predicate_peerChannel && replyPeer.ChannelId == peer.PeerId
	default:
		return false
	}
}

func (d *Dao) sendMessageToInbox(ctx context.Context, fromId int64, peer *mtproto.PeerUtil, toUserId int64, dialogMessageId, clientRandomId int64, message2 *mtproto.Message) (*mtproto.MessageBox, error) {
	if d.Postgres != nil {
		var existing *dataobject.MessagesDO
		var err error
		if clientRandomId != 0 {
			existing, err = d.Postgres.Store.Messages.SelectByRandomIdOn(ctx, d.Postgres.Pool, toUserId, fromId, clientRandomId)
		} else if dialogMessageId != 0 {
			existing, err = d.Postgres.Store.Messages.SelectByDeliveryIDOn(ctx, d.Postgres.Pool, toUserId, fromId, dialogMessageId)
		}
		if err != nil {
			return nil, err
		}
		if existing != nil {
			box := makeMessageBoxByDO(existing)
			if err := d.RestoreMessagePts(ctx, box); err != nil {
				return nil, err
			}
			box.PtsCount = 0
			return box, nil
		}
	}
	var (
		inBoxMsgId int32
		dialogId   = mtproto.MakeDialogId(fromId, peer.PeerType, peer.PeerId)
		date       = time.Now().Unix()
		message    = proto.Clone(message2).(*mtproto.Message)

		dialogDO *dataobject.DialogsDO
	)
	if d.Postgres == nil {
		inBoxMsgId = d.IDGenClient2.NextMessageBoxId(ctx, toUserId)
	}

	if peer.PeerType == mtproto.PEER_USER {
		if dialogMessageId == 0 {
			dialogMessageId = d.IDGenClient2.NextId(ctx)
		}
	}

	// fix message
	message.Out = false
	message.Id = inBoxMsgId
	switch message.GetReplyTo().GetPredicateName() {
	case mtproto.Predicate_messageReplyHeader:
		replyToPeer := message.GetReplyTo().GetReplyToPeerId()
		if replyToPeer != nil && !replyPeerMatchesCurrentDialog(peer, replyToPeer) {
			break
		}
		if replyId, _ := d.SelectPeerUserMessage(ctx, toUserId, fromId, message.GetReplyTo().GetFixedReplyToMsgId()); replyId != nil {
			// message.ReplyToMsgId.Value = replyId.UserMessageBoxId
			if message.ReplyTo != nil {
				message.ReplyTo.ReplyToMsgId = replyId.UserMessageBoxId
				message.ReplyTo.ReplyToMsgId_INT32 = replyId.UserMessageBoxId
				message.ReplyTo.ReplyToMsgId_FLAGINT32 = mtproto.MakeFlagsInt32(replyId.UserMessageBoxId)
			}

			if peer.PeerType == mtproto.PEER_CHAT && replyId.SenderUserId == toUserId {
				message.Mentioned = true
				if message2.GetAction().GetPredicateName() != mtproto.Predicate_messageActionPinMessage {
					message.MediaUnread = true
				}
			}
		} else {
			// message.ReplyToMsgId.Value = 0
			message.ReplyTo = nil
		}
	case mtproto.Predicate_messageReplyStoryHeader:
		// do nothing
	default:
		// do nothing
	}

	if peer.PeerType == mtproto.PEER_CHAT {
		if !message.Mentioned {
			message.Mentioned = mtproto.CheckHasMention(message.Entities, toUserId)
			if message.Mentioned {
				message.MediaUnread = true
			}
		}
	} else if peer.PeerType == mtproto.PEER_USER {
		message.FromId = nil
		message.PeerId = mtproto.MakePeerUser(fromId)
	}

	if !message.MediaUnread {
		message.MediaUnread = mtproto.CheckHasMediaUnread(message)
	}

	if peer.PeerType == mtproto.PEER_CHAT {
		if message2.GetAction().GetPredicateName() == mtproto.Predicate_messageActionGroupCall {
			call := message2.GetAction()
			if call != nil && len(call.Users) > 0 {
				if ok := container2.ContainsInt64(call.Users, toUserId); ok {
					message.MediaUnread = true
					message.Mentioned = true
				}
			}
		}
	}

	mData, err := jsonx.Marshal(message)
	if err != nil {
		return nil, err
	}
	// mType, mData := mtproto.EncodeMessage(message)
	inBox := &mtproto.MessageBox{
		UserId:            toUserId,
		SenderUserId:      fromId,
		PeerType:          peer.PeerType,
		PeerId:            peer.PeerId,
		MessageId:         inBoxMsgId,
		DialogId1:         dialogId.A,
		DialogId2:         dialogId.B,
		DialogMessageId:   dialogMessageId,
		RandomId:          clientRandomId,
		Pts:               0,
		PtsCount:          0,
		MessageFilterType: mtproto.GetMediaType(message),
		Message:           message,
		Mentioned:         message.Mentioned,
		MediaUnread:       message.MediaUnread,
	}
	if d.Postgres != nil && d.Postgres.Store != nil {
		return d.persistInboxPostgres(ctx, fromId, peer, toUserId, inBox, message, mData, date)
	}

	tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		// TODO(@benqi): do ignore

		// Pts:              pts,
		// PtsCount:         ptsCount,
		inBoxDO := &dataobject.MessagesDO{
			UserId:            inBox.UserId,
			UserMessageBoxId:  inBox.MessageId,
			DialogId1:         inBox.DialogId1,
			DialogId2:         inBox.DialogId2,
			SenderUserId:      fromId,
			PeerType:          peer.PeerType,
			PeerId:            inBox.PeerId,
			RandomId:          inBox.RandomId,
			DialogMessageId:   inBox.DialogMessageId,
			MessageData:       string(mData),
			MessageFilterType: inBox.MessageFilterType, // TODO(@benqi): message_type
			Message:           message.Message,
			Mentioned:         inBox.Mentioned,
			MediaUnread:       inBox.MediaUnread,
			Date2:             date,
			Deleted:           false,
		}

		_, _, result.Err = d.MessagesDAO.InsertOrReturnIdTx(tx, inBoxDO)
		if result.Err != nil {
			return
		}

		switch peer.PeerType {
		case mtproto.PEER_USER:
			//var (
			//	lastInsertId int64
			//	rowsAffected int64
			//)

			dialogDO = &dataobject.DialogsDO{
				UserId:           inBox.UserId,
				PeerType:         peer.PeerType,
				PeerId:           fromId,
				PeerDialogId:     mtproto.MakePeerDialogId(mtproto.PEER_USER, fromId),
				TopMessage:       inBoxMsgId,
				UnreadCount:      1,
				DraftMessageData: "null",
				Date2:            date,
			}

		case mtproto.PEER_CHAT:
			dialogDO = &dataobject.DialogsDO{
				UserId:               inBox.UserId,
				PeerType:             peer.PeerType,
				PeerId:               peer.PeerId,
				PeerDialogId:         mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId),
				Pinned:               0,
				TopMessage:           inBoxMsgId,
				PinnedMsgId:          0,
				ReadInboxMaxId:       0,
				ReadOutboxMaxId:      0,
				UnreadCount:          1,
				UnreadMentionsCount:  0,
				UnreadReactionsCount: 0,
				UnreadMark:           false,
				DraftType:            0,
				DraftMessageData:     "null",
				FolderId:             0,
				FolderPinned:         0,
				HasScheduled:         false,
				TtlPeriod:            0,
				ThemeEmoticon:        "",
				Date2:                date,
			}
			if inBox.Mentioned {
				dialogDO.UnreadMentionsCount = 1
			}

		default:
			result.Err = fmt.Errorf("fatal error - invalid peer_type: %v", peer)
		}

		for _, entity := range message.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag {
				if entity.GetUrl() != "" {
					_, _, _ = d.HashTagsDAO.InsertOrUpdateTx(tx, &dataobject.HashTagsDO{
						UserId:           inBox.UserId,
						PeerType:         peer.PeerType,
						PeerId:           peer.PeerId,
						HashTag:          entity.GetUrl(),
						HashTagMessageId: inBox.MessageId,
					})
				}
			}
		}
	})

	// TODO(@benqi): process duplicate

	if tR.Err != nil {
		return nil, tR.Err
	}

	_, _, _ = d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			lastInsertId, rowsAffected, err := d.InsertOrUpdateDialog(ctx, dialogDO)
			logx.WithContext(ctx).Infof("lastInsertId:%d, rowsAffected: %d, result: %v, do: %v", lastInsertId, rowsAffected, err, dialogDO)
			return 0, 0, err
		},
		dialog.GetDialogCacheKey(dialogDO.UserId, dialogDO.PeerDialogId))

	inBox.Pts = d.IDGenClient2.NextPtsId(ctx, toUserId)
	inBox.PtsCount = 1

	return inBox, nil
}

func (d *Dao) persistInboxPostgres(ctx context.Context, fromID int64, peer *mtproto.PeerUtil, toUserID int64, inBox *mtproto.MessageBox, message *mtproto.Message, messageData []byte, date int64) (*mtproto.MessageBox, error) {
	var dialogDO *dataobject.DialogsDO
	var duplicate *mtproto.MessageBox
	switch peer.PeerType {
	case mtproto.PEER_USER:
		dialogDO = &dataobject.DialogsDO{UserId: toUserID, PeerType: peer.PeerType, PeerId: fromID,
			PeerDialogId: mtproto.MakePeerDialogId(mtproto.PEER_USER, fromID), TopMessage: inBox.MessageId,
			UnreadCount: 1, DraftMessageData: "null", Date2: date}
	case mtproto.PEER_CHAT:
		dialogDO = &dataobject.DialogsDO{UserId: toUserID, PeerType: peer.PeerType, PeerId: peer.PeerId,
			PeerDialogId: mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId), TopMessage: inBox.MessageId,
			UnreadCount: 1, UnreadMentionsCount: 0, DraftMessageData: "null", Date2: date}
		if inBox.Mentioned {
			dialogDO.UnreadMentionsCount = 1
		}
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('messenger-user:' || $1::bigint::text, 0))`, toUserID); err != nil {
			return err
		}
		if inBox.MessageId == 0 {
			messageID, err := counter.NextOn(ctx, tx, counter.MessageBoxKey(toUserID), 1)
			if err != nil {
				return err
			}
			inBox.MessageId = int32(messageID)
			message.Id = inBox.MessageId
			dialogDO.TopMessage = inBox.MessageId
			messageData, err = jsonx.Marshal(message)
			if err != nil {
				return err
			}
		}
		storageID, rowsAffected, err := d.Postgres.Store.Messages.InsertOrReturnIdOn(ctx, tx, &dataobject.MessagesDO{
			UserId: inBox.UserId, UserMessageBoxId: inBox.MessageId, DialogId1: inBox.DialogId1, DialogId2: inBox.DialogId2,
			SenderUserId: fromID, PeerType: peer.PeerType, PeerId: inBox.PeerId, RandomId: inBox.RandomId,
			DialogMessageId: inBox.DialogMessageId, MessageData: string(messageData), MessageFilterType: inBox.MessageFilterType,
			Message: message.Message, Mentioned: inBox.Mentioned, MediaUnread: inBox.MediaUnread, Date2: date,
		})
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			existing, err := d.Postgres.Store.Messages.SelectByStorageIDOn(ctx, tx, toUserID, storageID)
			if err != nil {
				return err
			}
			if existing == nil {
				return fmt.Errorf("conflicting inbox message has no stored record")
			}
			duplicate = makeMessageBoxByDO(existing)
			return nil
		}
		if _, _, err = d.Postgres.Store.Dialogs.InsertOrUpdateOn(ctx, tx, dialogDO); err != nil {
			return err
		}
		for _, entity := range message.GetEntities() {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag && entity.GetUrl() != "" {
				if _, _, err = d.Postgres.Store.HashTags.InsertOrUpdateOn(ctx, tx, &dataobject.HashTagsDO{
					UserId: inBox.UserId, PeerType: peer.PeerType, PeerId: peer.PeerId,
					HashTag: entity.GetUrl(), HashTagMessageId: inBox.MessageId,
				}); err != nil {
					return err
				}
			}
		}
		pts, err := counter.NextOn(ctx, tx, counter.PtsKey(toUserID), 1)
		if err != nil {
			return err
		}
		inBox.Pts, inBox.PtsCount = int32(pts), 1
		_, err = d.AddToPtsQueueOn(ctx, tx, toUserID, inBox.Pts, inBox.PtsCount, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: message, Pts_INT32: inBox.Pts, PtsCount: inBox.PtsCount,
		}).To_Update())
		if err != nil {
			return err
		}
		if peer.PeerType == mtproto.PEER_CHAT && message.GetAction().GetPredicateName() == mtproto.Predicate_messageActionChatMigrateTo {
			if _, err := d.Postgres.Store.Dialogs.UpdateCustomMapOn(ctx, tx, map[string]any{"read_inbox_max_id": inBox.MessageId, "unread_count": 0}, toUserID, peer.PeerType, peer.PeerId); err != nil {
				return err
			}
			pts, err := counter.NextOn(ctx, tx, counter.PtsKey(toUserID), 1)
			if err != nil {
				return err
			}
			_, err = d.AddToPtsQueueOn(ctx, tx, toUserID, int32(pts), 1, mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
				Peer_PEER: mtproto.MakePeerChat(peer.PeerId), MaxId: inBox.MessageId,
				StillUnreadCount: 0, Pts_INT32: int32(pts), PtsCount: 1,
			}).To_Update())
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if duplicate != nil {
		if err := d.RestoreMessagePts(ctx, duplicate); err != nil {
			return nil, err
		}
		duplicate.PtsCount = 0
		return duplicate, nil
	}
	return inBox, nil
}

func (d *Dao) SendUserMessageToInbox(ctx context.Context, fromId, toId int64, dialogMessageId, clientRandomId int64, message *mtproto.Message) (*mtproto.MessageBox, error) {
	peer := &mtproto.PeerUtil{
		PeerType: mtproto.PEER_USER,
		PeerId:   toId,
	}
	return d.sendMessageToInbox(ctx, fromId, peer, toId, dialogMessageId, clientRandomId, message)
}

func (d *Dao) SendChatMessageToInbox(ctx context.Context, fromId, chatId, toId int64, dialogMessageId, clientRandomId int64, message *mtproto.Message) (*mtproto.MessageBox, error) {
	peer := &mtproto.PeerUtil{
		PeerType: mtproto.PEER_CHAT,
		PeerId:   chatId,
	}
	return d.sendMessageToInbox(ctx, fromId, peer, toId, dialogMessageId, clientRandomId, message)
}

func (d *Dao) SendUserMultiMessageToInbox(ctx context.Context, fromId, toId int64, inBoxList []*inbox.InboxMessageData) ([]*mtproto.MessageBox, error) {
	var (
		boxList = make([]*mtproto.MessageBox, 0, len(inBoxList))
	)

	for _, box := range inBoxList {
		peer := &mtproto.PeerUtil{
			PeerType: mtproto.PEER_USER,
			PeerId:   toId,
		}
		inBox, err := d.sendMessageToInbox(ctx, fromId, peer, toId, box.DialogMessageId, box.RandomId, box.Message)
		if err != nil {
			return nil, err
		}
		boxList = append(boxList, inBox)
	}

	return boxList, nil
}

func (d *Dao) SendChatMultiMessageToInbox(ctx context.Context, fromId, chatId, toId int64, inBoxList []*inbox.InboxMessageData) ([]*mtproto.MessageBox, error) {
	var (
		boxList = make([]*mtproto.MessageBox, 0, len(inBoxList))
	)
	for _, box := range inBoxList {
		peer := &mtproto.PeerUtil{
			PeerType: mtproto.PEER_CHAT,
			PeerId:   chatId,
		}
		inBox, err := d.sendMessageToInbox(ctx, fromId, peer, toId, box.DialogMessageId, box.RandomId, box.Message)
		if err != nil {
			return nil, err
		}
		boxList = append(boxList, inBox)
	}

	return boxList, nil
}

func (d *Dao) DeleteInboxMessages(ctx context.Context, deleteUserId int64, peer *mtproto.PeerUtil, deleteMsgDataIds []int64, cb func(ctx context.Context, userId int64, idList []int32)) error {
	var (
		deletedDialogsMap = map[int64][]*dataobject.MessagesDO{}
	)

	doDeleteMessageF := func(v *dataobject.MessagesDO) {
		if v.UserId == deleteUserId {
			return
		}

		if v2, ok := deletedDialogsMap[v.UserId]; !ok {
			deletedDialogsMap[v.UserId] = []*dataobject.MessagesDO{v}
		} else {
			deletedDialogsMap[v.UserId] = append(v2, v)
		}
	}

	switch peer.PeerType {
	case mtproto.PEER_USER:
		var err error
		if d.Postgres != nil && d.Postgres.Store != nil {
			_, err = d.SelectMessageByDataIDList(ctx, peer.PeerId, deleteMsgDataIds, func(sz, i int, v *dataobject.MessagesDO) { doDeleteMessageF(v) })
		} else {
			_, err = d.MessagesDAO.SelectByMessageDataIdListWithCB(ctx, d.MessagesDAO.CalcTableName(peer.PeerId), deleteMsgDataIds, func(sz, i int, v *dataobject.MessagesDO) { doDeleteMessageF(v) })
		}
		if err != nil {
			return err
		}
	case mtproto.PEER_CHAT:
		pUserIdList, _ := d.ChatClient.ChatGetChatParticipantIdList(ctx, &chatpb.TLChatGetChatParticipantIdList{
			ChatId: peer.PeerId,
		})

		logx.WithContext(ctx).Debugf("pUserIdList: %s", pUserIdList)

		for _, uId := range pUserIdList.GetDatas() {
			if d.Postgres != nil && d.Postgres.Store != nil {
				_, _ = d.SelectMessageByDataIDList(ctx, uId, deleteMsgDataIds, func(sz, i int, v *dataobject.MessagesDO) { doDeleteMessageF(v) })
			} else {
				d.MessagesDAO.SelectByMessageDataIdListWithCB(ctx, d.MessagesDAO.CalcTableName(uId), deleteMsgDataIds, func(sz, i int, v *dataobject.MessagesDO) { doDeleteMessageF(v) })
			}
		}
	}

	// TODO(@benqi): sort

	for userId, msgDOList := range deletedDialogsMap {
		var (
			// topMessage int32
			dialogId mtproto.DialogID
			msgIds   []int32
		)

		//if dlgDO == nil {
		//	dlgDO = &dataobject.DialogsDO{
		//		ReadInboxMaxId: math.MaxInt32,
		//		UnreadCount:    0,
		//		TopMessage:     0,
		//	}
		//	topMessage = dlgDO.TopMessage
		//}

		for i := 0; i < len(msgDOList); i++ {
			if dialogId.A == 0 && dialogId.B == 0 {
				dialogId.A = msgDOList[i].DialogId1
				dialogId.B = msgDOList[i].DialogId2
			}

			// check conversation peer_id
			if dialogId.A != msgDOList[i].DialogId1 && dialogId.B != msgDOList[i].DialogId2 {
				// dialogId
				err := mtproto.ErrMessageIdInvalid
				logx.WithContext(ctx).Errorf("deleteInboxMessages error: %v", err)
				// continue
				return err
			}
			msgIds = append(msgIds, msgDOList[i].UserMessageBoxId)
		}

		dlgDO, _ := d.SelectDialog(ctx, userId, msgDOList[0].PeerType, mtproto.GetPeerIdByDialogId(userId, dialogId))
		if dlgDO != nil {
			topMessage := dlgDO.TopMessage
			for i := 0; i < len(msgDOList); i++ {
				if msgDOList[i].UserMessageBoxId >= dlgDO.TopMessage {
					dlgDO.TopMessage -= 1
				}
				if msgDOList[i].UserMessageBoxId > dlgDO.ReadInboxMaxId {
					dlgDO.UnreadCount -= 1
				}
			}
			if !(topMessage == dlgDO.TopMessage ||
				dlgDO.TopMessage == msgDOList[len(msgDOList)-1].UserMessageBoxId) {

				dlgDO.TopMessage, _ = d.SelectDialogLastMessageIDNotIDList(ctx, userId, dialogId.A, dialogId.B, msgIds)
			}

			if dlgDO.UnreadCount < 0 {
				dlgDO.UnreadCount = 0
			}
		}

		// tR := sqlx.TxWrapper(ctx, d.DB, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		_, err2 := d.DeleteMessageByIDList(ctx, userId, msgIds)
		if err2 != nil {
			// return
		}
		if dlgDO != nil && d.Postgres != nil && d.Postgres.Store != nil {
			_, err2 = d.Postgres.Store.Dialogs.UpdateCustomMap(ctx, map[string]any{"top_message": dlgDO.TopMessage, "unread_count": dlgDO.UnreadCount}, userId, dlgDO.PeerType, dlgDO.PeerId)
		} else if dlgDO != nil {
			d.CachedConn.Exec(
				ctx,
				func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
					_, err2 = d.UpdateDialogCustomMap(
						ctx,
						map[string]interface{}{
							"top_message":  dlgDO.TopMessage,
							"unread_count": dlgDO.UnreadCount,
						},
						userId,
						dlgDO.PeerType,
						dlgDO.PeerId)

					return 0, 0, err2
				},
				dialog.GetDialogCacheKeyByPeer(userId, dlgDO.PeerType, dlgDO.PeerId))
		}
		//})
		if err2 != nil {
			return err2
		}

		if cb != nil {
			cb(ctx, userId, msgIds)
		}
	}
	return nil
}

func (d *Dao) EditUserInboxMessage(ctx context.Context, fromId, peerId int64, message *mtproto.Message) (box *mtproto.MessageBox, err error) {
	var peerMsgDO *dataobject.MessagesDO

	peerMsgDO, err = d.SelectPeerUserMessage(ctx, peerId, fromId, message.Id)
	if err != nil {
		return
	} else if peerMsgDO == nil {
		return
	}

	// message.Id
	message.Out = false
	message.Id = peerMsgDO.UserMessageBoxId
	var (
		peerMessage *mtproto.Message
	)
	jsonx.UnmarshalFromString(peerMsgDO.MessageData, &peerMessage)
	// peerMessage, _ := mtproto.DecodeMessage(int(peerMsgDO.MessageType), []byte(peerMsgDO.MessageData))
	message.FromId = peerMessage.FromId
	message.PeerId = peerMessage.PeerId
	message.ReplyTo = peerMessage.ReplyTo
	mData, _ := jsonx.Marshal(message)
	if _, err = d.UpdateMessageEdit(ctx, string(mData), message.Message, peerId, message.Id); err != nil {
		return
	}

	box = &mtproto.MessageBox{
		UserId:            peerId,
		SenderUserId:      0,
		PeerType:          mtproto.PEER_USER,
		PeerId:            peerId,
		MessageId:         message.Id,
		DialogId1:         0,
		DialogId2:         0,
		DialogMessageId:   0,
		RandomId:          0,
		Pts:               d.IDGenClient2.NextPtsId(ctx, peerId),
		PtsCount:          1,
		MessageFilterType: 0,
		Message:           message,
	}
	return
}

func (d *Dao) EditChatInboxMessage(ctx context.Context, fromId int64, peerChatId, toId int64, message *mtproto.Message) (box *mtproto.MessageBox, err error) {
	var peerMsgDO *dataobject.MessagesDO

	peerMsgDO, err = d.SelectPeerUserMessage(ctx, toId, fromId, message.Id)
	if err != nil {
		return
	} else if peerMsgDO == nil {
		return
	}

	// message.Id
	message.Out = false
	message.Id = peerMsgDO.UserMessageBoxId
	if message.GetReplyTo() != nil {
		var (
			peerMessage *mtproto.Message
		)
		// peerMessage, _ := mtproto.DecodeMessage(int(peerMsgDO.MessageType), []byte(peerMsgDO.MessageData))
		jsonx.UnmarshalFromString(peerMsgDO.MessageData, &peerMessage)
		message.ReplyTo = peerMessage.ReplyTo
	}

	mData, _ := jsonx.Marshal(message)
	if _, err = d.UpdateMessageEdit(ctx, string(mData), message.Message, toId, message.Id); err != nil {
		return
	}

	box = &mtproto.MessageBox{
		UserId:            toId,
		SenderUserId:      0,
		PeerType:          mtproto.PEER_CHAT,
		PeerId:            peerChatId,
		MessageId:         message.Id,
		DialogId1:         0,
		DialogId2:         0,
		DialogMessageId:   0,
		RandomId:          0,
		Pts:               d.IDGenClient2.NextPtsId(ctx, toId),
		PtsCount:          1,
		MessageFilterType: 0,
		Message:           message,
	}
	return
}
