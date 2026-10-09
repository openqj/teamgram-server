package dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

// DeleteChatUserHistory marks every message view authored by deleteUserID in
// the basic-group conversation as deleted in one authoritative transaction.
// The operation is used by msg.deleteChatHistory and must not expose a
// partially deleted history if the database rejects the write.
func (d *Dao) DeleteChatUserHistory(ctx context.Context, chatID, deleteUserID int64) error {
	if d == nil || d.Postgres == nil || d.Postgres.Store == nil {
		return errors.New("messenger/msg: postgres store is not configured")
	}
	if chatID <= 0 || deleteUserID <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
UPDATE messages
SET deleted = TRUE
WHERE peer_type = $1
  AND peer_id = $2
  AND sender_user_id = $3
  AND deleted = FALSE`, mtproto.PEER_CHAT, chatID, deleteUserID)
		return err
	})
}

// InsertOrUpdateHashTag writes through the PostgreSQL aggregate when the
// service has been configured with the authoritative store. Keeping the
// fallback here, at the adapter boundary, prevents handlers from acquiring a
// new dependency on the legacy MySQL DAO.
func (d *Dao) InsertOrUpdateHashTag(ctx context.Context, do *dataobject.HashTagsDO) (int64, int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.HashTags != nil {
		return d.Postgres.Store.HashTags.InsertOrUpdate(ctx, do)
	}
	return d.HashTagsDAO.InsertOrUpdate(ctx, do)
}

func (d *Dao) DeleteHashTagMessageID(ctx context.Context, userID int64, messageID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.HashTags != nil {
		return d.Postgres.Store.HashTags.DeleteHashTagMessageId(ctx, userID, messageID)
	}
	return d.HashTagsDAO.DeleteHashTagMessageId(ctx, userID, messageID)
}

func (d *Dao) InsertOrUpdateMessageReadOutbox(ctx context.Context, do *dataobject.MessageReadOutboxDO) (int64, int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.MessageReadOutbox != nil {
		return d.Postgres.Store.MessageReadOutbox.InsertOrUpdate(ctx, do)
	}
	return d.MessageReadOutboxDAO.InsertOrUpdate(ctx, do)
}

func (d *Dao) InsertOrUpdateDialog(ctx context.Context, do *dataobject.DialogsDO) (int64, int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Dialogs != nil {
		return d.Postgres.Store.Dialogs.InsertOrUpdate(ctx, do)
	}
	return d.DialogsDAO.InsertOrUpdate(ctx, do)
}

func (d *Dao) InsertIgnoreDialog(ctx context.Context, do *dataobject.DialogsDO) (int64, int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Dialogs != nil {
		return d.Postgres.Store.Dialogs.InsertIgnore(ctx, do)
	}
	return d.DialogsDAO.InsertIgnore(ctx, do)
}

func (d *Dao) InsertOrUpdateSavedDialog(ctx context.Context, do *dataobject.SavedDialogsDO) (int64, int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.SavedDialogs != nil {
		return d.Postgres.Store.SavedDialogs.InsertOrUpdate(ctx, do)
	}
	return d.SavedDialogsDAO.InsertOrUpdate(ctx, do)
}

func (d *Dao) SelectDialog(ctx context.Context, userID int64, peerType int32, peerID int64) (*dataobject.DialogsDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Dialogs != nil {
		return d.Postgres.Store.Dialogs.SelectDialog(ctx, userID, peerType, peerID)
	}
	return d.DialogsDAO.SelectDialog(ctx, userID, peerType, peerID)
}

func (d *Dao) SelectPeerDialogList(ctx context.Context, userID int64, ids []int64) ([]dataobject.DialogsDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Dialogs != nil {
		return d.Postgres.Store.Dialogs.SelectPeerDialogList(ctx, userID, ids)
	}
	return d.DialogsDAO.SelectPeerDialogList(ctx, userID, ids)
}

func (d *Dao) SelectPeerDialogListWithCB(ctx context.Context, userID int64, ids []int64, cb func(int, int, *dataobject.DialogsDO)) ([]dataobject.DialogsDO, error) {
	list, err := d.SelectPeerDialogList(ctx, userID, ids)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *Dao) SelectChatParticipants(ctx context.Context, chatID int64, cb func(int, int, *dataobject.ChatParticipantsDO)) ([]dataobject.ChatParticipantsDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.ChatParticipants != nil {
		list, err := d.Postgres.Store.ChatParticipants.SelectList(ctx, chatID)
		if cb != nil {
			for i := range list {
				cb(len(list), i, &list[i])
			}
		}
		return list, err
	}
	return nil, nil
}

func (d *Dao) UpdateDialogCustomMap(ctx context.Context, values map[string]any, userID int64, peerType int32, peerID int64) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Dialogs != nil {
		return d.Postgres.Store.Dialogs.UpdateCustomMap(ctx, values, userID, peerType, peerID)
	}
	return d.DialogsDAO.UpdateCustomMap(ctx, values, userID, peerType, peerID)
}

func (d *Dao) UpdateDialogUnreadCount(ctx context.Context, unreadCount, unreadMentionsCount, unreadReactionsCount int32, userID int64, peerType int32, peerID int64) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Dialogs != nil {
		return d.Postgres.Store.Dialogs.UpdateUnreadCount(ctx, unreadCount, unreadMentionsCount, unreadReactionsCount, userID, peerType, peerID)
	}
	return d.DialogsDAO.UpdateUnreadCount(ctx, unreadCount, unreadMentionsCount, unreadReactionsCount, userID, peerType, peerID)
}

func (d *Dao) SelectMessageByID(ctx context.Context, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageId(ctx, userID, messageID)
	}
	return d.MessagesDAO.SelectByMessageId(ctx, userID, messageID)
}

func (d *Dao) SelectMessageByDataID(ctx context.Context, userID, dialogMessageID int64) (*dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageDataId(ctx, userID, dialogMessageID)
	}
	return d.MessagesDAO.SelectByMessageDataId(ctx, userID, dialogMessageID)
}

func (d *Dao) SelectPeerUserMessage(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectPeerUserMessage(ctx, peerID, userID, messageID)
	}
	return d.MessagesDAO.SelectPeerUserMessage(ctx, peerID, userID, messageID)
}

func (d *Dao) SelectMessageByDataIDList(ctx context.Context, userID int64, ids []int64, cb func(int, int, *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		list, err := d.Postgres.Store.Messages.SelectByMessageDataIdList(ctx, userID, ids)
		if cb != nil {
			for i := range list {
				cb(len(list), i, &list[i])
			}
		}
		return list, err
	}
	return nil, nil
}

func (d *Dao) SelectPeerUserMessageId(ctx context.Context, peerID, userID int64, messageID int32) (*dataobject.MessagesDO, error) {
	return d.SelectPeerUserMessage(ctx, peerID, userID, messageID)
}

func (d *Dao) SelectMessagesByDataIDUsers(ctx context.Context, dialogMessageID int64, userIDs []int64, cb func(int, int, *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		list, err := d.Postgres.Store.Messages.SelectByMessageDataIdUserIdList(ctx, dialogMessageID, userIDs)
		if cb != nil {
			for i := range list {
				cb(len(list), i, &list[i])
			}
		}
		return list, err
	}
	return nil, nil
}

func (d *Dao) SelectMessageByRandomID(ctx context.Context, senderID, randomID int64) (*dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByRandomId(ctx, senderID, randomID)
	}
	return d.MessagesDAO.SelectByRandomId(ctx, d.MessagesDAO.CalcTableName(senderID), senderID, randomID)
}

func (d *Dao) SelectMessageByIDList(ctx context.Context, userID int64, ids []int32) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectByMessageIdList(ctx, userID, ids)
	}
	return d.MessagesDAO.SelectByMessageIdList(ctx, userID, ids)
}

func (d *Dao) SelectMessageByIDListWithCB(ctx context.Context, userID int64, ids []int32, cb func(int, int, *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	list, err := d.SelectMessageByIDList(ctx, userID, ids)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *Dao) SelectDialogMessageList(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectDialogMessageIdList(ctx, userID, dialogID1, dialogID2)
	}
	return d.MessagesDAO.SelectDialogMessageIdList(ctx, userID, dialogID1, dialogID2)
}

func (d *Dao) SelectDialogMessageListWithCB(ctx context.Context, userID, dialogID1, dialogID2 int64, cb func(int, int, *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	list, err := d.SelectDialogMessageList(ctx, userID, dialogID1, dialogID2)
	if cb != nil {
		for i := range list {
			cb(len(list), i, &list[i])
		}
	}
	return list, err
}

func (d *Dao) SelectDialogLastMessages(ctx context.Context, userID, dialogID1, dialogID2 int64, limit int32) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectDialogLastMessageList(ctx, userID, dialogID1, dialogID2, limit)
	}
	return d.MessagesDAO.SelectDialogLastMessageList(ctx, userID, dialogID1, dialogID2, limit)
}

func (d *Dao) SelectPhoneCallMessages(ctx context.Context, userID int64, mediaType int32, offset, limit int32, cb func(int, int, *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		list, err := d.Postgres.Store.Messages.SelectPhoneCallList(ctx, userID, mediaType, offset, limit)
		if cb != nil {
			for i := range list {
				cb(len(list), i, &list[i])
			}
		}
		return list, err
	}
	return nil, nil
}

func (d *Dao) DeleteMessageByIDList(ctx context.Context, userID int64, ids []int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.DeleteMessagesByMessageIdList(ctx, userID, ids)
	}
	return d.MessagesDAO.DeleteMessagesByMessageIdList(ctx, userID, ids)
}

func (d *Dao) DeleteMessageByIDListOn(ctx context.Context, tx postgres_dao.DB, userID int64, ids []int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.DeleteMessagesByMessageIdListOn(ctx, tx, userID, ids)
	}
	return 0, nil
}

func (d *Dao) UpdateMessagePinned(ctx context.Context, pinned bool, userID int64, messageID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdatePinned(ctx, pinned, userID, messageID)
	}
	return d.MessagesDAO.UpdatePinned(ctx, pinned, userID, messageID)
}

func (d *Dao) UpdateMessageMediaUnread(ctx context.Context, userID int64, messageID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateMediaUnread(ctx, userID, messageID)
	}
	return d.MessagesDAO.UpdateMediaUnread(ctx, userID, messageID)
}

func (d *Dao) UpdateMentionedAndMediaUnread(ctx context.Context, userID int64, messageID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateMentionedAndMediaUnread(ctx, userID, messageID)
	}
	return d.MessagesDAO.UpdateMentionedAndMediaUnread(ctx, userID, messageID)
}

func (d *Dao) CountMentionedMessages(ctx context.Context, userID int64, peerType int32, peerID int64) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountMentioned(ctx, userID, peerType, peerID)
	}
	return 0, nil
}

func (d *Dao) CountUnreadIncoming(ctx context.Context, userID, dialogID1, dialogID2, senderID int64, fromID, toID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountUnreadIncoming(ctx, userID, dialogID1, dialogID2, senderID, fromID, toID)
	}
	return 0, nil
}

func (d *Dao) SelectLastTwoPinnedMessages(ctx context.Context, userID, dialogID1, dialogID2 int64) ([]int32, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectLastTwoPinnedList(ctx, userID, dialogID1, dialogID2)
	}
	return d.MessagesDAO.SelectLastTwoPinnedList(ctx, userID, dialogID1, dialogID2)
}

func (d *Dao) SelectPinnedMessages(ctx context.Context, userID, dialogID1, dialogID2 int64, cb func(int, int, *dataobject.MessagesDO)) ([]dataobject.MessagesDO, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		list, err := d.Postgres.Store.Messages.SelectPinnedList(ctx, userID, dialogID1, dialogID2)
		if cb != nil {
			for i := range list {
				cb(len(list), i, &list[i])
			}
		}
		return list, err
	}
	return nil, nil
}

func (d *Dao) UpdateUnPinnedMessages(ctx context.Context, userID int64, ids []int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateUnPinnedByIdList(ctx, userID, ids)
	}
	return d.MessagesDAO.UpdateUnPinnedByIdList(ctx, userID, ids)
}

func (d *Dao) UpdateUnpinnedByIDList(ctx context.Context, userID int64, ids []int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateUnPinnedByIdList(ctx, userID, ids)
	}
	return 0, nil
}

func (d *Dao) SelectDialogLastMessageID(ctx context.Context, userID, dialogID1, dialogID2 int64) (int32, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectDialogLastMessageId(ctx, userID, dialogID1, dialogID2)
	}
	return 0, nil
}

func (d *Dao) SelectDialogLastMessageIDNotIDList(ctx context.Context, userID, dialogID1, dialogID2 int64, ids []int32) (int32, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.SelectDialogLastMessageIdNotIdList(ctx, userID, dialogID1, dialogID2, ids)
	}
	return 0, nil
}

func (d *Dao) CountUnreadInDialog(ctx context.Context, userID, dialogID1, dialogID2, senderUserID int64, minID, maxID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.CountUnreadInDialog(ctx, userID, dialogID1, dialogID2, senderUserID, minID, maxID)
	}
	return 0, nil
}

func (d *Dao) UpdateMessageEdit(ctx context.Context, messageData, message string, userID int64, messageID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateEditMessage(ctx, messageData, message, userID, messageID)
	}
	return d.MessagesDAO.UpdateEditMessage(ctx, messageData, message, userID, messageID)
}

func (d *Dao) UpdateMessageEditOn(ctx context.Context, tx postgres_dao.DB, messageData, message string, userID int64, messageID int32) (int64, error) {
	if d.Postgres != nil && d.Postgres.Store != nil && d.Postgres.Store.Messages != nil {
		return d.Postgres.Store.Messages.UpdateEditMessageOn(ctx, tx, messageData, message, userID, messageID)
	}
	return 0, nil
}
