package dao

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func lockUsernamePeer(ctx context.Context, tx pgx.Tx, actorID int64, peerType int32, peerID int64) error {
	if actorID <= 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	switch peerType {
	case mtproto.PEER_USER:
		if actorID != peerID {
			// Bot profile mutations lock the registry before the user row as well.
			var creatorID int64
			if err := tx.QueryRow(ctx, `SELECT creator_user_id FROM bots WHERE bot_id=$1 FOR UPDATE`, peerID).Scan(&creatorID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return mtproto.ErrForbiddenUserBotInvalid
				}
				return err
			}
			if creatorID != actorID {
				return mtproto.ErrForbiddenUserBotInvalid
			}
		}
		var userType int32
		if err := tx.QueryRow(ctx, `SELECT user_type FROM users WHERE id=$1 AND deleted=FALSE AND user_type NOT IN(0,1) FOR UPDATE`, peerID).Scan(&userType); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrPeerIdInvalid
			}
			return err
		}
		if actorID != peerID && userType != user.UserTypeBot {
			return mtproto.ErrForbiddenUserBotInvalid
		}
		return nil
	case mtproto.PEER_CHANNEL:
		var creatorID int64
		if err := tx.QueryRow(ctx, `SELECT creator_user_id FROM apifull_channel WHERE id=$1 FOR UPDATE`, peerID).Scan(&creatorID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrChannelInvalid
			}
			return err
		}
		if creatorID == actorID {
			return nil
		}
		var rightsJSON, banJSON string
		if err := tx.QueryRow(ctx, `SELECT admin_rights,banned_rights FROM apifull_channel_member WHERE channel_id=$1 AND user_id=$2 FOR UPDATE`, peerID, actorID).Scan(&rightsJSON, &banJSON); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrChatAdminRequired
			}
			return err
		}
		var rights *struct {
			ChangeInfo bool `json:"change_info"`
		}
		if rightsJSON != "" {
			if err := jsonx.UnmarshalFromString(rightsJSON, &rights); err != nil || rights == nil {
				return mtproto.ErrInternalServerError
			}
		}
		var ban *struct {
			ViewMessages bool `json:"view_messages"`
		}
		if banJSON != "" {
			if err := jsonx.UnmarshalFromString(banJSON, &ban); err != nil || ban == nil {
				return mtproto.ErrInternalServerError
			}
		}
		if rights == nil || !rights.ChangeInfo || (ban != nil && ban.ViewMessages) {
			return mtproto.ErrChatAdminRequired
		}
		return nil
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

func (d *Dao) TogglePeerUsername(ctx context.Context, actorID int64, peerType int32, peerID int64, name string, active bool) error {
	if name == "" {
		return mtproto.ErrUsernameInvalid
	}
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if err := lockUsernamePeer(ctx, tx, actorID, peerType, peerID); err != nil {
			return err
		}
		entries, err := d.Postgres.Store.Username.SelectAllByPeerForUpdate(ctx, tx, peerType, peerID)
		if err != nil {
			return err
		}
		var target *dataobject.UsernameDO
		for i := range entries {
			if strings.EqualFold(entries[i].Username, name) && !entries[i].Editable {
				target = &entries[i]
				break
			}
		}
		if target == nil {
			return mtproto.ErrUsernameInvalid
		}
		if target.Active == active {
			return mtproto.ErrUsernameNotModified
		}
		target.Active = active
		ordered := make([]dataobject.UsernameDO, 0, len(entries))
		for _, entry := range entries {
			if entry.Active && entry.Id != target.Id {
				ordered = append(ordered, entry)
			}
		}
		ordered = append(ordered, *target)
		for _, entry := range entries {
			if !entry.Active && entry.Id != target.Id {
				ordered = append(ordered, entry)
			}
		}
		return writeUsernameOrder(ctx, tx, ordered)
	})
}

func (d *Dao) ReorderPeerUsernames(ctx context.Context, actorID int64, peerType int32, peerID int64, names []string) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if err := lockUsernamePeer(ctx, tx, actorID, peerType, peerID); err != nil {
			return err
		}
		entries, err := d.Postgres.Store.Username.SelectAllByPeerForUpdate(ctx, tx, peerType, peerID)
		if err != nil {
			return err
		}
		active := make(map[string]dataobject.UsernameDO)
		for _, entry := range entries {
			if entry.Active {
				active[strings.ToLower(entry.Username)] = entry
			}
		}
		if len(active) != len(names) {
			return mtproto.ErrOrderInvalid
		}
		ordered := make([]dataobject.UsernameDO, 0, len(entries))
		modified := false
		for i, name := range names {
			entry, ok := active[strings.ToLower(name)]
			if !ok {
				return mtproto.ErrOrderInvalid
			}
			delete(active, strings.ToLower(name))
			ordered = append(ordered, entry)
			modified = modified || entry.Id != entries[i].Id
		}
		if !modified {
			return mtproto.ErrUsernameNotModified
		}
		for _, entry := range entries {
			if !entry.Active {
				ordered = append(ordered, entry)
			}
		}
		return writeUsernameOrder(ctx, tx, ordered)
	})
}

func (d *Dao) DeactivateChannelUsernames(ctx context.Context, actorID, channelID int64) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if err := lockUsernamePeer(ctx, tx, actorID, mtproto.PEER_CHANNEL, channelID); err != nil {
			return err
		}
		_, err := d.Postgres.Store.Username.DeactivateAllChannelUsernamesTx(ctx, tx, channelID)
		return err
	})
}

func writeUsernameOrder(ctx context.Context, tx pgx.Tx, entries []dataobject.UsernameDO) error {
	for i, entry := range entries {
		if _, err := tx.Exec(ctx, `UPDATE username SET order2=$1,active=$2 WHERE id=$3 AND peer_type=$4 AND peer_id=$5 AND deleted=FALSE`,
			int64(i), entry.Active, entry.Id, entry.PeerType, entry.PeerId); err != nil {
			return err
		}
	}
	return nil
}
