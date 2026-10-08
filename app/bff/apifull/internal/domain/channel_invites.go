package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrChannelInviteMissing = errors.New("channel invite missing")
	ErrChannelInviteExpired = errors.New("channel invite expired")
	ErrChannelInviteBanned  = errors.New("channel invite user banned")
)

// ChannelInvite is the subset of the shared chat_invites row used by an
// APIFull channel. The invite rows intentionally remain shared with basic
// chats so the management RPCs have one source of truth.
type ChannelInvite struct {
	ID            int64
	ChannelID     int64
	AdminID       int64
	Link          string
	Permanent     bool
	RequestNeeded bool
	Revoked       bool
	StartDate     int64
	ExpireDate    int64
	UsageLimit    int32
	Usage         int32
	Requested     int32
	Title         string
	Date          int64
}

type ChannelInviteImport struct {
	Channel       Channel
	RequestNeeded bool
}

type ChannelInviteUpdate struct {
	Revoked       bool
	ExpireDate    *int32
	UsageLimit    *int32
	RequestNeeded *bool
	Title         *string
}

type ChannelInviteImporter struct {
	UserID     int64
	Requested  bool
	ApprovedBy int64
	Date       int64
}

// CreateChannelInvite writes an APIFull channel invite into the shared invite
// table. Authorization belongs to the caller because this persistence helper
// is also used to create the replacement for a permanent invite.
func CreateChannelInvite(invite ChannelInvite) (ChannelInvite, error) {
	if db == nil {
		return ChannelInvite{}, errors.New("domain PostgreSQL is not open")
	}
	if invite.ChannelID <= 0 || invite.AdminID <= 0 || invite.Link == "" {
		return ChannelInvite{}, ErrChannelInviteMissing
	}
	if invite.Date == 0 {
		invite.Date = time.Now().Unix()
	}
	_, err := db.Exec(`INSERT INTO chat_invites
		(chat_id, admin_id, link, permanent, revoked, request_needed, start_date, expire_date,
		usage_limit, usage2, requested, title, date2)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		invite.ChannelID, invite.AdminID, invite.Link, invite.Permanent, invite.Revoked,
		invite.RequestNeeded, invite.StartDate, invite.ExpireDate, invite.UsageLimit,
		invite.Usage, invite.Requested, invite.Title, invite.Date)
	if err != nil {
		return ChannelInvite{}, err
	}
	return loadChannelInvite(db, invite.Link)
}

func GetChannelInvite(channelID int64, link string) (ChannelInvite, error) {
	if db == nil {
		return ChannelInvite{}, errors.New("domain PostgreSQL is not open")
	}
	invite, err := loadChannelInvite(db, link)
	if err != nil {
		return ChannelInvite{}, err
	}
	if invite.ChannelID != channelID {
		return ChannelInvite{}, ErrChannelInviteMissing
	}
	return invite, nil
}

func ListChannelInvites(channelID, adminID int64) ([]ChannelInvite, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT id, chat_id, admin_id, link, permanent, revoked, request_needed, start_date,
		expire_date, usage_limit, usage2, requested, title, date2
		FROM chat_invites WHERE chat_id=? AND admin_id=?`, channelID, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelInvites(rows)
}

func ListChannelInvitesForChannel(channelID int64) ([]ChannelInvite, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	rows, err := db.Query(`SELECT id, chat_id, admin_id, link, permanent, revoked, request_needed, start_date,
		expire_date, usage_limit, usage2, requested, title, date2
		FROM chat_invites WHERE chat_id=?`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChannelInvites(rows)
}

func UpdateChannelInvite(channelID int64, link, replacementLink string, update ChannelInviteUpdate, now int64) ([]ChannelInvite, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	invite, err := loadChannelInviteTx(tx, link)
	if err != nil {
		return nil, err
	}
	if invite.ChannelID != channelID {
		return nil, ErrChannelInviteMissing
	}

	if update.Revoked {
		if _, err = tx.Exec(`UPDATE chat_invites SET revoked=1 WHERE chat_id=? AND link=?`, channelID, link); err != nil {
			return nil, err
		}
		invite.Revoked = true
		result := []ChannelInvite{invite}
		if invite.Permanent {
			if replacementLink == "" {
				return nil, ErrChannelInviteMissing
			}
			replacement := ChannelInvite{
				ChannelID: channelID,
				AdminID:   invite.AdminID,
				Link:      replacementLink,
				Permanent: true,
				Date:      now,
			}
			if _, err = tx.Exec(`INSERT INTO chat_invites
				(chat_id, admin_id, link, permanent, revoked, request_needed, start_date, expire_date,
				usage_limit, usage2, requested, title, date2)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				replacement.ChannelID, replacement.AdminID, replacement.Link, replacement.Permanent,
				replacement.Revoked, replacement.RequestNeeded, replacement.StartDate,
				replacement.ExpireDate, replacement.UsageLimit, replacement.Usage,
				replacement.Requested, replacement.Title, replacement.Date); err != nil {
				return nil, err
			}
			result = append(result, replacement)
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}

	sets := make([]string, 0, 4)
	args := make([]any, 0, 6)
	if update.ExpireDate != nil {
		sets = append(sets, "expire_date=?")
		args = append(args, *update.ExpireDate)
		invite.ExpireDate = int64(*update.ExpireDate)
	}
	if update.UsageLimit != nil {
		sets = append(sets, "usage_limit=?")
		args = append(args, *update.UsageLimit)
		invite.UsageLimit = *update.UsageLimit
	}
	if update.RequestNeeded != nil {
		sets = append(sets, "request_needed=?")
		args = append(args, *update.RequestNeeded)
		invite.RequestNeeded = *update.RequestNeeded
	}
	if update.Title != nil {
		sets = append(sets, "title=?")
		args = append(args, *update.Title)
		invite.Title = *update.Title
	}
	if len(sets) > 0 {
		query := "UPDATE chat_invites SET " + strings.Join(sets, ", ") + " WHERE chat_id=? AND link=?"
		args = append(args, channelID, link)
		if _, err = tx.Exec(query, args...); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return []ChannelInvite{invite}, nil
}

func DeleteChannelInvite(channelID int64, link string) (bool, error) {
	if db == nil {
		return false, errors.New("domain PostgreSQL is not open")
	}
	result, err := db.Exec(`DELETE FROM chat_invites WHERE chat_id=? AND link=?`, channelID, link)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func DeleteRevokedChannelInvites(channelID, adminID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	_, err := db.Exec(`DELETE FROM chat_invites WHERE chat_id=? AND admin_id=? AND revoked=1`, channelID, adminID)
	return err
}

func CountChannelInviteParticipants(channelID int64, link string, requested bool) (int32, error) {
	if db == nil {
		return 0, errors.New("domain PostgreSQL is not open")
	}
	var count int32
	err := db.QueryRow(`SELECT COUNT(*) FROM chat_invite_participants
		WHERE chat_id=? AND link=? AND requested=?`, channelID, link, requested).Scan(&count)
	return count, err
}

func ListChannelInviteImporters(channelID int64, link string, requested bool, query string) ([]ChannelInviteImporter, error) {
	if db == nil {
		return nil, errors.New("domain PostgreSQL is not open")
	}

	args := []any{channelID, requested}
	statement := `SELECT p.user_id, p.requested, p.approved_by, p.date2
		FROM chat_invite_participants p`
	if query != "" {
		statement += ` INNER JOIN users u ON u.id=p.user_id`
	}
	statement += ` WHERE p.chat_id=? AND p.requested=?`
	if !requested || link != "" {
		statement += ` AND p.link=?`
		args = append(args, link)
	}
	if query != "" {
		statement += ` AND u.deleted=0 AND (u.username LIKE ? OR u.first_name LIKE ? OR u.last_name LIKE ?)`
		args = append(args, query+"%", "%"+query+"%", "%"+query+"%")
	}
	statement += ` ORDER BY p.date2 DESC, p.id DESC`
	rows, err := db.Query(statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	importers := make([]ChannelInviteImporter, 0)
	for rows.Next() {
		var importer ChannelInviteImporter
		if err = rows.Scan(&importer.UserID, &importer.Requested, &importer.ApprovedBy, &importer.Date); err != nil {
			return nil, err
		}
		importers = append(importers, importer)
	}
	return importers, rows.Err()
}

// ResolveChannelInviteRequest approves or rejects pending requests for one
// channel member. An empty link applies to all invite links for that member;
// a non-empty link scopes the operation to that invite.
func ResolveChannelInviteRequest(channelID, userID, approverID int64, link string, approved bool, now int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if channelID <= 0 || userID <= 0 || approverID <= 0 {
		return ErrInvalidChannelMember
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var creatorID int64
	if err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=? FOR UPDATE`, channelID).Scan(&creatorID); errors.Is(err, sql.ErrNoRows) {
		return ErrChannelInviteMissing
	} else if err != nil {
		return err
	}
	if approverID != creatorID {
		var rawRights, rawBanned string
		if err = tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, approverID).Scan(&rawRights, &rawBanned); errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidChannelMember
		} else if err != nil {
			return err
		}
		var rights ChannelAdminRights
		if rawRights != "" {
			if err = json.Unmarshal([]byte(rawRights), &rights); err != nil {
				return err
			}
		}
		var banned ChannelBannedRights
		if rawBanned != "" {
			if err = json.Unmarshal([]byte(rawBanned), &banned); err != nil {
				return err
			}
		}
		if banned.Active(now) && (banned.ViewMessages || banned.InviteUsers) {
			return ErrInvalidChannelMember
		}
		if !rights.InviteUsers {
			return ErrInvalidChannelMember
		}
	}

	args := []any{channelID, userID}
	where := ` WHERE chat_id=? AND user_id=? AND requested=1`
	if link != "" {
		where += ` AND link=?`
		args = append(args, link)
	}
	rows, err := tx.Query(`SELECT id FROM chat_invite_participants`+where+` FOR UPDATE`, args...)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		found = true
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		return rowsErr
	}
	if !found {
		return tx.Commit()
	}

	if approved {
		if userID == creatorID {
			return ErrChannelMemberExists
		}
		var existing int
		err = tx.QueryRow(`SELECT 1 FROM apifull_channel_member WHERE channel_id=? AND user_id=? FOR UPDATE`, channelID, userID).Scan(&existing)
		if err == nil {
			return ErrChannelMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO apifull_channel_member (channel_id, user_id, invited_by_user_id, joined_at) VALUES (?,?,?,?)`, channelID, userID, approverID, now); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE chat_invite_participants SET requested=0, approved_by=?`+where, append([]any{approverID}, args...)...)
	} else {
		_, err = tx.Exec(`DELETE FROM chat_invite_participants`+where, args...)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func scanChannelInvites(rows *sql.Rows) ([]ChannelInvite, error) {
	invites := make([]ChannelInvite, 0)
	for rows.Next() {
		var invite ChannelInvite
		if err := rows.Scan(&invite.ID, &invite.ChannelID, &invite.AdminID, &invite.Link,
			&invite.Permanent, &invite.Revoked, &invite.RequestNeeded, &invite.StartDate,
			&invite.ExpireDate, &invite.UsageLimit, &invite.Usage, &invite.Requested,
			&invite.Title, &invite.Date); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// ChannelCanInvite reports whether a current channel member may manage
// exported invites. The creator always has the permission.
func ChannelCanInvite(ch Channel, userID, now int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidChannelMember
	}
	if ch.Creator == userID {
		return true, nil
	}
	member, ok, err := LoadChannelMember(ch.ID, userID)
	if err != nil || !ok {
		return false, err
	}
	if member.BannedRights.Active(now) && (member.BannedRights.ViewMessages || member.BannedRights.InviteUsers) {
		return false, nil
	}
	return member.AdminRights != nil && member.AdminRights.InviteUsers, nil
}

// CheckChannelInvite validates an active APIFull channel invite and returns
// whether userID is already an active channel member.
func CheckChannelInvite(hash string, userID int64, now int64) (ChannelInvite, Channel, bool, error) {
	invite, channel, err := activeChannelInvite(hash, now)
	if err != nil {
		return ChannelInvite{}, Channel{}, false, err
	}
	member, err := ChannelIsMember(channel.ID, userID)
	if err != nil {
		return ChannelInvite{}, Channel{}, false, err
	}
	return invite, channel, member, nil
}

// ImportChannelInvite records either a join request or an active membership.
// It locks the invite row while checking its limits so concurrent imports do
// not exceed a configured usage limit.
func ImportChannelInvite(hash string, userID, now int64) (ChannelInviteImport, error) {
	if db == nil {
		return ChannelInviteImport{}, errors.New("domain PostgreSQL is not open")
	}
	if userID <= 0 {
		return ChannelInviteImport{}, ErrInvalidChannelMember
	}

	tx, err := db.Begin()
	if err != nil {
		return ChannelInviteImport{}, err
	}
	defer tx.Rollback()

	invite, channel, err := activeChannelInviteTx(tx, hash, now)
	if err != nil {
		return ChannelInviteImport{}, err
	}
	if channel.Creator == userID {
		return ChannelInviteImport{}, ErrChannelMemberExists
	}

	var bannedRights string
	err = tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member
		WHERE channel_id=? AND user_id=? FOR UPDATE`, channel.ID, userID).Scan(new(string), &bannedRights)
	if err == nil {
		var banned ChannelBannedRights
		if bannedRights != "" {
			if err = jsonUnmarshalChannelBannedRights(bannedRights, &banned); err != nil {
				return ChannelInviteImport{}, err
			}
		}
		if banned.Kicks(now) {
			return ChannelInviteImport{}, ErrChannelInviteBanned
		}
		return ChannelInviteImport{}, ErrChannelMemberExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ChannelInviteImport{}, err
	}

	if invite.UsageLimit > 0 {
		var usage int32
		if err = tx.QueryRow(`SELECT COUNT(*) FROM chat_invite_participants
			WHERE chat_id=? AND link=? AND requested=0`, invite.ChannelID, invite.Link).Scan(&usage); err != nil {
			return ChannelInviteImport{}, err
		}
		if usage >= invite.UsageLimit {
			return ChannelInviteImport{}, ErrChannelInviteExpired
		}
	}

	if invite.RequestNeeded {
		var requested int
		err = tx.QueryRow(`SELECT 1 FROM chat_invite_participants
			WHERE chat_id=? AND link=? AND user_id=? AND requested=1 LIMIT 1 FOR UPDATE`,
			invite.ChannelID, invite.Link, userID).Scan(&requested)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err = tx.Exec(`INSERT INTO chat_invite_participants
				(chat_id, link, user_id, requested, approved_by, date2) VALUES (?,?,?,?,?,?)`,
				invite.ChannelID, invite.Link, userID, 1, 0, now); err != nil {
				return ChannelInviteImport{}, err
			}
		} else if err != nil {
			return ChannelInviteImport{}, err
		}
		if err = tx.Commit(); err != nil {
			return ChannelInviteImport{}, err
		}
		return ChannelInviteImport{Channel: channel, RequestNeeded: true}, nil
	}

	if _, err = tx.Exec(`INSERT INTO apifull_channel_member
		(channel_id, user_id, invited_by_user_id, joined_at) VALUES (?,?,?,?)`,
		channel.ID, userID, invite.AdminID, now); err != nil {
		return ChannelInviteImport{}, err
	}
	if _, err = tx.Exec(`INSERT INTO chat_invite_participants
		(chat_id, link, user_id, requested, approved_by, date2) VALUES (?,?,?,?,?,?)`,
		invite.ChannelID, invite.Link, userID, 0, 0, now); err != nil {
		return ChannelInviteImport{}, err
	}
	if err = tx.Commit(); err != nil {
		return ChannelInviteImport{}, err
	}
	return ChannelInviteImport{Channel: channel}, nil
}

func activeChannelInvite(hash string, now int64) (ChannelInvite, Channel, error) {
	if db == nil {
		return ChannelInvite{}, Channel{}, errors.New("domain PostgreSQL is not open")
	}
	invite, err := loadChannelInvite(db, hash)
	if err != nil {
		return ChannelInvite{}, Channel{}, err
	}
	channel, ok, err := LoadChannel(invite.ChannelID)
	if err != nil {
		return ChannelInvite{}, Channel{}, err
	}
	if !ok {
		return ChannelInvite{}, Channel{}, ErrChannelInviteMissing
	}
	if err = validateChannelInvite(invite, channel, now); err != nil {
		return ChannelInvite{}, Channel{}, err
	}
	return invite, channel, nil
}

func activeChannelInviteTx(tx *sql.Tx, hash string, now int64) (ChannelInvite, Channel, error) {
	invite, err := loadChannelInviteTx(tx, hash)
	if err != nil {
		return ChannelInvite{}, Channel{}, err
	}

	var channel Channel
	err = tx.QueryRow(`SELECT id, access_hash, creator_user_id, title, about, broadcast, megagroup,
		signatures_enabled, signature_profiles_enabled, hidden_prehistory, participants_hidden,
		slowmode_seconds, created_at FROM apifull_channel WHERE id=? FOR UPDATE`, invite.ChannelID).Scan(
		&channel.ID, &channel.AccessHash, &channel.Creator, &channel.Title, &channel.About,
		&channel.Broadcast, &channel.Megagroup, &channel.Signatures, &channel.SignatureProfiles,
		&channel.HiddenPrehistory, &channel.ParticipantsHidden, &channel.SlowmodeSeconds, &channel.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelInvite{}, Channel{}, ErrChannelInviteMissing
	}
	if err != nil {
		return ChannelInvite{}, Channel{}, err
	}
	if err = validateChannelInviteTx(tx, invite, channel, now); err != nil {
		return ChannelInvite{}, Channel{}, err
	}
	return invite, channel, nil
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func loadChannelInvite(q queryRower, hash string) (ChannelInvite, error) {
	var invite ChannelInvite
	err := q.QueryRow(`SELECT id, chat_id, admin_id, link, permanent, revoked, request_needed, start_date,
		expire_date, usage_limit, usage2, requested, title, date2
		FROM chat_invites WHERE link=?`, hash).Scan(
		&invite.ID, &invite.ChannelID, &invite.AdminID, &invite.Link, &invite.Permanent,
		&invite.Revoked, &invite.RequestNeeded, &invite.StartDate, &invite.ExpireDate,
		&invite.UsageLimit, &invite.Usage, &invite.Requested, &invite.Title, &invite.Date)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelInvite{}, ErrChannelInviteMissing
	}
	return invite, err
}

func loadChannelInviteTx(tx *sql.Tx, hash string) (ChannelInvite, error) {
	var invite ChannelInvite
	err := tx.QueryRow(`SELECT id, chat_id, admin_id, link, permanent, revoked, request_needed, start_date,
		expire_date, usage_limit, usage2, requested, title, date2
		FROM chat_invites WHERE link=? FOR UPDATE`, hash).Scan(
		&invite.ID, &invite.ChannelID, &invite.AdminID, &invite.Link, &invite.Permanent,
		&invite.Revoked, &invite.RequestNeeded, &invite.StartDate, &invite.ExpireDate,
		&invite.UsageLimit, &invite.Usage, &invite.Requested, &invite.Title, &invite.Date)
	if errors.Is(err, sql.ErrNoRows) {
		return ChannelInvite{}, ErrChannelInviteMissing
	}
	return invite, err
}

func validateChannelInvite(invite ChannelInvite, channel Channel, now int64) error {
	if invite.Revoked || (invite.ExpireDate != 0 && invite.ExpireDate < now) {
		return ErrChannelInviteExpired
	}
	if invite.UsageLimit > 0 {
		var usage int32
		if err := db.QueryRow(`SELECT COUNT(*) FROM chat_invite_participants
			WHERE chat_id=? AND link=? AND requested=0`, invite.ChannelID, invite.Link).Scan(&usage); err != nil {
			return err
		}
		if usage >= invite.UsageLimit {
			return ErrChannelInviteExpired
		}
	}
	allowed, err := ChannelCanInvite(channel, invite.AdminID, now)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrChannelInviteExpired
	}
	return nil
}

func validateChannelInviteTx(tx *sql.Tx, invite ChannelInvite, channel Channel, now int64) error {
	if invite.Revoked || (invite.ExpireDate != 0 && invite.ExpireDate < now) {
		return ErrChannelInviteExpired
	}
	if invite.UsageLimit > 0 {
		var usage int32
		if err := tx.QueryRow(`SELECT COUNT(*) FROM chat_invite_participants
			WHERE chat_id=? AND link=? AND requested=0`, invite.ChannelID, invite.Link).Scan(&usage); err != nil {
			return err
		}
		if usage >= invite.UsageLimit {
			return ErrChannelInviteExpired
		}
	}
	allowed, err := channelCanInviteTx(tx, channel, invite.AdminID, now)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrChannelInviteExpired
	}
	return nil
}

func channelCanInviteTx(tx *sql.Tx, channel Channel, userID, now int64) (bool, error) {
	if channel.Creator == userID {
		return true, nil
	}
	var adminRights, bannedRights string
	err := tx.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member
		WHERE channel_id=? AND user_id=?`, channel.ID, userID).Scan(&adminRights, &bannedRights)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var banned ChannelBannedRights
	if bannedRights != "" {
		if err = jsonUnmarshalChannelBannedRights(bannedRights, &banned); err != nil {
			return false, err
		}
	}
	if banned.Active(now) && (banned.ViewMessages || banned.InviteUsers) {
		return false, nil
	}
	var rights ChannelAdminRights
	if adminRights == "" {
		return false, nil
	}
	if err = json.Unmarshal([]byte(adminRights), &rights); err != nil {
		return false, err
	}
	return rights.InviteUsers, nil
}

func jsonUnmarshalChannelBannedRights(value string, rights *ChannelBannedRights) error {
	return json.Unmarshal([]byte(value), rights)
}
