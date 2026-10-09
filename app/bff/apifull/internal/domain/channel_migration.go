package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrChannelMigrationConflict = errors.New("channel migration conflicts with existing data")

type MigratedChannelMember struct {
	UserID      int64
	InvitedBy   int64
	JoinedAt    int64
	AdminRights *ChannelAdminRights
	Rank        string
}

type MigratedChannel struct {
	ChatID     int64
	ChannelID  int64
	AccessHash int64
	CreatorID  int64
	Title      string
	About      string
	CreatedAt  int64
	Members    []MigratedChannelMember
}

// ImportMigratedChannel atomically creates the APIFull channel projection and
// its initial roster. Repeating the same migration is safe; conflicting channel
// identities fail without changing the stored projection.
func ImportMigratedChannel(in MigratedChannel) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if in.ChatID <= 0 || in.ChannelID <= 0 || in.AccessHash == 0 || in.CreatorID <= 0 {
		return ErrChannelMigrationConflict
	}
	if in.CreatedAt <= 0 {
		in.CreatedAt = time.Now().Unix()
	}
	members, err := normalizeMigratedMembers(in)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingID, existingHash, existingChat, existingCreator int64
	err = tx.QueryRow(`SELECT id, access_hash, COALESCE(migrated_from_chat_id, 0), creator_user_id
		FROM apifull_channel WHERE id=? OR migrated_from_chat_id=? FOR UPDATE`, in.ChannelID, in.ChatID).
		Scan(&existingID, &existingHash, &existingChat, &existingCreator)
	switch {
	case err == nil:
		if existingID != in.ChannelID || existingHash != in.AccessHash || existingChat != in.ChatID || existingCreator != in.CreatorID {
			return ErrChannelMigrationConflict
		}
	case errors.Is(err, sql.ErrNoRows):
		if _, err = tx.Exec(`INSERT INTO apifull_channel
			(id, access_hash, migrated_from_chat_id, creator_user_id, title, about, broadcast, megagroup, created_at)
			VALUES (?,?,?,?,?,?,0,1,?)`, in.ChannelID, in.AccessHash, in.ChatID, in.CreatorID, in.Title, in.About, in.CreatedAt); err != nil {
			return err
		}
	default:
		return err
	}

	for _, member := range members {
		rights := ""
		if !member.AdminRights.Empty() {
			encoded, marshalErr := json.Marshal(member.AdminRights)
			if marshalErr != nil {
				return marshalErr
			}
			rights = string(encoded)
		}
		if _, err = tx.Exec(`INSERT INTO apifull_channel_member
			(channel_id, user_id, invited_by_user_id, joined_at, admin_rights, admin_rank)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (channel_id, user_id) DO UPDATE SET invited_by_user_id=EXCLUDED.invited_by_user_id, joined_at=EXCLUDED.joined_at,
				admin_rights=EXCLUDED.admin_rights, admin_rank=EXCLUDED.admin_rank, banned_rights='', banned_by_user_id=0, banned_at=0`,
			in.ChannelID, member.UserID, member.InvitedBy, member.JoinedAt, rights, member.Rank); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeMigratedMembers(in MigratedChannel) ([]MigratedChannelMember, error) {
	seen := map[int64]struct{}{in.CreatorID: {}}
	members := make([]MigratedChannelMember, 0, len(in.Members))
	for _, member := range in.Members {
		if member.UserID <= 0 {
			return nil, ErrInvalidChannelMember
		}
		if _, ok := seen[member.UserID]; ok {
			continue
		}
		seen[member.UserID] = struct{}{}
		if member.InvitedBy <= 0 {
			member.InvitedBy = in.CreatorID
		}
		if member.JoinedAt <= 0 {
			member.JoinedAt = in.CreatedAt
		}
		members = append(members, member)
	}
	return members, nil
}
