package dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func (d *Dao) DeleteHistoryState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, maxID int32, justClear, revoke bool) ([]*mtproto.Update, int32, error) {
	if !validStatePeer(peer) || maxID < 0 {
		return nil, 0, mtproto.ErrPeerIdInvalid
	}
	return d.MutateMessageStateOnce(ctx, userID, "delete-history", func(tx pgx.Tx) ([]*mtproto.Update, error) {
		if revoke && peer.PeerType == mtproto.PEER_CHAT {
			if err := authorizeChatStateOn(ctx, tx, userID, peer.PeerId, false); err != nil {
				return nil, err
			}
			var admin bool
			if err := tx.QueryRow(ctx, `SELECT creator_user_id=$2 OR EXISTS(SELECT 1 FROM chat_participants WHERE chat_id=$1 AND user_id=$2 AND participant_type=$3 AND (admin_rights & $4)<>0) FROM chats WHERE id=$1`, peer.PeerId, userID, mtproto.ChatMemberAdmin, mtproto.ADMIN_DELETE_MESSAGES).Scan(&admin); err != nil {
				return nil, err
			}
			if !admin {
				return nil, mtproto.ErrChatAdminRequired
			}
		}
		rows, err := tx.Query(ctx, `SELECT user_message_box_id FROM messages WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND NOT deleted AND ($4::integer=0 OR user_message_box_id<=$4) ORDER BY user_message_box_id DESC`, userID, peer.PeerType, peer.PeerId, maxID)
		if err != nil {
			return nil, err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[int32])
		if err != nil || len(ids) == 0 {
			return nil, err
		}
		messages, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageIdList(ctx, userID, ids)
		if err != nil {
			return nil, err
		}
		top, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageId(ctx, userID, ids[0])
		if err != nil || top == nil {
			return nil, err
		}
		last := new(mtproto.Message)
		if err := jsonx.UnmarshalFromString(top.MessageData, last); err != nil {
			return nil, err
		}
		if justClear && len(ids) == 1 && last.GetAction().GetPredicateName() == mtproto.Predicate_messageActionHistoryClear {
			return nil, nil
		}
		deletedIDs := ids
		if justClear {
			deletedIDs = ids[1:]
		}
		if _, err := d.Postgres.Store.Messages.DeleteMessagesByMessageIdListOn(ctx, tx, userID, deletedIDs); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE hash_tags SET deleted=TRUE WHERE user_id=$1 AND hash_tag_message_id=ANY($2::integer[])`, userID, ids); err != nil {
			return nil, err
		}
		var updates []*mtproto.Update
		if len(deletedIDs) > 0 {
			updates = append(updates, mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{Messages: deletedIDs, PtsCount: int32(len(deletedIDs))}).To_Update())
		}
		if justClear {
			marker := mtproto.MakeTLMessageService(&mtproto.Message{Id: top.UserMessageBoxId, Out: last.Out, FromId: last.FromId, PeerId: peer.ToPeer(), Date: last.Date, Action: mtproto.MakeTLMessageActionHistoryClear(nil).To_MessageAction()}).To_Message()
			data, err := jsonx.Marshal(marker)
			if err != nil {
				return nil, err
			}
			if _, err := d.UpdateMessageEditOn(ctx, tx, string(data), "", userID, top.UserMessageBoxId); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `UPDATE messages SET media_unread=FALSE,mentioned=FALSE,reaction_unread=FALSE,pinned=FALSE,message_filter_type=0 WHERE user_id=$1 AND user_message_box_id=$2`, userID, top.UserMessageBoxId); err != nil {
				return nil, err
			}
			updates = append(updates, mtproto.MakeTLUpdateEditMessage(&mtproto.Update{Message_MESSAGE: marker, PtsCount: 1}).To_Update())
		}
		if _, err := tx.Exec(ctx, `UPDATE dialogs SET read_inbox_max_id=GREATEST(read_inbox_max_id,$4),read_outbox_max_id=GREATEST(read_outbox_max_id,$4),unread_mark=FALSE WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, userID, peer.PeerType, peer.PeerId, ids[0]); err != nil {
			return nil, err
		}
		if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
			return nil, err
		}
		updates = append(updates, mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{Peer_PEER: peer.ToPeer(), MaxId: ids[0], PtsCount: 1}).To_Update())
		if revoke && !peer.IsSelfUser(userID) {
			dataIDs := make([]int64, 0, len(messages))
			for _, message := range messages {
				dataIDs = append(dataIDs, message.DialogMessageId)
			}
			if err := d.EnqueueMessageStateDeliveryOn(ctx, tx, userID, &inbox.TLInboxDeleteMessagesToInbox{FromId: userID, PeerType: peer.PeerType, PeerId: peer.PeerId, Id: dataIDs}); err != nil {
				return nil, err
			}
		}
		return updates, nil
	})
}
