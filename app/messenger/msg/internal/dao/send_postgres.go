package dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"google.golang.org/protobuf/proto"
)

// SendMessagesWithDeliveries commits the sender's entire batch and recipient
// snapshot together. A worker can resume delivery after a process restart.
func (d *Dao) SendMessagesWithDeliveries(ctx context.Context, senderID int64, peer *mtproto.PeerUtil, messages []*msg.OutboxMessage, recipients []*inbox.TLInboxSendUserMessageToInboxV2) ([]*mtproto.MessageBox, error) {
	if senderID <= 0 || peer == nil || peer.PeerId <= 0 || len(messages) == 0 || len(recipients) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	for _, recipient := range recipients {
		if recipient == nil || recipient.FromId != senderID || recipient.UserId <= 0 || recipient.PeerType != peer.PeerType || recipient.PeerId != peer.PeerId || recipient.Out != (recipient.UserId == senderID) {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	boxes := make([]*mtproto.MessageBox, 0, len(messages))
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('messenger-user:' || $1::bigint::text, 0))`, senderID); err != nil {
			return err
		}
		var err error
		switch peer.PeerType {
		case mtproto.PEER_USER:
			recipients, err = authorizeUserDeliveryOn(ctx, tx, senderID, peer.PeerId, recipients)
		case mtproto.PEER_CHAT:
			recipients, err = authorizeChatDeliveryOn(ctx, tx, senderID, peer.PeerId, messages, recipients)
		default:
			return mtproto.ErrPeerIdInvalid
		}
		if err != nil {
			return err
		}
		for _, message := range messages {
			box, inserted, err := d.sendMessageToOutboxPostgresOn(ctx, tx, senderID, peer, message)
			if err != nil {
				return err
			}
			boxes = append(boxes, box)
			if !inserted {
				continue
			}
			for _, recipient := range recipients {
				request := proto.Clone(recipient).(*inbox.TLInboxSendUserMessageToInboxV2)
				request.BoxList = []*mtproto.MessageBox{box}
				if err := d.EnqueueInboxDeliveryOn(ctx, tx, request); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return boxes, nil
}

func authorizeUserDeliveryOn(ctx context.Context, tx pgx.Tx, senderID, peerID int64, recipients []*inbox.TLInboxSendUserMessageToInboxV2) ([]*inbox.TLInboxSendUserMessageToInboxV2, error) {
	rows, err := tx.Query(ctx, `SELECT id, deleted FROM users WHERE id = ANY($1::bigint[]) ORDER BY id FOR SHARE`, []int64{senderID, peerID})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make(map[int64]bool, 2)
	for rows.Next() {
		var id int64
		var deleted bool
		if err := rows.Scan(&id, &deleted); err != nil {
			return nil, err
		}
		users[id] = !deleted
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !users[senderID] || !users[peerID] {
		return nil, mtproto.ErrInputUserDeactivated
	}
	var blocked bool
	if senderID != peerID {
		var deleted bool
		err := tx.QueryRow(ctx, `SELECT deleted FROM user_peer_blocks WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 FOR SHARE`, peerID, mtproto.PEER_USER, senderID).Scan(&deleted)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		blocked = err == nil && !deleted
	}
	seen := make(map[int64]bool, 2)
	authorized := make([]*inbox.TLInboxSendUserMessageToInboxV2, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient.UserId != senderID && recipient.UserId != peerID || seen[recipient.UserId] {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seen[recipient.UserId] = true
		if recipient.UserId == senderID || !blocked {
			authorized = append(authorized, recipient)
		}
	}
	if !seen[senderID] || (!blocked && !seen[peerID]) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return authorized, nil
}

func authorizeChatDeliveryOn(ctx context.Context, tx pgx.Tx, senderID, chatID int64, messages []*msg.OutboxMessage, recipients []*inbox.TLInboxSendUserMessageToInboxV2) ([]*inbox.TLInboxSendUserMessageToInboxV2, error) {
	var deactivated bool
	var migratedID int64
	if err := tx.QueryRow(ctx, `SELECT deactivated, migrated_to_id FROM chats WHERE id = $1 FOR SHARE`, chatID).Scan(&deactivated, &migratedID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, mtproto.ErrChatIdInvalid
		}
		return nil, err
	}
	if deactivated || migratedID != 0 {
		return nil, mtproto.ErrChatWriteForbidden
	}
	rows, err := tx.Query(ctx, `SELECT user_id, state FROM chat_participants WHERE chat_id = $1 ORDER BY user_id FOR SHARE`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make(map[int64]int32)
	for rows.Next() {
		var userID int64
		var state int32
		if err := rows.Scan(&userID, &state); err != nil {
			return nil, err
		}
		states[userID] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	senderState, member := states[senderID]
	if !member {
		return nil, mtproto.ErrChatWriteForbidden
	}
	if senderState != mtproto.ChatMemberStateNormal {
		if senderState != mtproto.ChatMemberStateLeft {
			return nil, mtproto.ErrChatWriteForbidden
		}
		// The Chats service sends the user's leave notice after committing the leave.
		for _, message := range messages {
			if message.GetMessage().GetPredicateName() != mtproto.Predicate_messageService || message.GetMessage().GetAction().GetPredicateName() != mtproto.Predicate_messageActionChatDeleteUser || message.GetMessage().GetAction().GetUserId() != senderID {
				return nil, mtproto.ErrChatWriteForbidden
			}
		}
	}
	seen := make(map[int64]bool)
	authorized := make([]*inbox.TLInboxSendUserMessageToInboxV2, 0, len(recipients))
	for _, recipient := range recipients {
		if state, ok := states[recipient.UserId]; !ok || (state != mtproto.ChatMemberStateNormal && recipient.UserId != senderID) {
			continue
		}
		if seen[recipient.UserId] {
			return nil, mtproto.ErrInputRequestInvalid
		}
		seen[recipient.UserId] = true
		authorized = append(authorized, recipient)
	}
	for userID, state := range states {
		if (state == mtproto.ChatMemberStateNormal || userID == senderID) && !seen[userID] {
			// A new member joined after the RPC snapshot; retry with current entities.
			return nil, mtproto.ErrInternalServerError
		}
	}
	return authorized, nil
}
