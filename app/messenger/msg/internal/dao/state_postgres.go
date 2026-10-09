package dao

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
)

// errMessageStateAlreadyApplied tells MutateMessageState that the Kafka
// delivery receipt already committed this operation. The retry is successful,
// but must not allocate another PTS or enqueue another sync publication.
var errMessageStateAlreadyApplied = errors.New("messenger/msg: message state already applied")

// MutateMessageState serializes a user's message mutations with sends and
// commits their PTS allocation and difference records in the same transaction.
func (d *Dao) MutateMessageState(ctx context.Context, userID int64, fn func(pgx.Tx) ([]*mtproto.Update, error)) ([]*mtproto.Update, int32, error) {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil || d.Postgres.Store == nil {
		return nil, 0, errors.New("messenger/msg: PostgreSQL message state is not configured")
	}
	if userID <= 0 {
		return nil, 0, mtproto.ErrInputRequestInvalid
	}
	var updates []*mtproto.Update
	var pts int32
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('messenger-user:' || $1::bigint::text, 0))`, userID); err != nil {
			return err
		}
		var err error
		updates, err = fn(tx)
		if errors.Is(err, errMessageStateAlreadyApplied) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, update := range updates {
			if update == nil || update.PtsCount <= 0 {
				return errors.New("messenger/msg: mutation requires a positive PTS count")
			}
			if update.Pts_INT32 == 0 {
				value, err := counter.NextOn(ctx, tx, counter.PtsKey(userID), int64(update.PtsCount))
				if err != nil {
					return err
				}
				update.Pts_INT32 = int32(value)
				if _, err := d.AddToPtsQueueOn(ctx, tx, userID, update.Pts_INT32, update.PtsCount, update); err != nil {
					return err
				}
			}
			pts = update.Pts_INT32
		}
		if len(updates) == 0 {
			return tx.QueryRow(ctx, `SELECT COALESCE((SELECT value FROM idgen_counters WHERE key=$1),0)`, counter.PtsKey(userID)).Scan(&pts)
		}
		return d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &syncpb.TLSyncPushUpdates{UserId: userID, Updates: mtproto.MakeUpdatesByUpdates(updates...)})
	})
	if err != nil {
		return nil, 0, err
	}
	return updates, pts, nil
}

func (d *Dao) CurrentMessagePts(ctx context.Context, userID int64) (int32, error) {
	if d == nil || d.Postgres == nil || d.Postgres.Pool == nil {
		return 0, errors.New("messenger/msg: PostgreSQL message state is not configured")
	}
	value, err := counter.NewCounterStore(d.Postgres.Pool).Current(ctx, counter.PtsKey(userID))
	return int32(value), err
}

func validStatePeer(peer *mtproto.PeerUtil) bool {
	return peer != nil && peer.PeerId > 0 && (peer.PeerType == mtproto.PEER_USER || peer.PeerType == mtproto.PEER_CHAT)
}

func authorizeChatStateOn(ctx context.Context, tx pgx.Tx, userID, chatID int64, pin bool) error {
	var creator, banned int64
	var migrated int64
	var deactivated bool
	if err := tx.QueryRow(ctx, `SELECT creator_user_id,default_banned_rights,migrated_to_id,deactivated FROM chats WHERE id=$1 FOR SHARE`, chatID).Scan(&creator, &banned, &migrated, &deactivated); err != nil {
		if err == pgx.ErrNoRows {
			return mtproto.ErrChatIdInvalid
		}
		return err
	}
	var state, participantType, rights int32
	if err := tx.QueryRow(ctx, `SELECT state,participant_type,admin_rights FROM chat_participants WHERE chat_id=$1 AND user_id=$2 FOR SHARE`, chatID, userID).Scan(&state, &participantType, &rights); err != nil {
		if err == pgx.ErrNoRows {
			return mtproto.ErrChatWriteForbidden
		}
		return err
	}
	if state != mtproto.ChatMemberStateNormal || migrated != 0 || deactivated {
		return mtproto.ErrChatWriteForbidden
	}
	if pin && creator != userID && participantType != mtproto.ChatMemberCreator &&
		((participantType == mtproto.ChatMemberAdmin && rights&mtproto.ADMIN_PIN_MESSAGES == 0) ||
			(participantType != mtproto.ChatMemberAdmin && banned&mtproto.BANNED_PIN_MESSAGES != 0)) {
		return mtproto.ErrChatAdminRequired
	}
	return nil
}

func (d *Dao) refreshMessageDialogOn(ctx context.Context, tx pgx.Tx, userID int64, peerType int32, peerID int64) error {
	_, err := tx.Exec(ctx, `UPDATE dialogs d SET
 top_message=COALESCE((SELECT max(user_message_box_id) FROM messages m WHERE m.user_id=d.user_id AND m.peer_type=d.peer_type AND m.peer_id=d.peer_id AND NOT m.deleted),0),
 pinned_msg_id=COALESCE((SELECT max(user_message_box_id) FROM messages m WHERE m.user_id=d.user_id AND m.peer_type=d.peer_type AND m.peer_id=d.peer_id AND m.pinned AND NOT m.deleted),0),
 unread_count=(SELECT count(*) FROM messages m WHERE m.user_id=d.user_id AND m.peer_type=d.peer_type AND m.peer_id=d.peer_id AND m.sender_user_id<>d.user_id AND m.user_message_box_id>d.read_inbox_max_id AND NOT m.deleted),
 unread_mentions_count=(SELECT count(*) FROM messages m WHERE m.user_id=d.user_id AND m.peer_type=d.peer_type AND m.peer_id=d.peer_id AND m.mentioned AND NOT m.deleted),
 unread_reactions_count=(SELECT count(*) FROM messages m WHERE m.user_id=d.user_id AND m.peer_type=d.peer_type AND m.peer_id=d.peer_id AND m.reaction_unread AND NOT m.deleted)
 WHERE d.user_id=$1 AND d.peer_type=$2 AND d.peer_id=$3`, userID, peerType, peerID)
	return err
}

func (d *Dao) DeleteMessageState(ctx context.Context, userID int64, ids []int32, revoke bool) ([]*mtproto.Update, int32, []dataobject.MessagesDO, error) {
	var deleted []dataobject.MessagesDO
	updates, pts, err := d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		var err error
		deleted, err = postgres_dao.NewMessagesDAO(tx).SelectByMessageIdList(ctx, userID, ids)
		if err != nil || len(deleted) == 0 {
			return nil, err
		}
		actualIDs := make([]int32, 0, len(deleted))
		peers := make(map[mtproto.DialogID]*mtproto.PeerUtil)
		for _, row := range deleted {
			if revoke && row.PeerType == mtproto.PEER_CHAT {
				if err := authorizeChatStateOn(ctx, tx, userID, row.PeerId, false); err != nil {
					return nil, err
				}
				if row.SenderUserId != userID {
					var allowed bool
					if err := tx.QueryRow(ctx, `SELECT c.creator_user_id=$2 OR p.participant_type=$3 OR (p.participant_type=$4 AND (p.admin_rights & $5)<>0) FROM chats c JOIN chat_participants p ON p.chat_id=c.id AND p.user_id=$2 WHERE c.id=$1`, row.PeerId, userID, mtproto.ChatMemberCreator, mtproto.ChatMemberAdmin, mtproto.ADMIN_DELETE_MESSAGES).Scan(&allowed); err != nil {
						return nil, err
					}
					if !allowed {
						return nil, mtproto.ErrMessageDeleteForbidden
					}
				}
			}
			actualIDs = append(actualIDs, row.UserMessageBoxId)
			peers[mtproto.DialogID{A: row.DialogId1, B: row.DialogId2}] = mtproto.MakePeerUtil(row.PeerType, row.PeerId)
		}
		if _, err := d.Postgres.Store.Messages.DeleteMessagesByMessageIdListOn(ctx, tx, userID, actualIDs); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE hash_tags SET deleted=TRUE WHERE user_id=$1 AND hash_tag_message_id=ANY($2::integer[])`, userID, actualIDs); err != nil {
			return nil, err
		}
		for _, peer := range peers {
			if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
				return nil, err
			}
		}
		if revoke {
			byPeer := make(map[mtproto.DialogID][]int64)
			for _, row := range deleted {
				id := mtproto.DialogID{A: row.DialogId1, B: row.DialogId2}
				byPeer[id] = append(byPeer[id], row.DialogMessageId)
			}
			for id, dataIDs := range byPeer {
				peer := peers[id]
				if !peer.IsSelfUser(userID) {
					if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &inbox.TLInboxDeleteMessagesToInbox{FromId: userID, PeerType: peer.PeerType, PeerId: peer.PeerId, Id: dataIDs}); err != nil {
						return nil, err
					}
				}
			}
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{Messages: actualIDs, PtsCount: int32(len(actualIDs))}).To_Update()}, nil
	})
	if err != nil {
		return nil, 0, nil, err
	}
	return updates, pts, deleted, nil
}

