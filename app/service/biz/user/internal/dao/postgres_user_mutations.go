package dao

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func postgresDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (d *Dao) pgCreateNewUser(ctx context.Context, do *dataobject.UsersDO, lastSeenAt int64, expires int32) error {
	if d.Postgres == nil || d.Postgres.Store == nil || d.Postgres.Store.Users == nil {
		return errors.New("biz/user: postgres store is not configured")
	}
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, _, err := d.Postgres.Store.Users.InsertTx(ctx, tx, do); err != nil {
			if postgresDuplicate(err) {
				return mtproto.ErrPhoneNumberOccupied
			}
			return err
		}
		_, _, err := d.Postgres.Store.Presences.InsertOrUpdateTx(ctx, tx, &dataobject.UserPresencesDO{
			UserId: do.Id, LastSeenAt: lastSeenAt, Expires: expires,
		})
		return err
	})
}

func (d *Dao) pgUpdateUsername(ctx context.Context, id int64, username string) error {
	if d.Postgres == nil || d.Postgres.Store == nil || d.Postgres.Store.Users == nil || d.Postgres.Store.Username == nil {
		return errors.New("biz/user: postgres store is not configured")
	}
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT username FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&current); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		if username != "" {
			var peerType int32
			var peerID int64
			err := tx.QueryRow(ctx, `SELECT peer_type,peer_id FROM username WHERE lower(username)=lower($1) AND deleted=FALSE FOR UPDATE`, username).Scan(&peerType, &peerID)
			if err == nil {
				if current != username || peerType != mtproto.PEER_USER || peerID != id {
					return mtproto.ErrUsernameOccupied
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			} else if _, err = tx.Exec(ctx, `INSERT INTO username(username,peer_type,peer_id,editable,active,order2,deleted) VALUES($1,$2,$3,TRUE,TRUE,$4,FALSE)`, username, mtproto.PEER_USER, id, time.Now().Unix()<<32); err != nil {
				if postgresDuplicate(err) {
					return mtproto.ErrUsernameOccupied
				}
				return err
			}
		}
		if current != "" && current != username {
			if _, err := tx.Exec(ctx, `DELETE FROM username WHERE lower(username)=lower($1) AND peer_type=$2 AND peer_id=$3`, current, mtproto.PEER_USER, id); err != nil {
				return err
			}
		}
		if current != username {
			if tag, err := d.Postgres.Store.Users.UpdateTx(ctx, tx, map[string]any{"username": username}, id); err != nil {
				return err
			} else if tag != 1 {
				return mtproto.ErrUserIdInvalid
			}
		}
		return nil
	})
	return err
}

