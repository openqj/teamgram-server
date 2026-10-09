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

func (d *Dao) prepareUserPrivacy(ctx context.Context, snapshots []*mtproto.ImmutableUser, viewers map[int64][]int64) error {
	if d.Postgres == nil {
		return nil
	}
	if d.Postgres.Pool == nil {
		return errors.New("biz/user: postgres store is not configured")
	}
	var ownerIDs, userIDs []int64
	seen := make(map[int64]bool)
	for _, snapshot := range snapshots {
		if snapshot.Deleted() || snapshot.GetUser().GetUserType() == user.UserTypeUnknown || snapshot.GetUser().GetUserType() == user.UserTypeDeleted {
			continue
		}
		ownerIDs = append(ownerIDs, snapshot.Id())
		for _, id := range append([]int64{snapshot.Id()}, viewers[snapshot.Id()]...) {
			if id > 0 && !seen[id] {
				seen[id] = true
				userIDs = append(userIDs, id)
			}
		}
	}
	if len(ownerIDs) == 0 {
		return nil
	}
	tx, err := d.Postgres.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	identity := make(map[int64]privacyUserContext, len(userIDs))
	rows, err := tx.Query(ctx, `SELECT id, is_bot OR user_type=$2,
		premium AND (premium_expire_date=0 OR premium_expire_date>EXTRACT(EPOCH FROM CURRENT_TIMESTAMP))
		FROM users WHERE id=ANY($1::bigint[]) AND deleted=FALSE AND user_type NOT IN($3,$4)`,
		userIDs, user.UserTypeBot, user.UserTypeUnknown, user.UserTypeDeleted)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var peer privacyUserContext
		if err := rows.Scan(&id, &peer.bot, &peer.premium); err != nil {
			rows.Close()
			return err
		}
		identity[id] = peer
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, ownerID := range ownerIDs {
		if _, exists := identity[ownerID]; !exists {
			return mtproto.ErrUserIdInvalid
		}
	}
	rulesByOwner := make(map[int64]map[int32][]*mtproto.PrivacyRule, len(ownerIDs))
	rows, err = tx.Query(ctx, `SELECT user_id,key_type,rules FROM user_privacies
		WHERE user_id=ANY($1::bigint[]) AND key_type=ANY($2::integer[])`, ownerIDs, userPrivacyKeys)
	if err != nil {
		return err
	}
	var chatIDs []int64
	seenChats := make(map[int64]bool)
	for rows.Next() {
		var ownerID int64
		var key int32
		var raw string
		if err := rows.Scan(&ownerID, &key, &raw); err != nil {
			rows.Close()
			return err
		}
		var rules []*mtproto.PrivacyRule
		if err := jsonx.UnmarshalFromString(raw, &rules); err != nil {
			rows.Close()
			return fmt.Errorf("decode user privacy rules: %w", err)
		}
		if rules == nil {
			rows.Close()
			return mtproto.ErrPrivacyValueInvalid
		}
		if _, err := mtproto.NormalizePrivacyRules(rules); err != nil {
			rows.Close()
			return err
		}
		if rulesByOwner[ownerID] == nil {
			rulesByOwner[ownerID] = make(map[int32][]*mtproto.PrivacyRule)
		}
		rulesByOwner[ownerID][key] = rules
		for _, rule := range rules {
			for _, id := range rule.GetChats() {
				if !seenChats[id] {
					seenChats[id] = true
					chatIDs = append(chatIDs, id)
				}
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	contacts := make(map[[2]int64]privacyUserContext)
	rows, err = tx.Query(ctx, `SELECT owner_user_id,contact_user_id,close_friend FROM user_contacts
		WHERE owner_user_id=ANY($1::bigint[]) AND contact_user_id=ANY($2::bigint[]) AND is_deleted=FALSE`, ownerIDs, userIDs)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ownerID, peerID int64
		var closeFriend bool
		if err := rows.Scan(&ownerID, &peerID, &closeFriend); err != nil {
			rows.Close()
			return err
		}
		contacts[[2]int64{ownerID, peerID}] = privacyUserContext{contact: true, closeFriend: closeFriend}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	members, err := privacyChatMemberships(ctx, tx, userIDs, chatIDs)
	if err != nil {
		return err
	}
	prepared := make(map[int64][]*mtproto.PrivacyKeyRules, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		for _, key := range userPrivacyKeys {
			rules, stored := rulesByOwner[ownerID][key]
			if !stored {
				rules = makeDefaultPrivacyRules(key)
			}
			allowIDs, denyIDs := []int64{}, []int64{}
			seenViewers := make(map[int64]bool)
			for _, peerID := range viewers[ownerID] {
				if peerID <= 0 || peerID == ownerID || seenViewers[peerID] {
					continue
				}
				seenViewers[peerID] = true
				peer, peerKnown := identity[peerID]
				_, ownerKnown := identity[ownerID]
				allowed := false
				if ownerKnown && peerKnown {
					contact := contacts[[2]int64{ownerID, peerID}]
					peer.contact, peer.closeFriend = contact.contact, contact.closeFriend
					allowed, err = privacyRulesAllow(rules, peerID, peer, func(ids []int64) (bool, error) {
						for _, id := range ids {
							if members[[2]int64{peerID, id}] {
								return true, nil
							}
						}
						return false, nil
					})
					if err != nil {
						return err
					}
				}
				if allowed {
					allowIDs = append(allowIDs, peerID)
				} else {
					denyIDs = append(denyIDs, peerID)
				}
			}
			// Resolve contextual exceptions for these viewers using existing TL rules.
			projected := make([]*mtproto.PrivacyRule, 0, len(rules)+2)
			if len(allowIDs) > 0 {
				projected = append(projected, mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: allowIDs}).To_PrivacyRule())
			}
			if len(denyIDs) > 0 {
				projected = append(projected, mtproto.MakeTLPrivacyValueDisallowUsers(&mtproto.PrivacyRule{Users: denyIDs}).To_PrivacyRule())
			}
			projected = append(projected, rules...)
			prepared[ownerID] = append(prepared[ownerID], mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{Key: key, Rules: projected}).To_PrivacyKeyRules())
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if rules, ok := prepared[snapshot.Id()]; ok {
			snapshot.KeysPrivacyRules = rules
		}
	}
	return nil
}