func (d *Dao) ReadMessageContentsState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, ids []int32) ([]*mtproto.Update, int32, error) {
	if !validStatePeer(peer) {
		return nil, 0, mtproto.ErrPeerIdInvalid
	}
	return d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		messages, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageIdList(ctx, userID, ids)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `UPDATE messages SET mentioned=FALSE,media_unread=FALSE,reaction_unread=FALSE
 WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND user_message_box_id=ANY($4::integer[])
 AND NOT deleted AND (mentioned OR media_unread OR reaction_unread) RETURNING user_message_box_id`, userID, peer.PeerType, peer.PeerId, ids)
		if err != nil {
			return nil, err
		}
		actualIDs, err := pgx.CollectRows(rows, pgx.RowTo[int32])
		if err != nil || len(actualIDs) == 0 {
			return nil, err
		}
		sort.Slice(actualIDs, func(i, j int) bool { return actualIDs[i] < actualIDs[j] })
		if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
			return nil, err
		}
		for _, message := range messages {
			if message.PeerType != peer.PeerType || message.PeerId != peer.PeerId || !message.MediaUnread || message.SenderUserId == userID {
				continue
			}
			peerID := peer.PeerId
			if peer.PeerType == mtproto.PEER_USER {
				peerID = userID
			}
			if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &inbox.TLInboxReadMediaUnreadToInboxV2{UserId: message.SenderUserId, PeerType: peer.PeerType, PeerId: peerID, DialogMessageId: message.DialogMessageId}); err != nil {
				return nil, err
			}
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{Messages: actualIDs, PtsCount: int32(len(actualIDs))}).To_Update()}, nil
	})
}