func (d *Dao) pgUpdateProfilePhoto(ctx context.Context, userID, photoID int64) (int64, error) {
	store := d.Postgres.Store
	var mainID int64
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var current int64
		if err := tx.QueryRow(ctx, `SELECT photo_id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&current); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		mainID = photoID
		if photoID == 0 {
			if current != 0 {
				if _, err := store.ProfilePhotos.DeleteTx(ctx, tx, userID, []int64{current}); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `SELECT photo_id FROM user_profile_photos WHERE user_id=$1 AND photo_id<>$2 AND deleted=FALSE ORDER BY date2 DESC,id DESC LIMIT 1 FOR UPDATE`, userID, current).Scan(&mainID); errors.Is(err, pgx.ErrNoRows) {
					mainID = 0
				} else if err != nil {
					return err
				}
			}
		} else if _, _, err := store.ProfilePhotos.InsertOrUpdateTx(ctx, tx, &dataobject.UserProfilePhotosDO{UserId: userID, PhotoId: photoID, Date2: time.Now().Unix()}); err != nil {
			return err
		}
		_, err := store.Users.UpdateTx(ctx, tx, map[string]any{"photo_id": mainID}, userID)
		return err
	})
	return mainID, err
}

func (d *Dao) pgDeleteProfilePhotos(ctx context.Context, userID int64, photoIDs []int64) (int64, error) {
	store := d.Postgres.Store
	var mainID int64
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT photo_id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&mainID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		if len(photoIDs) == 0 {
			return nil
		}
		if _, err := store.ProfilePhotos.DeleteTx(ctx, tx, userID, photoIDs); err != nil {
			return err
		}
		for _, photoID := range photoIDs {
			if photoID == mainID {
				mainID = 0
				if err := tx.QueryRow(ctx, `SELECT photo_id FROM user_profile_photos WHERE user_id=$1 AND NOT (photo_id=ANY($2::bigint[])) AND deleted=FALSE ORDER BY date2 DESC,id DESC LIMIT 1 FOR UPDATE`, userID, photoIDs).Scan(&mainID); errors.Is(err, pgx.ErrNoRows) {
					mainID = 0
				} else if err != nil {
					return err
				}
				_, updateErr := store.Users.UpdateTx(ctx, tx, map[string]any{"photo_id": mainID}, userID)
				return updateErr
			}
		}
		return nil
	})
	return mainID, err
}

func (d *Dao) pgDeleteUser(ctx context.Context, id int64, phone, reason string) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM username WHERE peer_type=$1 AND peer_id=$2`, mtproto.PEER_USER, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM popular_contacts WHERE phone=$1`, phone); err != nil {
			return err
		}
		for _, query := range []string{
			`DELETE FROM user_contacts WHERE owner_user_id=$1 OR contact_user_id=$1`,
			`DELETE FROM imported_contacts WHERE user_id=$1 OR imported_user_id=$1`,
			`DELETE FROM unregistered_contacts WHERE importer_user_id=$1`,
			`DELETE FROM user_global_privacy_settings WHERE user_id=$1`,
			`DELETE FROM user_notify_settings WHERE user_id=$1`,
			`DELETE FROM user_peer_blocks WHERE user_id=$1`,
			`DELETE FROM user_peer_settings WHERE user_id=$1`,
			`DELETE FROM user_presences WHERE user_id=$1`,
			`DELETE FROM user_privacies WHERE user_id=$1`,
			`DELETE FROM user_profile_photos WHERE user_id=$1`,
			`DELETE FROM user_saved_music WHERE user_id=$1`,
			`DELETE FROM user_settings WHERE user_id=$1`,
			`DELETE FROM default_history_ttl WHERE user_id=$1`,
			`DELETE FROM bots WHERE creator_user_id=$1`,
		} {
			_, err := tx.Exec(ctx, query, id)
			if err != nil {
				return err
			}
		}
		rows, err := d.Postgres.Store.Users.DeleteTx(ctx, tx, "-"+toString(id), reason, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return mtproto.ErrUserIdInvalid
		}
		return nil
	})
}

func (d *Dao) pgUpdatePremium(ctx context.Context, id int64, premium bool, months int32) error {
	if !premium {
		_, err := d.UpdateUserFields(ctx, id, map[string]any{"premium": false, "premium_expire_date": int64(0)})
		return err
	}
	if months < 0 {
		return mtproto.ErrInputRequestInvalid
	}
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var currentPremium bool
		var currentExpiry int64
		if err := tx.QueryRow(ctx, `SELECT premium,premium_expire_date FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&currentPremium, &currentExpiry); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		var expiry int64
		if months != 0 && !(currentPremium && currentExpiry == 0) {
			base := time.Now()
			if currentPremium && currentExpiry > base.Unix() {
				base = time.Unix(currentExpiry, 0)
			}
			expiry = base.AddDate(0, int(months), 0).Unix()
		}
		_, err := tx.Exec(ctx, `UPDATE users SET premium=TRUE,premium_expire_date=$1 WHERE id=$2`, expiry, id)
		return err
	})
}

func (d *Dao) pgSaveMusic(ctx context.Context, userID, musicID int64, unsave bool) error {
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var lockedID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) && unsave {
				return nil
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}

		store := d.Postgres.Store
		if unsave {
			list, err := store.SavedMusic.SelectListTx(ctx, tx, userID)
			if err != nil {
				return err
			}
			for _, entry := range list {
				if musicID > 0 && entry.SavedMusicId == musicID {
					if _, err := store.SavedMusic.DeleteTx(ctx, tx, userID, musicID); err != nil {
						return err
					}
					break
				}
			}
			list, err = store.SavedMusic.SelectListTx(ctx, tx, userID)
			if err != nil {
				return err
			}
			nextID := int64(0)
			if len(list) > 0 {
				nextID = list[0].SavedMusicId
			}
			_, err = store.Users.UpdateTx(ctx, tx, map[string]any{"saved_music_id": nextID}, userID)
			return err
		}

		if _, _, err := store.SavedMusic.InsertOrUpdateTx(ctx, tx, &dataobject.UserSavedMusicDO{
			UserId: userID, SavedMusicId: musicID, Order2: time.Now().Unix() << 32,
		}); err != nil {
			return err
		}
		_, err := store.Users.UpdateTx(ctx, tx, map[string]any{"saved_music_id": musicID}, userID)
		return err
	})
}

