package dao

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/mqconsumer"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
)

// The receipt shares the mutation transaction, so an offset retry cannot
// overwrite a newer message view or allocate a second PTS value.
func (d *Dao) MutateMessageStateOnce(ctx context.Context, userID int64, operation string, fn func(pgx.Tx) ([]*mtproto.Update, error)) ([]*mtproto.Update, int32, error) {
	return d.MutateMessageState(ctx, userID, func(tx pgx.Tx) ([]*mtproto.Update, error) {
		if metadata, ok := mqconsumer.MetadataFromContext(ctx); ok {
			tag, err := tx.Exec(ctx, `INSERT INTO msg_inbox_consumer_receipts(consumer_group,topic,partition,message_offset,user_id,operation) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, metadata.ConsumerGroup, metadata.Topic, metadata.Partition, metadata.Offset, userID, operation)
			if err != nil {
				return nil, err
			}
			if tag.RowsAffected() == 0 {
				return nil, errMessageStateAlreadyApplied
			}
		}
		return fn(tx)
	})
}

func (d *Dao) EditInboxMessageState(ctx context.Context, in *inbox.TLInboxEditMessageToInboxV2) error {
	if in == nil || in.NewMessage == nil || in.NewMessage.Message == nil || in.UserId <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	_, _, err := d.MutateMessageStateOnce(ctx, in.UserId, fmt.Sprintf("edit:%d", in.NewMessage.DialogMessageId), func(tx pgx.Tx) ([]*mtproto.Update, error) {
		if in.Out {
			// The sender's authoritative edit and sync intent were already committed.
			return nil, nil
		}
		row, err := postgres_dao.NewMessagesDAO(tx).SelectByMessageDataId(ctx, in.UserId, in.NewMessage.DialogMessageId)
		if err != nil || row == nil {
			return nil, err
		}
		peerID := in.PeerId
		if in.PeerType == mtproto.PEER_USER {
			peerID = in.FromId
		}
		if row.SenderUserId != in.FromId || row.PeerType != in.PeerType || row.PeerId != peerID {
			return nil, mtproto.ErrPeerIdInvalid
		}
		message := new(mtproto.Message)
		if err := jsonx.UnmarshalFromString(row.MessageData, message); err != nil {
			return nil, err
		}
		replacement := in.NewMessage.Message
		updated := proto.Clone(message).(*mtproto.Message)
		updated.Message, updated.Action, updated.Media = replacement.Message, replacement.Action, replacement.Media
		updated.ReplyMarkup, updated.Entities = replacement.ReplyMarkup, replacement.Entities
		updated.EditDate, updated.EditHide = replacement.EditDate, replacement.EditHide
		if proto.Equal(message, updated) || updated.GetEditDate().GetValue() < message.GetEditDate().GetValue() {
			return nil, nil
		}
		data, err := jsonx.Marshal(updated)
		if err != nil {
			return nil, err
		}
		if _, err := d.UpdateMessageEditOn(ctx, tx, string(data), updated.Message, in.UserId, row.UserMessageBoxId); err != nil {
			return nil, err
		}
		if _, err := d.Postgres.Store.HashTags.DeleteHashTagMessageIdOn(ctx, tx, in.UserId, row.UserMessageBoxId); err != nil {
			return nil, err
		}
		for _, entity := range updated.Entities {
			if entity.GetPredicateName() == mtproto.Predicate_messageEntityHashtag && entity.Url != "" {
				if _, _, err := d.Postgres.Store.HashTags.InsertOrUpdateOn(ctx, tx, &dataobject.HashTagsDO{UserId: in.UserId, PeerType: row.PeerType, PeerId: row.PeerId, HashTag: entity.Url, HashTagMessageId: row.UserMessageBoxId}); err != nil {
					return nil, err
				}
			}
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateEditMessage(&mtproto.Update{Message_MESSAGE: updated, PtsCount: 1}).To_Update()}, nil
	})
	return err
}

func (d *Dao) DeleteInboxMessageState(ctx context.Context, userID int64, ids []int64) error {
	_, _, err := d.MutateMessageStateOnce(ctx, userID, "delete", func(tx pgx.Tx) ([]*mtproto.Update, error) {
		rows, err := tx.Query(ctx, `UPDATE messages SET deleted=TRUE WHERE user_id=$1 AND dialog_message_id=ANY($2::bigint[]) AND NOT deleted RETURNING user_message_box_id,peer_type,peer_id`, userID, ids)
		if err != nil {
			return nil, err
		}
		type deletedMessage struct {
			ID, PeerType int32
			PeerID       int64
		}
		deleted, err := pgx.CollectRows(rows, pgx.RowToStructByPos[deletedMessage])
		if err != nil || len(deleted) == 0 {
			return nil, err
		}
		messageIDs := make([]int32, 0, len(deleted))
		for _, row := range deleted {
			messageIDs = append(messageIDs, row.ID)
			if err := d.refreshMessageDialogOn(ctx, tx, userID, row.PeerType, row.PeerID); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE hash_tags SET deleted=TRUE WHERE user_id=$1 AND hash_tag_message_id=ANY($2::integer[])`, userID, messageIDs); err != nil {
			return nil, err
		}
		return []*mtproto.Update{mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{Messages: messageIDs, PtsCount: int32(len(messageIDs))}).To_Update()}, nil
	})
	return err
}

func (d *Dao) PinInboxMessageState(ctx context.Context, userID int64, peer *mtproto.PeerUtil, dialogID int64, pinned bool) error {
	_, _, err := d.MutateMessageStateOnce(ctx, userID, fmt.Sprintf("pin:%d", dialogID), func(tx pgx.Tx) ([]*mtproto.Update, error) {
		var id int32
		err := tx.QueryRow(ctx, `UPDATE messages SET pinned=$5 WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND dialog_message_id=$4 AND pinned<>$5 AND NOT deleted RETURNING user_message_box_id`, userID, peer.PeerType, peer.PeerId, dialogID, pinned).Scan(&id)
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if err := d.refreshMessageDialogOn(ctx, tx, userID, peer.PeerType, peer.PeerId); err != nil {
			return nil, err
		}
		return []*mtproto.Update{mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Pinned: pinned, Peer_PEER: peer.ToPeer(), Messages: []int32{id}, PtsCount: 1}).To_Update()}, nil
	})
	return err
}
