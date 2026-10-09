package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/jsonx"
)

type privacyUserContext struct {
	contact, closeFriend, bot, premium bool
}

func (d *Dao) CheckUserPrivacy(ctx context.Context, ownerID int64, key int32, peerID int64) (bool, error) {
	if genUserPrivacyKeyPrefix(ownerID, key) == "" {
		return false, mtproto.ErrPrivacyKeyInvalid
	}
	if ownerID <= 0 || peerID <= 0 {
		return false, mtproto.ErrUserIdInvalid
	}
	if d.Postgres == nil || d.Postgres.Pool == nil {
		return false, errors.New("biz/user: postgres store is not configured")
	}
	// User, contact, privacy, and group membership reads must share a snapshot.
	tx, err := d.Postgres.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var rulesJSON *string
	var peerExists bool
	var peer privacyUserContext
	err = tx.QueryRow(ctx, `SELECT p.rules, peer.id IS NOT NULL,
		c.id IS NOT NULL, COALESCE(c.close_friend,FALSE),
		COALESCE(peer.is_bot OR peer.user_type=$6,FALSE),
		COALESCE(peer.premium AND (peer.premium_expire_date=0 OR
			peer.premium_expire_date>EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)),FALSE)
		FROM users owner
		LEFT JOIN users peer ON peer.id=$3 AND peer.deleted=FALSE AND peer.user_type NOT IN($4,$5)
		LEFT JOIN user_privacies p ON p.user_id=owner.id AND p.key_type=$2
		LEFT JOIN user_contacts c ON c.owner_user_id=owner.id AND c.contact_user_id=$3 AND c.is_deleted=FALSE
		WHERE owner.id=$1 AND owner.deleted=FALSE AND owner.user_type NOT IN($4,$5)`,
		ownerID, key, peerID, user.UserTypeUnknown, user.UserTypeDeleted, user.UserTypeBot).
		Scan(&rulesJSON, &peerExists, &peer.contact, &peer.closeFriend, &peer.bot, &peer.premium)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !peerExists) {
		return false, mtproto.ErrUserIdInvalid
	}
	if err != nil {
		return false, err
	}
	allowed := ownerID == peerID
	if !allowed {
		var rules []*mtproto.PrivacyRule
		if rulesJSON != nil {
			if err := jsonx.UnmarshalFromString(*rulesJSON, &rules); err != nil {
				return false, fmt.Errorf("decode user privacy rules: %w", err)
			}
			if rules == nil {
				return false, mtproto.ErrPrivacyValueInvalid
			}
		} else {
			rules = makeDefaultPrivacyRules(key)
		}
		allowed, err = privacyRulesAllow(rules, peerID, peer, func(ids []int64) (bool, error) {
			return privacyChatParticipant(ctx, tx, peerID, ids)
		})
		if err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return allowed, nil
}

func privacyRulesAllow(rules []*mtproto.PrivacyRule, peerID int64, peer privacyUserContext, checkChats func([]int64) (bool, error)) (bool, error) {
	return mtproto.CheckPrivacyRules(rules, peerID, mtproto.PrivacyRuleContext{
		Contact: peer.contact, CloseFriend: peer.closeFriend, ContactKnown: true,
		Bot: peer.bot, Premium: peer.premium, UserKnown: true, ChatParticipant: checkChats,
	})
}

func privacyChatParticipant(ctx context.Context, tx pgx.Tx, peerID int64, ids []int64) (bool, error) {
	members, err := privacyChatMemberships(ctx, tx, []int64{peerID}, ids)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if members[[2]int64{peerID, id}] {
			return true, nil
		}
	}
	return false, nil
}

func privacyChatMemberships(ctx context.Context, tx pgx.Tx, peerIDs, ids []int64) (map[[2]int64]bool, error) {
	members := make(map[[2]int64]bool)
	if len(ids) == 0 || len(peerIDs) == 0 {
		return members, nil
	}
	// Channel membership is authoritative when a basic chat has the same ID.
	rows, err := tx.Query(ctx, `SELECT p.user_id, p.chat_id, ''::text, FALSE FROM chat_participants p
		JOIN chats c ON c.id=p.chat_id
		WHERE p.user_id=ANY($1::bigint[]) AND p.chat_id=ANY($2::bigint[]) AND p.state=$3 AND c.deactivated=FALSE
			AND NOT EXISTS(SELECT 1 FROM apifull_channel channel WHERE channel.id=c.id)
		UNION ALL
		SELECT m.user_id, c.id, COALESCE(m.banned_rights,''), c.creator_user_id=m.user_id FROM apifull_channel c
		JOIN apifull_channel_member m ON m.channel_id=c.id
		WHERE c.id=ANY($2::bigint[]) AND m.user_id=ANY($1::bigint[])
		UNION ALL
		SELECT c.creator_user_id, c.id, ''::text, TRUE FROM apifull_channel c
		WHERE c.id=ANY($2::bigint[]) AND c.creator_user_id=ANY($1::bigint[])`,
		peerIDs, ids, mtproto.ChatMemberStateNormal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var peerID, chatID int64
		var rawBan string
		var creator bool
		if err := rows.Scan(&peerID, &chatID, &rawBan, &creator); err != nil {
			return nil, err
		}
		var ban *struct {
			ViewMessages bool `json:"view_messages"`
		}
		if rawBan != "" {
			if err := jsonx.UnmarshalFromString(rawBan, &ban); err != nil {
				return nil, fmt.Errorf("decode channel privacy membership: %w", err)
			}
			if ban == nil {
				return nil, errors.New("decode channel privacy membership: null ban record")
			}
		}
		// APIFull keeps kicked users as ban rows; ban expiry does not rejoin them.
		key := [2]int64{peerID, chatID}
		members[key] = members[key] || creator || ban == nil || !ban.ViewMessages
	}
	return members, rows.Err()
}
