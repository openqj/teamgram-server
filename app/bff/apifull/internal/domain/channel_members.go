package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrChannelCreator           = errors.New("channel creator cannot leave")
	ErrNotChannelMember         = errors.New("user is not channel member")
	ErrChannelMemberExists      = errors.New("user is already channel member")
	ErrInvalidChannelMember     = errors.New("invalid channel member")
	ErrInvalidChannelAccessHash = errors.New("invalid channel access hash")
	ErrNoChannelMembers         = errors.New("no channel members")
)

// ChannelBannedRights is the persisted subset of Telegram's ChatBannedRights.
// Keeping this model independent from mtproto lets the member store remain
// usable by non-wire callers.
type ChannelBannedRights struct {
	ViewMessages    bool  `json:"view_messages,omitempty"`
	SendMessages    bool  `json:"send_messages,omitempty"`
	SendMedia       bool  `json:"send_media,omitempty"`
	SendStickers    bool  `json:"send_stickers,omitempty"`
	SendGifs        bool  `json:"send_gifs,omitempty"`
	SendGames       bool  `json:"send_games,omitempty"`
	SendInline      bool  `json:"send_inline,omitempty"`
	EmbedLinks      bool  `json:"embed_links,omitempty"`
	SendPolls       bool  `json:"send_polls,omitempty"`
	ChangeInfo      bool  `json:"change_info,omitempty"`
	InviteUsers     bool  `json:"invite_users,omitempty"`
	PinMessages     bool  `json:"pin_messages,omitempty"`
	ManageTopics    bool  `json:"manage_topics,omitempty"`
	SendPhotos      bool  `json:"send_photos,omitempty"`
	SendVideos      bool  `json:"send_videos,omitempty"`
	SendRoundvideos bool  `json:"send_roundvideos,omitempty"`
	SendAudios      bool  `json:"send_audios,omitempty"`
	SendVoices      bool  `json:"send_voices,omitempty"`
	SendDocs        bool  `json:"send_docs,omitempty"`
	SendPlain       bool  `json:"send_plain,omitempty"`
	EditRank        bool  `json:"edit_rank,omitempty"`
	SendReactions   bool  `json:"send_reactions,omitempty"`
	UntilDate       int32 `json:"until_date,omitempty"`
}

func (r *ChannelBannedRights) Empty() bool {
	return r == nil || (!r.ViewMessages && !r.SendMessages && !r.SendMedia &&
		!r.SendStickers && !r.SendGifs && !r.SendGames && !r.SendInline &&
		!r.EmbedLinks && !r.SendPolls && !r.ChangeInfo && !r.InviteUsers &&
		!r.PinMessages && !r.ManageTopics && !r.SendPhotos && !r.SendVideos &&
		!r.SendRoundvideos && !r.SendAudios && !r.SendVoices && !r.SendDocs &&
		!r.SendPlain && !r.EditRank && !r.SendReactions)
}

// Active reports whether at least one ban right is currently in force.
// Telegram treats until_date=0 as an indefinite restriction.
func (r *ChannelBannedRights) Active(now int64) bool {
	if r.Empty() {
		return false
	}
	return r.UntilDate == 0 || int64(r.UntilDate) >= now
}

func (r *ChannelBannedRights) Kicks(_ int64) bool {
	return r != nil && r.ViewMessages
}

// ChannelAdminRights is the persisted subset of Telegram's channel admin
// rights. It stays in the domain package so storage does not depend on the
// wire representation.
type ChannelAdminRights struct {
	ChangeInfo           bool `json:"change_info,omitempty"`
	PostMessages         bool `json:"post_messages,omitempty"`
	EditMessages         bool `json:"edit_messages,omitempty"`
	DeleteMessages       bool `json:"delete_messages,omitempty"`
	BanUsers             bool `json:"ban_users,omitempty"`
	InviteUsers          bool `json:"invite_users,omitempty"`
	PinMessages          bool `json:"pin_messages,omitempty"`
	AddAdmins            bool `json:"add_admins,omitempty"`
	Anonymous            bool `json:"anonymous,omitempty"`
	ManageCall           bool `json:"manage_call,omitempty"`
	Other                bool `json:"other,omitempty"`
	ManageTopics         bool `json:"manage_topics,omitempty"`
	PostStories          bool `json:"post_stories,omitempty"`
	EditStories          bool `json:"edit_stories,omitempty"`
	DeleteStories        bool `json:"delete_stories,omitempty"`
	ManageDirectMessages bool `json:"manage_direct_messages,omitempty"`
	ManageRanks          bool `json:"manage_ranks,omitempty"`
	ManageLinkedPeers    bool `json:"manage_linked_peers,omitempty"`
}