func (d *Dao) pgGrantPremium(ctx context.Context, id int64, months int32, provider, transactionID string) error {
	transactionKey := sha256.Sum256([]byte(provider + "\x00" + transactionID))
	return d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var premium bool
		var expiry int64
		if err := tx.QueryRow(ctx, `SELECT premium,premium_expire_date FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&premium, &expiry); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		var inserted int64
		err := tx.QueryRow(ctx, `INSERT INTO user_premium_payment_grant(transaction_key,provider,transaction_id,user_id,months,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(transaction_key) DO NOTHING RETURNING 1`, transactionKey[:], provider, transactionID, id, months, time.Now().Unix()).Scan(&inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			var existingProvider, existingTransaction string
			var existingUser int64
			var existingMonths int32
			if err = tx.QueryRow(ctx, `SELECT provider,transaction_id,user_id,months FROM user_premium_payment_grant WHERE transaction_key=$1 FOR UPDATE`, transactionKey[:]).Scan(&existingProvider, &existingTransaction, &existingUser, &existingMonths); err != nil {
				return err
			}
			if existingProvider != provider || existingTransaction != transactionID || existingUser != id || existingMonths != months {
				return errPremiumPaymentConflict
			}
			return nil
		}
		if err != nil {
			return err
		}
		if inserted != 1 || (premium && expiry == 0) {
			return nil
		}
		base := time.Now()
		if premium && expiry > base.Unix() {
			base = time.Unix(expiry, 0)
		}
		_, err = tx.Exec(ctx, `UPDATE users SET premium=TRUE,premium_expire_date=$1 WHERE id=$2`, base.AddDate(0, int(months), 0).Unix(), id)
		return err
	})
}

func (d *Dao) pgCreateBot(ctx context.Context, creatorUserID, managerBotID, managerAccessHash int64, name, username string, accessHash int64, phone string) (int64, error) {
	var botID int64
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var creatorType int32
		var creatorDeleted bool
		if err := tx.QueryRow(ctx, `SELECT user_type,deleted FROM users WHERE id=$1 FOR UPDATE`, creatorUserID).Scan(&creatorType, &creatorDeleted); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrUserIdInvalid
			}
			return err
		}
		if creatorDeleted || creatorType != user.UserTypeRegular {
			return mtproto.ErrForbiddenUserBotInvalid
		}
		if managerBotID > 0 {
			var managerAccess int64
			var managerType int32
			var managerDeleted, canManage bool
			if err := tx.QueryRow(ctx, `SELECT u.access_hash,u.user_type,u.deleted,b.bot_can_manage_bots FROM bots b JOIN users u ON u.id=b.bot_id WHERE b.bot_id=$1 FOR UPDATE`, managerBotID).Scan(&managerAccess, &managerType, &managerDeleted, &canManage); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return mtproto.ErrForbiddenUserBotInvalid
				}
				return err
			}
			if managerAccess != managerAccessHash || managerType != user.UserTypeBot || managerDeleted || !canManage {
				return mtproto.ErrForbiddenUserBotInvalid
			}
		}
		var existing int64
		if err := tx.QueryRow(ctx, `SELECT 1 FROM username WHERE lower(username)=lower($1) AND deleted=FALSE`, username).Scan(&existing); err == nil {
			return mtproto.ErrUsernameOccupied
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		userDO := &dataobject.UsersDO{UserType: user.UserTypeBot, AccessHash: accessHash, FirstName: name, Username: username, Phone: phone, CountryCode: "BOT", IsBot: true}
		var err error
		botID, _, err = d.Postgres.Store.Users.InsertTx(ctx, tx, userDO)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO username(username,peer_type,peer_id,editable,active,order2,deleted) VALUES($1,$2,$3,TRUE,TRUE,$4,FALSE)`, username, mtproto.PEER_USER, botID, time.Now().Unix()<<32); err != nil {
			if postgresDuplicate(err) {
				return mtproto.ErrUsernameOccupied
			}
			return err
		}
		token, err := newBotToken(botID)
		if err != nil {
			return err
		}
		return d.Postgres.Store.Bots.InsertRegistryTx(ctx, tx, botID, creatorUserID, managerBotID, token)
	})
	if err != nil {
		return 0, err
	}
	return botID, nil
}

func (d *Dao) pgExportBotToken(ctx context.Context, botID, creatorUserID int64, revoke bool) (string, error) {
	var token string
	err := d.Postgres.InTx(ctx, func(tx pgx.Tx) error {
		var owner int64
		if err := tx.QueryRow(ctx, `SELECT creator_user_id,token FROM bots WHERE bot_id=$1 FOR UPDATE`, botID).Scan(&owner, &token); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return mtproto.ErrBotInvalid
			}
			return err
		}
		if owner != creatorUserID {
			return mtproto.ErrForbiddenUserBotInvalid
		}
		if revoke {
			var err error
			token, err = newBotToken(botID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE bots SET token=$1 WHERE bot_id=$2`, token, botID)
			return err
		}
		if token == "" {
			return mtproto.ErrTokenInvalid
		}
		return nil
	})
	return token, err
}

func toString(id int64) string {
	// Kept local to avoid pulling the legacy DAO formatting helpers into this
	// PostgreSQL-only mutation path.
	return fmt.Sprintf("%d", id)
}