func (d *Dao) ReadMentionsState(ctx context.Context, userID, chatID int64, topID int32, hasTopID bool) ([]*mtproto.Update, int32, error) {
	return d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		rows, err := tx.Query(ctx, `UPDATE messages SET mentioned=FALSE
 WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND mentioned AND NOT deleted
 AND (NOT $4::boolean OR user_message_box_id<=$5) RETURNING user_message_box_id`, userID, mtproto.PEER_CHAT, chatID, hasTopID, topID)
		if err != nil {
			return nil, err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[int32])
		if err != nil || len(ids) == 0 {
			return nil, err
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if err := d.refreshMessageDialogOn(ctx, tx, userID, mtproto.PEER_CHAT, chatID); err != nil {
			return nil, err
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{Messages: ids, PtsCount: int32(len(ids))}).To_Update()}, nil
	})
}

func (d *Dao) ReadHistoryState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, maxID int32) ([]*mtproto.Update, int32, *dataobject.MessagesDO, error) {
	if !validStatePeer(peer) || maxID < 0 {
		return nil, 0, nil, mtproto.ErrPeerIdInvalid
	}
	var maxMessage *dataobject.MessagesDO
	updates, pts, err := d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		dialog, err := postgres_dao.NewDialogsDAO(tx).SelectDialog(ctx, userID, peer.PeerType, peer.PeerId)
		if err != nil {
			return nil, err
		}
		if dialog == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		if maxID == 0 || maxID > dialog.TopMessage {
			maxID = dialog.TopMessage
		}
		if peer.IsSelfUser(userID) || maxID <= dialog.ReadInboxMaxId {
			return nil, nil
		}
		maxMessage, err = postgres_dao.NewMessagesDAO(tx).SelectByMessageId(ctx, userID, maxID)
		if err != nil {
			return nil, err
		}
		if maxMessage == nil || maxMessage.PeerType != peer.PeerType || maxMessage.PeerId != peer.PeerId {
			return nil, mtproto.ErrMessageIdInvalid
		}
		// Reading history advances the inbox cursor only through an incoming
		// message.  The legacy path treats an outgoing target as no-op; keep
		// that behavior so a sender cannot manufacture an inbox read update for
		// its own message when max_id points at the dialog's top outbox row.
		if maxMessage.SenderUserId == userID {
			return nil, nil
		}
		if _, err := tx.Exec(ctx, `UPDATE dialogs SET read_inbox_max_id=GREATEST(read_inbox_max_id,$4),unread_mark=FALSE WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, userID, peer.PeerType, peer.PeerId, maxID); err != nil {
			return nil, err
		}
		if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `SELECT DISTINCT ON (sender_user_id) sender_user_id,dialog_message_id,user_message_box_id
 FROM messages WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND sender_user_id<>$1
 AND user_message_box_id<=$4 AND NOT deleted ORDER BY sender_user_id,user_message_box_id DESC`, userID, peer.PeerType, peer.PeerId, maxID)
		if err != nil {
			return nil, err
		}
		type senderCursor struct {
			UserID    int64
			DialogID  int64
			MessageID int32
		}
		cursors, err := pgx.CollectRows(rows, pgx.RowToStructByPos[senderCursor])
		if err != nil {
			return nil, err
		}
		for _, cursor := range cursors {
			if peer.PeerType == mtproto.PEER_CHAT {
				var senderMessageID int32
				if err := tx.QueryRow(ctx, `SELECT user_message_box_id FROM messages
 WHERE user_id=$1 AND dialog_message_id=$2 AND peer_type=$3 AND peer_id=$4
 AND sender_user_id=$1 AND NOT deleted ORDER BY user_message_box_id DESC LIMIT 1`,
					cursor.UserID, cursor.DialogID, peer.PeerType, peer.PeerId).Scan(&senderMessageID); err != nil {
					return nil, err
				}
				if _, _, err := d.Postgres.Store.MessageReadOutbox.InsertOrUpdateOn(ctx, tx, &dataobject.MessageReadOutboxDO{
					UserId:            cursor.UserID,
					PeerDialogId:      mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId),
					ReadUserId:        userID,
					ReadOutboxMaxId:   senderMessageID,
					ReadOutboxMaxDate: time.Now().Unix(),
				}); err != nil {
					return nil, err
				}
			}
			peerID := peer.PeerId
			if peer.PeerType == mtproto.PEER_USER {
				peerID = userID
			}
			if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &inbox.TLInboxReadOutboxHistory{UserId: cursor.UserID, PeerType: peer.PeerType, PeerId: peerID, MaxDialogMessageId: cursor.DialogID}); err != nil {
				return nil, err
			}
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{Peer_PEER: peer.ToPeer(), MaxId: maxID, PtsCount: 1}).To_Update()}, nil
	})
	return updates, pts, maxMessage, err
}

func (d *Dao) EditMessageState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, messageID int32, replacement *mtproto.Message, deliveries ...*inbox.TLInboxEditMessageToInboxV2) (*mtproto.MessageBox, error) {
	if !validStatePeer(peer) || replacement == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var box *mtproto.MessageBox
	updates, _, err := d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		if peer.PeerType == mtproto.PEER_CHAT {
			if err := authorizeChatStateOn(ctx, tx, userID, peer.PeerId, false); err != nil {
				return nil, err
			}
		}
		row, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageId(ctx, userID, messageID)
		if err != nil {
			return nil, err
		}
		if row == nil || row.PeerType != peer.PeerType || row.PeerId != peer.PeerId || row.SenderUserId != userID {
			return nil, mtproto.ErrMessageIdInvalid
		}
		message := new(mtproto.Message)
		if err := jsonx.UnmarshalFromString(row.MessageData, message); err != nil {
			return nil, err
		}
		message.Message, message.Action, message.Media = replacement.Message, replacement.Action, replacement.Media
		message.ReplyMarkup, message.Entities = replacement.ReplyMarkup, replacement.Entities
		message.EditDate, message.EditHide = replacement.EditDate, replacement.EditHide
		data, err := jsonx.Marshal(message)
		if err != nil {
			return nil, err
		}
		if _, err := d.UpdateMessageEditOn(ctx, tx, string(data), message.Message, userID, messageID); err != nil {
			return nil, err
		}
		if _, err := d.Postgres.Store.HashTags.DeleteHashTagMessageIdOn(ctx, tx, userID, messageID); err != nil {
			return nil, err
		}
		for _, entity := range message.Entities {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag && entity.Url != "" {
				if _, _, err := d.Postgres.Store.HashTags.InsertOrUpdateOn(ctx, tx, &dataobject.HashTagsDO{UserId: userID, PeerType: peer.PeerType, PeerId: peer.PeerId, HashTag: entity.Url, HashTagMessageId: messageID}); err != nil {
					return nil, err
				}
			}
		}
		box = &mtproto.MessageBox{UserId: userID, MessageId: messageID, SenderUserId: row.SenderUserId, PeerType: row.PeerType, PeerId: row.PeerId, RandomId: row.RandomId, DialogId1: row.DialogId1, DialogId2: row.DialogId2, DialogMessageId: row.DialogMessageId, Message: message, PtsCount: 1, Mentioned: row.Mentioned, MediaUnread: row.MediaUnread, Pinned: row.Pinned}
		for _, delivery := range deliveries {
			if delivery == nil || delivery.UserId <= 0 || delivery.UserId == userID || delivery.FromId != userID || delivery.PeerType != peer.PeerType || delivery.PeerId != peer.PeerId {
				return nil, mtproto.ErrInputRequestInvalid
			}
			if peer.PeerType == mtproto.PEER_USER {
				if delivery.UserId != peer.PeerId {
					return nil, mtproto.ErrInputRequestInvalid
				}
				var blocked bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_peer_blocks WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND NOT deleted)`, peer.PeerId, mtproto.PEER_USER, userID).Scan(&blocked); err != nil {
					return nil, err
				}
				if blocked {
					continue
				}
			} else {
				var member bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chat_participants WHERE chat_id=$1 AND user_id=$2 AND state=$3)`, peer.PeerId, delivery.UserId, mtproto.ChatMemberStateNormal).Scan(&member); err != nil {
					return nil, err
				}
				if !member {
					continue
				}
			}
			request := proto.Clone(delivery).(*inbox.TLInboxEditMessageToInboxV2)
			request.NewMessage = box
			if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, request); err != nil {
				return nil, err
			}
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateEditMessage(&mtproto.Update{Message_MESSAGE: message, PtsCount: 1}).To_Update()}, nil
	})
	if err != nil {
		return nil, err
	}
	box.Pts = updates[0].Pts_INT32
	return box, nil
}

func (d *Dao) PinMessageState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, messageID int32, pinned, fanout bool) ([]*mtproto.Update, int32, error) {
	return d.pinMessageState(ctx, userID, peer, messageID, pinned, fanout, nil, nil)
}

func (d *Dao) PinMessageWithServiceState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, messageID int32, service *msg.OutboxMessage, recipients []*inbox.TLInboxSendUserMessageToInboxV2) ([]*mtproto.Update, int32, error) {
	return d.pinMessageState(ctx, userID, peer, messageID, true, true, service, recipients)
}

func (d *Dao) pinMessageState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, messageID int32, pinned, fanout bool, service *msg.OutboxMessage, recipients []*inbox.TLInboxSendUserMessageToInboxV2) ([]*mtproto.Update, int32, error) {
	if !validStatePeer(peer) {
		return nil, 0, mtproto.ErrPeerIdInvalid
	}
	return d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		if fanout && peer.PeerType == mtproto.PEER_CHAT {
			if err := authorizeChatStateOn(ctx, tx, userID, peer.PeerId, true); err != nil {
				return nil, err
			}
		}
		row, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageId(ctx, userID, messageID)
		if err != nil {
			return nil, err
		}
		if row == nil || row.PeerType != peer.PeerType || row.PeerId != peer.PeerId {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if row.Pinned == pinned {
			return nil, nil
		}
		if _, err := d.Postgres.Store.Messages.UpdatePinnedOn(ctx, tx, pinned, userID, messageID); err != nil {
			return nil, err
		}
		if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
			return nil, err
		}
		if fanout && !peer.IsSelfUser(userID) {
			if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &inbox.TLInboxUpdatePinnedMessage{UserId: userID, Unpin: !pinned, PeerType: peer.PeerType, PeerId: peer.PeerId, Id: messageID, DialogMessageId: row.DialogMessageId}); err != nil {
				return nil, err
			}
		}
		pinUpdate := mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Pinned: pinned, Peer_PEER: peer.ToPeer(), Messages: []int32{messageID}, PtsCount: 1}).To_Update()
		if service == nil {
			return []*mtproto.Update{pinUpdate}, nil
		}
		if peer.PeerType == mtproto.PEER_USER {
			recipients, err = authorizeUserDeliveryOn(ctx, tx, userID, peer.PeerId, recipients)
		} else {
			recipients, err = authorizeChatDeliveryOn(ctx, tx, userID, peer.PeerId, []*msg.OutboxMessage{service}, recipients)
		}
		if err != nil {
			return nil, err
		}
		pts, err := counter.NextOn(ctx, tx, counter.PtsKey(userID), 1)
		if err != nil {
			return nil, err
		}
		pinUpdate.Pts_INT32 = int32(pts)
		if _, err := d.AddToPtsQueueOn(ctx, tx, userID, int32(pts), 1, pinUpdate); err != nil {
			return nil, err
		}
		box, _, err := d.sendMessageToOutboxPostgresOn(ctx, tx, userID, peer, service)
		if err != nil {
			return nil, err
		}
		for _, recipient := range recipients {
			request := proto.Clone(recipient).(*inbox.TLInboxSendUserMessageToInboxV2)
			request.BoxList = []*mtproto.MessageBox{box}
			if err := d.EnqueueInboxDeliveryOn(ctx, tx, request); err != nil {
				return nil, err
			}
		}
		return []*mtproto.Update{pinUpdate, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: box.Message, Pts_INT32: box.Pts, PtsCount: 1}).To_Update()}, nil
	})
}

func (d *Dao) UnpinMessageState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, fanout bool) ([]*mtproto.Update, int32, error) {
	if !validStatePeer(peer) {
		return nil, 0, mtproto.ErrPeerIdInvalid
	}
	return d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		if fanout && peer.PeerType == mtproto.PEER_CHAT {
			if err := authorizeChatStateOn(ctx, tx, userID, peer.PeerId, true); err != nil {
				return nil, err
			}
		}
		rows, err := tx.Query(ctx, `UPDATE messages SET pinned=FALSE WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND pinned AND NOT deleted RETURNING user_message_box_id`, userID, peer.PeerType, peer.PeerId)
		if err != nil {
			return nil, err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[int32])
		if err != nil || len(ids) == 0 {
			return nil, err
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
			return nil, err
		}
		if fanout && !peer.IsSelfUser(userID) {
			if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &inbox.TLInboxUnpinAllMessages{UserId: userID, PeerType: peer.PeerType, PeerId: peer.PeerId}); err != nil {
				return nil, err
			}
		}
		return []*mtproto.Update{mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Peer_PEER: peer.ToPeer(), Messages: ids, PtsCount: int32(len(ids))}).To_Update()}, nil
	})
}

func (d *Dao) ReadOutboxHistoryState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, dialogMessageID int64) error {
	if !validStatePeer(peer) {
		return mtproto.ErrPeerIdInvalid
	}
	_, _, err := d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		row, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageDataId(ctx, userID, dialogMessageID)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, nil
		}
		if row.PeerType != peer.PeerType || row.PeerId != peer.PeerId {
			return nil, mtproto.ErrPeerIdInvalid
		}
		tag, err := tx.Exec(ctx, `UPDATE dialogs SET read_outbox_max_id=$4 WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND read_outbox_max_id<$4`, userID, peer.PeerType, peer.PeerId, row.UserMessageBoxId)
		if err != nil || tag.RowsAffected() == 0 {
			return nil, err
		}
		if peer.PeerType == mtproto.PEER_USER {
			if _, _, err := d.Postgres.Store.MessageReadOutbox.InsertOrUpdateOn(ctx, tx, &dataobject.MessageReadOutboxDO{UserId: userID, PeerDialogId: mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId), ReadUserId: peer.PeerId, ReadOutboxMaxId: row.UserMessageBoxId, ReadOutboxMaxDate: time.Now().Unix()}); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE messages SET outbox_read_date=EXTRACT(EPOCH FROM clock_timestamp())::bigint WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND user_message_box_id<=$4 AND sender_user_id=$1 AND outbox_read_date=0`, userID, peer.PeerType, peer.PeerId, row.UserMessageBoxId); err != nil {
			return nil, err
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateReadHistoryOutbox(&mtproto.Update{Peer_PEER: peer.ToPeer(), MaxId: row.UserMessageBoxId, PtsCount: 1}).To_Update()}, nil
	})
	return err
}

func (d *Dao) DeletePhoneCallState(ctx context.Context, userID int64, revoke bool) ([]*mtproto.Update, int32, []dataobject.MessagesDO, error) {
	rows, err := d.Postgres.Store.Messages.SelectPhoneCallList(ctx, userID, mtproto.MEDIA_PHONE_CALL, math.MaxInt32, math.MaxInt32)
	if err != nil {
		return nil, 0, nil, err
	}
	ids := make([]int32, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserMessageBoxId)
	}
	return d.DeleteMessageState(ctx, userID, ids, revoke)
}