func (r *ChannelAdminRights) Empty() bool {
	return r == nil || (!r.ChangeInfo && !r.PostMessages && !r.EditMessages &&
		!r.DeleteMessages && !r.BanUsers && !r.InviteUsers && !r.PinMessages &&
		!r.AddAdmins && !r.Anonymous && !r.ManageCall && !r.Other &&
		!r.ManageTopics && !r.PostStories && !r.EditStories && !r.DeleteStories &&
		!r.ManageDirectMessages && !r.ManageRanks && !r.ManageLinkedPeers)
}

type ChannelMember struct {
	ChannelID    int64
	UserID       int64
	InvitedBy    int64
	JoinedAt     int64
	Creator      bool
	AdminRights  *ChannelAdminRights
	Rank         string
	BannedRights *ChannelBannedRights
	BannedBy     int64
	BannedAt     int64
}

func JoinChannel(channelID, userID int64) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if userID <= 0 {
		return ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if userID == creator {
		return ErrChannelMemberExists
	}
	var existing int
	err = tx.QueryRow(`SELECT 1 FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&existing)
	if err == nil {
		return ErrChannelMemberExists
	}
	if err != sql.ErrNoRows {
		return err
	}
	joinedAt := time.Now().Unix()
	if _, err = tx.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at) VALUES (?,?,0,?)`,
		channelID, userID, joinedAt); err != nil {
		return err
	}
	// Keep membership and its audit event in the same transaction. The event
	// snapshot is the only durable representation after a later leave/kick.
	if err = SaveChannelAdminLogTx(tx, channelID, userID, userID, ChannelAdminLogParticipantJoin,
		nil, &ChannelMember{ChannelID: channelID, UserID: userID, JoinedAt: joinedAt}, joinedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func LeaveChannel(channelID, userID int64) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if userID <= 0 {
		return ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if userID == creator {
		return ErrChannelCreator
	}
	previous, found, err := loadChannelMemberTx(tx, channelID, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotChannelMember
	}
	result, err := tx.Exec(`DELETE FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrNotChannelMember
	}
	if err = SaveChannelAdminLogTx(tx, channelID, userID, userID, ChannelAdminLogParticipantLeave,
		&previous, nil, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// EditChannelAdmin changes one member's admin rights atomically. The creator
// and a persisted administrator with add-admins permission may make the
// change; the creator row itself is immutable.
func EditChannelAdmin(channelID, actorID, userID int64, rights *ChannelAdminRights, rank string) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if actorID <= 0 || userID <= 0 {
		return ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if actorID != creator {
		var actorRights string
		err = tx.QueryRow(`SELECT admin_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, actorID).Scan(&actorRights)
		if err == sql.ErrNoRows {
			return ErrNotCreator
		}
		if err != nil {
			return err
		}
		var stored ChannelAdminRights
		if actorRights == "" || json.Unmarshal([]byte(actorRights), &stored) != nil || !stored.AddAdmins {
			return ErrNotCreator
		}
	}
	if userID == creator {
		return ErrChannelCreator
	}
	previous, found, err := loadChannelMemberTx(tx, channelID, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotChannelMember
	}
	raw := ""
	if !rights.Empty() {
		encoded, marshalErr := json.Marshal(rights)
		if marshalErr != nil {
			return marshalErr
		}
		raw = string(encoded)
	} else {
		rank = ""
	}
	if _, err = tx.Exec(`UPDATE apifull_channel_member SET admin_rights=?, admin_rank=? WHERE channel_id=? AND user_id=?`, raw, rank, channelID, userID); err != nil {
		return err
	}
	next := previous
	if rights == nil || rights.Empty() {
		next.AdminRights = nil
		next.Rank = ""
	} else {
		next.AdminRights = rights
		next.Rank = rank
	}
	if err = SaveChannelAdminLogTx(tx, channelID, actorID, userID, ChannelAdminLogParticipantToggleAdmin,
		&previous, &next, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// EditChannelBanned applies or clears one member's ban atomically. The
// creator and administrators with ban_users permission may change bans.
// A view_messages ban is a kick, so clearing it removes the member row.
func EditChannelBanned(channelID, actorID, userID int64, rights *ChannelBannedRights, bannedAt int64) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if actorID <= 0 || userID <= 0 {
		return ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var creator int64
	err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator)
	if err == sql.ErrNoRows {
		return ErrChannelMissing
	}
	if err != nil {
		return err
	}
	if actorID != creator {
		var actorRaw string
		err = tx.QueryRow(`SELECT admin_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, actorID).Scan(&actorRaw)
		if err == sql.ErrNoRows {
			return ErrNotCreator
		}
		if err != nil {
			return err
		}
		var actorRights ChannelAdminRights
		if actorRaw == "" || json.Unmarshal([]byte(actorRaw), &actorRights) != nil || !actorRights.BanUsers {
			return ErrNotCreator
		}
	}
	if userID == creator {
		return ErrChannelCreator
	}

	var previousRaw, targetAdminRaw string
	err = tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, userID).
		Scan(&targetAdminRaw, &previousRaw)
	existing := err == nil
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var previous *ChannelMember
	if existing {
		loaded, loadedOK, loadErr := loadChannelMemberTx(tx, channelID, userID)
		if loadErr != nil {
			return loadErr
		}
		if !loadedOK {
			return ErrNotChannelMember
		}
		previous = &loaded
	}
	if actorID != creator && targetAdminRaw != "" {
		var targetRights ChannelAdminRights
		if err = json.Unmarshal([]byte(targetAdminRaw), &targetRights); err != nil {
			return err
		}
		if !targetRights.Empty() {
			return ErrNotCreator
		}
	}
	if rights == nil || rights.Empty() {
		if !existing {
			return ErrNotChannelMember
		}
		var previousRights ChannelBannedRights
		if previousRaw != "" {
			if err = json.Unmarshal([]byte(previousRaw), &previousRights); err != nil {
				return err
			}
		}
		if previousRights.Kicks(time.Now().Unix()) {
			if _, err = tx.Exec(`DELETE FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID); err != nil {
				return err
			}
			if err = SaveChannelAdminLogTx(tx, channelID, actorID, userID, ChannelAdminLogParticipantToggleBan,
				previous, nil, time.Now().Unix()); err != nil {
				return err
			}
			return tx.Commit()
		}
		if _, err = tx.Exec(`UPDATE apifull_channel_member SET banned_rights='', banned_by_user_id=0, banned_at=0 WHERE channel_id=? AND user_id=?`, channelID, userID); err != nil {
			return err
		}
		next := *previous
		next.BannedRights = nil
		next.BannedBy = 0
		next.BannedAt = 0
		if err = SaveChannelAdminLogTx(tx, channelID, actorID, userID, ChannelAdminLogParticipantToggleBan,
			previous, &next, time.Now().Unix()); err != nil {
			return err
		}
		return tx.Commit()
	}
	if !existing && !rights.ViewMessages {
		return ErrNotChannelMember
	}
	encoded, err := json.Marshal(rights)
	if err != nil {
		return err
	}
	if bannedAt <= 0 {
		bannedAt = time.Now().Unix()
	}
	if existing {
		_, err = tx.Exec(`UPDATE apifull_channel_member SET admin_rights='', admin_rank='', banned_rights=?, banned_by_user_id=?, banned_at=? WHERE channel_id=? AND user_id=?`,
			string(encoded), actorID, bannedAt, channelID, userID)
	} else {
		_, err = tx.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at, admin_rights, admin_rank, banned_rights, banned_by_user_id, banned_at)
			VALUES (?,?,?,?,?,?,?,?,?)`, channelID, userID, actorID, bannedAt, "", "", string(encoded), actorID, bannedAt)
	}
	if err != nil {
		return err
	}
	var next ChannelMember
	if previous != nil {
		next = *previous
	} else {
		next = ChannelMember{ChannelID: channelID, UserID: userID, InvitedBy: actorID, JoinedAt: bannedAt}
	}
	next.AdminRights = nil
	next.Rank = ""
	next.BannedRights = rights
	next.BannedBy = actorID
	next.BannedAt = bannedAt
	if err = SaveChannelAdminLogTx(tx, channelID, actorID, userID, ChannelAdminLogParticipantToggleBan,
		previous, &next, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func InviteChannelMembers(channelID, inviterID int64, userIDs []int64) error {
	if db == nil {
		return errors.New("domain mysql is not open")
	}
	if len(userIDs) == 0 {
		return ErrNoChannelMembers
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockChannelInvitePermission(tx, channelID, inviterID); err != nil {
		return err
	}
	seen := make(map[int64]struct{}, len(userIDs))
	joinedAt := time.Now().Unix()
	for _, userID := range userIDs {
		if userID <= 0 {
			return ErrInvalidChannelMember
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		var existing int
		err = tx.QueryRow(`SELECT 1 FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&existing)
		if err == nil {
			return ErrChannelMemberExists
		}
		if err != sql.ErrNoRows {
			return err
		}
		if userID == inviterID {
			return ErrChannelMemberExists
		}
		if _, err = tx.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at) VALUES (?,?,?,?)`,
			channelID, userID, inviterID, joinedAt); err != nil {
			return err
		}
		if err = SaveChannelAdminLogTx(tx, channelID, inviterID, userID, ChannelAdminLogParticipantInvite,
			nil, &ChannelMember{ChannelID: channelID, UserID: userID, InvitedBy: inviterID, JoinedAt: joinedAt}, joinedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// lockChannelInvitePermission locks the channel row before checking whether
// the caller is the creator or a persisted administrator with invite_users.
// The lock keeps a simultaneous creator/member update from changing the
// authorization decision between the check and the inserts below.
func lockChannelInvitePermission(tx *sql.Tx, channelID, userID int64) error {
	var creator int64
	if err := tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creator); err != nil {
		if err == sql.ErrNoRows {
			return ErrChannelMissing
		}
		return err
	}
	if creator == userID {
		return nil
	}
	var rawRights string
	if err := tx.QueryRow(`SELECT admin_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&rawRights); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotCreator
		}
		return err
	}
	var rights ChannelAdminRights
	if rawRights == "" || json.Unmarshal([]byte(rawRights), &rights) != nil || !rights.InviteUsers {
		return ErrNotCreator
	}
	return nil
}

