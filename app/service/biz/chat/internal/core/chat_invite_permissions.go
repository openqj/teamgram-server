package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

func (c *ChatCore) requireInviteCaller() (int64, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return 0, mtproto.ErrUserIdInvalid
	}
	return c.MD.UserId, nil
}

func (c *ChatCore) requireInviteSelf(selfID int64) (int64, error) {
	callerID, err := c.requireInviteCaller()
	if err != nil {
		return 0, err
	}
	if selfID != callerID {
		return 0, mtproto.ErrUserIdInvalid
	}
	return callerID, nil
}

func (c *ChatCore) requireInvitePermission(chatID, selfID, targetAdminID int64) (*mtproto.MutableChat, error) {
	// APIFull channels share the invite tables with basic chats, but their
	// membership and administrator rights live in the APIFull tables. Resolve
	// that model first so a channel id cannot accidentally be treated as a chat
	// (and so a channel with the same numeric id as a chat remains authoritative
	// for channel RPC callers).
	if handled, err := c.requireChannelInvitePermission(chatID, selfID, targetAdminID); handled {
		return nil, err
	}

	chat2, err := c.svcCtx.Dao.GetMutableChat(c.ctx, chatID)
	if err != nil {
		return nil, mtproto.ErrChatIdInvalid
	}
	me, _ := chat2.GetImmutableChatParticipant(selfID)
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		return nil, mtproto.ErrUserNotParticipant
	}
	if !me.CanInviteUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if targetAdminID != 0 && targetAdminID != selfID && !me.CanAdminAddAdmins() {
		return nil, mtproto.ErrChatAdminRequired
	}
	return chat2, nil
}

// requireChannelInvitePermission checks the APIFull channel membership model.
// The bool reports whether chatID is an APIFull channel; false means callers
// should continue with the basic-chat permission path. This keeps older
// deployments, where the APIFull tables do not exist yet, compatible.
func (c *ChatCore) requireChannelInvitePermission(chatID, selfID, targetAdminID int64) (bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return false, nil
	}

	var channel struct {
		CreatorID int64 `db:"creator_user_id"`
	}
	var err error
	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Pool != nil {
		err = c.svcCtx.Dao.Postgres.Pool.QueryRow(c.ctx,
			"SELECT creator_user_id FROM apifull_channel WHERE id = $1", chatID).Scan(&channel.CreatorID)
	} else if c.svcCtx.Dao.DB != nil {
		err = c.svcCtx.Dao.DB.QueryRowPartial(c.ctx, &channel,
			"SELECT creator_user_id FROM apifull_channel WHERE id = ?", chatID)
	} else {
		return false, nil
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) || apifullTableMissing(err) {
			return false, nil
		}
		return true, mtproto.ErrInternalServerError
	}

	if selfID <= 0 {
		return true, mtproto.ErrUserIdInvalid
	}
	if selfID == channel.CreatorID {
		return true, nil
	}

	var member struct {
		AdminRights  string `db:"admin_rights"`
		BannedRights string `db:"banned_rights"`
	}
	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Pool != nil {
		err = c.svcCtx.Dao.Postgres.Pool.QueryRow(c.ctx,
			"SELECT admin_rights, banned_rights FROM apifull_channel_member WHERE channel_id = $1 AND user_id = $2",
			chatID, selfID).Scan(&member.AdminRights, &member.BannedRights)
	} else {
		err = c.svcCtx.Dao.DB.QueryRowPartial(c.ctx, &member,
			"SELECT admin_rights, banned_rights FROM apifull_channel_member WHERE channel_id = ? AND user_id = ?",
			chatID, selfID)
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
		return true, mtproto.ErrUserNotParticipant
	}
	if err != nil {
		return true, mtproto.ErrInternalServerError
	}

	var banned channelInviteBannedRights
	if member.BannedRights != "" {
		if err = json.Unmarshal([]byte(member.BannedRights), &banned); err != nil {
			return true, mtproto.ErrInternalServerError
		}
		if banned.Active(time.Now().Unix()) {
			if banned.ViewMessages {
				return true, mtproto.ErrUserNotParticipant
			}
			if banned.InviteUsers {
				return true, mtproto.ErrChatAdminRequired
			}
		}
	}

	var rights channelInviteAdminRights
	if member.AdminRights != "" {
		if err = json.Unmarshal([]byte(member.AdminRights), &rights); err != nil {
			return true, mtproto.ErrInternalServerError
		}
	}
	if !rights.InviteUsers {
		return true, mtproto.ErrChatAdminRequired
	}
	if targetAdminID != 0 && targetAdminID != selfID && !rights.AddAdmins {
		return true, mtproto.ErrChatAdminRequired
	}
	return true, nil
}

type channelInviteAdminRights struct {
	InviteUsers bool `json:"invite_users"`
	AddAdmins   bool `json:"add_admins"`
}

type channelInviteBannedRights struct {
	ViewMessages bool  `json:"view_messages"`
	InviteUsers  bool  `json:"invite_users"`
	UntilDate    int64 `json:"until_date"`
}

func (r channelInviteBannedRights) Active(now int64) bool {
	if !r.ViewMessages && !r.InviteUsers {
		return false
	}
	return r.UntilDate == 0 || r.UntilDate >= now
}

func apifullTableMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "doesn't exist") ||
		strings.Contains(message, "no such table") ||
		strings.Contains(message, "unknown table")
}

func (c *ChatCore) isAPIFullChannel(chatID int64) bool {
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Pool != nil {
		var marker int
		err := c.svcCtx.Dao.Postgres.Pool.QueryRow(c.ctx,
			"SELECT 1 FROM apifull_channel WHERE id = $1", chatID).Scan(&marker)
		return err == nil
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DB == nil {
		return false
	}
	var marker int
	err := c.svcCtx.Dao.DB.QueryRowPartial(c.ctx, &marker,
		"SELECT 1 FROM apifull_channel WHERE id = ?", chatID)
	return err == nil
}

func (c *ChatCore) requireInviteLinkPermission(chatID, selfID int64, link string) (*mtproto.MutableChat, error) {
	var invite *dataobject.ChatInvitesDO
	var err error
	if c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil {
		invite, err = c.svcCtx.Dao.Postgres.Store.Invites.SelectByLink(c.ctx, chat.GetInviteHashByLink(link))
	} else {
		invite, err = c.svcCtx.Dao.ChatInvitesDAO.SelectByLink(c.ctx, chat.GetInviteHashByLink(link))
	}
	if err != nil {
		return nil, err
	}
	if invite == nil || invite.ChatId != chatID {
		return nil, mtproto.ErrInviteHashInvalid
	}
	return c.requireInvitePermission(chatID, selfID, invite.AdminId)
}
