package domain

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// SetBotDefaultAdminRights stores the canonical ChatAdminRights constructor
// for a bot. The two rights families are kept in one row so an update is
// atomic and a bot can never expose a partially replaced defaults record.
func SetBotDefaultAdminRights(botID int64, group bool, rights []byte) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if botID <= 0 || !json.Valid(rights) {
		return errors.New("invalid bot default admin rights")
	}
	column := "broadcast_admin_rights"
	if group {
		column = "group_admin_rights"
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_bot_default_admin_rights
		(bot_user_id, group_admin_rights, broadcast_admin_rights, updated_at)
		VALUES ($1, CASE WHEN $2 THEN $3::jsonb ELSE '{}'::jsonb END,
			CASE WHEN $2 THEN '{}'::jsonb ELSE $3::jsonb END, CURRENT_TIMESTAMP)
		ON CONFLICT (bot_user_id) DO UPDATE SET `+column+`=$3::jsonb,
		updated_at=CURRENT_TIMESTAMP`, botID, group, string(rights))
	return err
}

func GetBotDefaultAdminRights(botID int64, group bool) ([]byte, bool, error) {
	if db == nil {
		return nil, false, errors.New("domain PostgreSQL is not open")
	}
	if botID <= 0 {
		return nil, false, errors.New("invalid bot id")
	}
	column := "broadcast_admin_rights"
	if group {
		column = "group_admin_rights"
	}
	var rights []byte
	err := db.QueryRow(`SELECT `+column+` FROM apifull_bot_default_admin_rights WHERE bot_user_id=$1`, botID).Scan(&rights)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return rights, true, nil
}

// SetBotMenuButton stores one bot menu button per owner/bot pair. The JSON
// payload is the canonical Layer 229 constructor encoded by APIFull.
func SetBotMenuButton(ownerID, botID int64, button []byte) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if ownerID <= 0 || botID <= 0 || !json.Valid(button) {
		return errors.New("invalid bot menu button state")
	}
	_, err := execPostgresMutation(`INSERT INTO apifull_bot_menu_button
		(owner_user_id, bot_user_id, button, updated_at)
		VALUES ($1,$2,$3::jsonb,CURRENT_TIMESTAMP)
		ON CONFLICT (owner_user_id, bot_user_id) DO UPDATE
		SET button=EXCLUDED.button, updated_at=CURRENT_TIMESTAMP`, ownerID, botID, string(button))
	return err
}

func GetBotMenuButton(ownerID, botID int64) ([]byte, bool, error) {
	if db == nil {
		return nil, false, errors.New("domain PostgreSQL is not open")
	}
	var button []byte
	err := db.QueryRow(`SELECT button FROM apifull_bot_menu_button WHERE owner_user_id=$1 AND bot_user_id=$2`, ownerID, botID).Scan(&button)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return button, true, nil
}

// SetChannelStickerSet changes a channel's sticker binding atomically with
// the admin permission check. The creator or an admin with change_info may
// update it; stickerSetID=0 clears the binding.
func SetChannelStickerSet(actorID, channelID, stickerSetID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if actorID <= 0 || channelID <= 0 || stickerSetID < 0 {
		return ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var creator int64
	if err = tx.QueryRow(`SELECT creator_user_id FROM apifull_channel WHERE id=$1 FOR UPDATE`, channelID).Scan(&creator); err == sql.ErrNoRows {
		return ErrChannelMissing
	} else if err != nil {
		return err
	}
	if creator != actorID {
		var adminJSON, bannedJSON string
		if err = tx.QueryRow(`SELECT admin_rights,banned_rights FROM apifull_channel_member WHERE channel_id=$1 AND user_id=$2`, channelID, actorID).Scan(&adminJSON, &bannedJSON); err == sql.ErrNoRows {
			return ErrNotCreator
		} else if err != nil {
			return err
		}
		var admin ChannelAdminRights
		if adminJSON == "" || json.Unmarshal([]byte(adminJSON), &admin) != nil || !admin.ChangeInfo {
			return ErrNotCreator
		}
		var banned ChannelBannedRights
		if bannedJSON != "" && json.Unmarshal([]byte(bannedJSON), &banned) == nil && banned.Active(time.Now().Unix()) && banned.ChangeInfo {
			return ErrNotCreator
		}
	}
	if stickerSetID == 0 {
		if _, err = tx.Exec(`DELETE FROM apifull_channel_sticker_set WHERE channel_id=$1`, channelID); err != nil {
			return err
		}
	} else if _, err = tx.Exec(`INSERT INTO apifull_channel_sticker_set
			(channel_id,sticker_set_id,updated_by_user_id,updated_at) VALUES ($1,$2,$3,CURRENT_TIMESTAMP)
		ON CONFLICT (channel_id) DO UPDATE SET sticker_set_id=EXCLUDED.sticker_set_id,
		updated_by_user_id=EXCLUDED.updated_by_user_id, updated_at=CURRENT_TIMESTAMP`, channelID, stickerSetID, actorID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetChannelStickerSetAfterAuthorization persists a binding for the legacy
// Biz Chat path, whose channel roster and admin rights are owned by that
// service rather than apifull_channel.
func SetChannelStickerSetAfterAuthorization(actorID, channelID, stickerSetID int64) error {
	return setChannelStickerSetAfterAuthorization("apifull_channel_sticker_set", actorID, channelID, stickerSetID)
}

// SetChannelEmojiStickerSetAfterAuthorization stores the channel emoji set
// separately from the regular sticker-set binding exposed by channelFull.
func SetChannelEmojiStickerSetAfterAuthorization(actorID, channelID, stickerSetID int64) error {
	return setChannelStickerSetAfterAuthorization("apifull_channel_emoji_sticker_set", actorID, channelID, stickerSetID)
}

func setChannelStickerSetAfterAuthorization(table string, actorID, channelID, stickerSetID int64) error {
	if db == nil {
		return errors.New("domain PostgreSQL is not open")
	}
	if actorID <= 0 || channelID <= 0 || stickerSetID < 0 {
		return ErrInvalidChannelMember
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if stickerSetID == 0 {
		if _, err = tx.Exec(`DELETE FROM `+table+` WHERE channel_id=$1`, channelID); err != nil {
			return err
		}
	} else {
		var existingSetID int64
		setQuery := `SELECT id FROM apifull_sticker_set WHERE id=$1 FOR SHARE`
		if table == "apifull_channel_emoji_sticker_set" {
			setQuery = `SELECT id FROM apifull_sticker_set WHERE id=$1 AND emojis=TRUE FOR SHARE`
		}
		if err = tx.QueryRow(setQuery, stickerSetID).Scan(&existingSetID); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO `+table+`
			(channel_id,sticker_set_id,updated_by_user_id,updated_at) VALUES ($1,$2,$3,CURRENT_TIMESTAMP)
		ON CONFLICT (channel_id) DO UPDATE SET sticker_set_id=EXCLUDED.sticker_set_id,
		updated_by_user_id=EXCLUDED.updated_by_user_id, updated_at=CURRENT_TIMESTAMP`, channelID, stickerSetID, actorID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func LoadChannelStickerSet(channelID int64) (int64, bool, error) {
	return loadChannelStickerSet("apifull_channel_sticker_set", channelID)
}

func LoadChannelEmojiStickerSet(channelID int64) (int64, bool, error) {
	return loadChannelStickerSet("apifull_channel_emoji_sticker_set", channelID)
}

func loadChannelStickerSet(table string, channelID int64) (int64, bool, error) {
	if db == nil {
		return 0, false, errors.New("domain PostgreSQL is not open")
	}
	var setID int64
	err := db.QueryRow(`SELECT sticker_set_id FROM `+table+` WHERE channel_id=$1`, channelID).Scan(&setID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return setID, true, nil
}