// loadChannelMemberTx reads a member snapshot under the caller's transaction
// lock. It is intentionally private: callers should not expose partially
// decoded rows outside the mutation transaction.
func loadChannelMemberTx(tx *sql.Tx, channelID, userID int64) (ChannelMember, bool, error) {
	var member ChannelMember
	if tx == nil || channelID <= 0 || userID <= 0 {
		return member, false, ErrInvalidChannelMember
	}
	var invitedBy, joinedAt, bannedBy, bannedAt int64
	var rawRights, rank, rawBanned string
	err := tx.QueryRow(`SELECT invited_by_user_id, joined_at, admin_rights, admin_rank, banned_rights, banned_by_user_id, banned_at
		FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, userID).
		Scan(&invitedBy, &joinedAt, &rawRights, &rank, &rawBanned, &bannedBy, &bannedAt)
	if err == sql.ErrNoRows {
		return ChannelMember{}, false, nil
	}
	if err != nil {
		return ChannelMember{}, false, err
	}
	member = ChannelMember{ChannelID: channelID, UserID: userID, InvitedBy: invitedBy, JoinedAt: joinedAt, Rank: rank, BannedBy: bannedBy, BannedAt: bannedAt}
	if rawRights != "" {
		member.AdminRights = &ChannelAdminRights{}
		if err = json.Unmarshal([]byte(rawRights), member.AdminRights); err != nil {
			return ChannelMember{}, false, err
		}
		if member.AdminRights.Empty() {
			member.AdminRights = nil
		}
	}
	if rawBanned != "" {
		member.BannedRights = &ChannelBannedRights{}
		if err = json.Unmarshal([]byte(rawBanned), member.BannedRights); err != nil {
			return ChannelMember{}, false, err
		}
		if member.BannedRights.Empty() {
			member.BannedRights = nil
		}
	}
	return member, true, nil
}

func ChannelIsMember(channelID, userID int64) (bool, error) {
	member, ok, err := LoadChannelMember(channelID, userID)
	if err != nil || !ok {
		return ok, err
	}
	return !member.BannedRights.Kicks(time.Now().Unix()), nil
}

// ListChannelIDsForUser returns channels where the user is represented in the
// authoritative roster, including channels they created.  Creator rows are
// intentionally not duplicated in apifull_channel_member, so they are part of
// the query's explicit OR branch.
func ListChannelIDsForUser(userID int64) ([]int64, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if userID <= 0 {
		return []int64{}, nil
	}
	rows, err := db.Query(`SELECT id FROM apifull_channel
		WHERE creator_user_id=? OR EXISTS (
			SELECT 1 FROM apifull_channel_member
			WHERE channel_id=apifull_channel.id AND user_id=?)
		ORDER BY id`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// CommonChannelIDs returns channels where both users are active members.
// Creators are represented by apifull_channel rather than a member row.
func CommonChannelIDs(firstUserID, secondUserID int64) ([]int64, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if firstUserID <= 0 || secondUserID <= 0 || firstUserID == secondUserID {
		return []int64{}, nil
	}
	rows, err := db.Query(`SELECT id FROM apifull_channel
		WHERE (creator_user_id=? OR EXISTS (SELECT 1 FROM apifull_channel_member WHERE channel_id=apifull_channel.id AND user_id=?))
		  AND (creator_user_id=? OR EXISTS (SELECT 1 FROM apifull_channel_member WHERE channel_id=apifull_channel.id AND user_id=?))
		ORDER BY id`, firstUserID, firstUserID, secondUserID, secondUserID)
	if err != nil {
		return nil, err
	}
	candidates := make([]int64, 0)
	for rows.Next() {
		var channelID int64
		if err := rows.Scan(&channelID); err != nil {
			return nil, err
		}
		candidates = append(candidates, channelID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(candidates))
	for _, channelID := range candidates {
		firstMember, err := ChannelIsMember(channelID, firstUserID)
		if err != nil {
			return nil, err
		}
		secondMember, err := ChannelIsMember(channelID, secondUserID)
		if err != nil {
			return nil, err
		}
		if firstMember && secondMember {
			ids = append(ids, channelID)
		}
	}
	return ids, nil
}

func LoadChannelMember(channelID, userID int64) (ChannelMember, bool, error) {
	var member ChannelMember
	ch, ok, err := LoadChannel(channelID)
	if err != nil || !ok {
		if !ok && err == nil {
			return member, false, ErrChannelMissing
		}
		return member, false, err
	}
	if userID == ch.Creator {
		return ChannelMember{ChannelID: channelID, UserID: userID, InvitedBy: userID, JoinedAt: ch.CreatedAt, Creator: true}, true, nil
	}
	var rawRights string
	var rawBanned string
	err = db.QueryRow(`SELECT invited_by_user_id, joined_at, admin_rights, admin_rank, banned_rights, banned_by_user_id, banned_at
		FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID).
		Scan(&member.InvitedBy, &member.JoinedAt, &rawRights, &member.Rank, &rawBanned, &member.BannedBy, &member.BannedAt)
	if err == sql.ErrNoRows {
		return ChannelMember{}, false, nil
	}
	if err != nil {
		return ChannelMember{}, false, err
	}
	member.ChannelID = channelID
	member.UserID = userID
	if rawRights != "" {
		member.AdminRights = &ChannelAdminRights{}
		if err = json.Unmarshal([]byte(rawRights), member.AdminRights); err != nil {
			return ChannelMember{}, false, err
		}
		if member.AdminRights.Empty() {
			member.AdminRights = nil
		}
	}
	if rawBanned != "" {
		member.BannedRights = &ChannelBannedRights{}
		if err = json.Unmarshal([]byte(rawBanned), member.BannedRights); err != nil {
			return ChannelMember{}, false, err
		}
		if member.BannedRights.Empty() {
			member.BannedRights = nil
		}
	}
	return member, true, nil
}

