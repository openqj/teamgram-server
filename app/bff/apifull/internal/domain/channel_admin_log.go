package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ChannelAdminLogAction values are deliberately kept independent of the
// MTProto constructors. The wire layer can choose the appropriate action
// constructor while the domain stores an immutable audit record.
const (
	ChannelAdminLogParticipantJoin        = "participant_join"
	ChannelAdminLogParticipantLeave       = "participant_leave"
	ChannelAdminLogParticipantInvite      = "participant_invite"
	ChannelAdminLogParticipantToggleBan   = "participant_toggle_ban"
	ChannelAdminLogParticipantToggleAdmin = "participant_toggle_admin"
)

// ChannelAdminLog is one immutable channel administration event. Member
// snapshots are kept with the event because a leave/kick removes the live
// member row and the log must remain readable afterwards.
type ChannelAdminLog struct {
	ID        int64
	ChannelID int64
	ActorID   int64
	TargetID  int64
	Action    string
	Date      int64
	Prev      *ChannelMember
	New       *ChannelMember
}

// SaveChannelAdminLogTx appends an audit row in the caller's transaction.
// Keeping the write in the membership transaction prevents a successful
// mutation from being reported without its corresponding audit event.
func SaveChannelAdminLogTx(tx *sql.Tx, channelID, actorID, targetID int64, action string, prev, next *ChannelMember, date int64) error {
	if tx == nil || channelID <= 0 || actorID <= 0 || targetID <= 0 || strings.TrimSpace(action) == "" {
		return ErrInvalidChannelMember
	}
	if date <= 0 {
		date = time.Now().Unix()
	}
	prevRaw, err := marshalChannelMemberSnapshot(prev)
	if err != nil {
		return err
	}
	nextRaw, err := marshalChannelMemberSnapshot(next)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO apifull_channel_admin_log
		(channel_id, actor_user_id, target_user_id, action, prev_member, new_member, date)
		VALUES (?,?,?,?,?,?,?)`, channelID, actorID, targetID, action, prevRaw, nextRaw, date)
	return err
}

func marshalChannelMemberSnapshot(member *ChannelMember) (string, error) {
	if member == nil {
		return "", nil
	}
	raw, err := json.Marshal(member)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalChannelMemberSnapshot(raw string) (*ChannelMember, error) {
	if raw == "" {
		return nil, nil
	}
	var member ChannelMember
	if err := json.Unmarshal([]byte(raw), &member); err != nil {
		return nil, err
	}
	return &member, nil
}

// CanViewChannelAdminLog grants the creator and active administrators access
// to the audit stream. Banned or missing members fail closed.
func CanViewChannelAdminLog(channelID, userID int64) (bool, error) {
	if db == nil {
		return false, errors.New("domain mysql is not open")
	}
	if channelID <= 0 || userID <= 0 {
		return false, ErrInvalidChannelMember
	}
	var creator int64
	if err := db.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=?`, channelID).Scan(&creator); err != nil {
		if err == sql.ErrNoRows {
			return false, ErrChannelMissing
		}
		return false, err
	}
	if creator == userID {
		return true, nil
	}
	var rawRights, rawBanned string
	if err := db.QueryRow(`SELECT admin_rights, banned_rights FROM apifull_channel_member WHERE channel_id=? AND user_id=?`, channelID, userID).
		Scan(&rawRights, &rawBanned); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if rawRights == "" {
		return false, nil
	}
	var rights ChannelAdminRights
	if err := json.Unmarshal([]byte(rawRights), &rights); err != nil || rights.Empty() {
		return false, nil
	}
	if rawBanned != "" {
		var banned ChannelBannedRights
		if err := json.Unmarshal([]byte(rawBanned), &banned); err != nil {
			return false, nil
		}
		if banned.Active(time.Now().Unix()) {
			return false, nil
		}
	}
	return true, nil
}

// ListChannelAdminLogs returns newest events first. Actor filtering is done in
// SQL so an admin-filtered request cannot lose older matching records merely
// because unrelated events filled the page.
func ListChannelAdminLogs(channelID, minID, maxID int64, limit int32, actorIDs []int64) ([]ChannelAdminLog, error) {
	if db == nil {
		return nil, errors.New("domain mysql is not open")
	}
	if channelID <= 0 {
		return nil, ErrChannelMissing
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}
	query := `SELECT id, actor_user_id, target_user_id, action, prev_member, new_member, date
		FROM apifull_channel_admin_log WHERE channel_id=?`
	args := []any{channelID}
	if maxID > 0 {
		query += ` AND id<=?`
		args = append(args, maxID)
	}
	if minID > 0 {
		query += ` AND id>=?`
		args = append(args, minID)
	}
	if len(actorIDs) > 0 {
		placeholders := make([]string, len(actorIDs))
		for i, actorID := range actorIDs {
			if actorID <= 0 {
				return nil, ErrInvalidChannelMember
			}
			placeholders[i] = "?"
			args = append(args, actorID)
		}
		query += ` AND actor_user_id IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ChannelAdminLog, 0, limit)
	for rows.Next() {
		var event ChannelAdminLog
		var prevRaw, nextRaw string
		if err = rows.Scan(&event.ID, &event.ActorID, &event.TargetID, &event.Action, &prevRaw, &nextRaw, &event.Date); err != nil {
			return nil, err
		}
		event.ChannelID = channelID
		if event.Prev, err = unmarshalChannelMemberSnapshot(prevRaw); err != nil {
			return nil, err
		}
		if event.New, err = unmarshalChannelMemberSnapshot(nextRaw); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