func ListChannelMembers(channelID int64) ([]ChannelMember, error) {
	ch, ok, err := LoadChannel(channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrChannelMissing
	}
	rows, err := db.Query(`SELECT user_id, invited_by_user_id, joined_at, admin_rights, admin_rank, banned_rights, banned_by_user_id, banned_at FROM apifull_channel_member WHERE channel_id=? ORDER BY joined_at DESC, user_id ASC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []ChannelMember{{ChannelID: channelID, UserID: ch.Creator, InvitedBy: ch.Creator, JoinedAt: ch.CreatedAt, Creator: true}}
	for rows.Next() {
		var member ChannelMember
		var rawRights string
		var rawBanned string
		member.ChannelID = channelID
		if err = rows.Scan(&member.UserID, &member.InvitedBy, &member.JoinedAt, &rawRights, &member.Rank, &rawBanned, &member.BannedBy, &member.BannedAt); err != nil {
			return nil, err
		}
		if member.UserID == ch.Creator {
			continue
		}
		if rawRights != "" {
			member.AdminRights = &ChannelAdminRights{}
			if err = json.Unmarshal([]byte(rawRights), member.AdminRights); err != nil {
				return nil, err
			}
			if member.AdminRights.Empty() {
				member.AdminRights = nil
			}
		}
		if rawBanned != "" {
			member.BannedRights = &ChannelBannedRights{}
			if err = json.Unmarshal([]byte(rawBanned), member.BannedRights); err != nil {
				return nil, err
			}
			if member.BannedRights.Empty() {
				member.BannedRights = nil
			}
		}
		members = append(members, member)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}
