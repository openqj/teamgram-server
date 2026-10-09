// Package postgres_dao contains the PostgreSQL persistence boundary for the
// user service. It intentionally uses pgx directly so the new database path
// does not inherit MySQL placeholder or type semantics.
package postgres_dao

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Store struct {
	Pool           *pgxpool.Pool
	Users          *UsersDAO
	Contacts       *UserContactsDAO
	Username       *UsernameDAO
	Presences      *UserPresencesDAO
	Privacies      *UserPrivaciesDAO
	PeerBlocks     *UserPeerBlocksDAO
	PeerSettings   *UserPeerSettingsDAO
	Settings       *UserSettingsDAO
	NotifySettings *UserNotifySettingsDAO
	GlobalPrivacy  *UserGlobalPrivacySettingsDAO
	ProfilePhotos  *UserProfilePhotosDAO
	SavedMusic     *UserSavedMusicDAO
	Bots           *BotsDAO
	BotCommands    *BotCommandsDAO
	Imported       *ImportedContactsDAO
	Unregistered   *UnregisteredContactsDAO
	HistoryTTL     *DefaultHistoryTtlDAO
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool, Users: NewUsersDAO(pool), Contacts: NewUserContactsDAO(pool), Username: NewUsernameDAO(pool),
		Presences: NewUserPresencesDAO(pool), Privacies: NewUserPrivaciesDAO(pool), PeerBlocks: NewUserPeerBlocksDAO(pool),
		PeerSettings: NewUserPeerSettingsDAO(pool), Settings: NewUserSettingsDAO(pool), NotifySettings: NewUserNotifySettingsDAO(pool),
		GlobalPrivacy: NewUserGlobalPrivacySettingsDAO(pool), ProfilePhotos: NewUserProfilePhotosDAO(pool), SavedMusic: NewUserSavedMusicDAO(pool),
		Bots: NewBotsDAO(pool), BotCommands: NewBotCommandsDAO(pool), Imported: NewImportedContactsDAO(pool), Unregistered: NewUnregisteredContactsDAO(pool), HistoryTTL: NewDefaultHistoryTtlDAO(pool)}
}

func scanUser(row pgx.Row) (*dataobject.UsersDO, error) {
	do := new(dataobject.UsersDO)
	err := row.Scan(userScanArgs(do)...)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanUserRows(rows pgx.Rows) ([]dataobject.UsersDO, error) {
	defer rows.Close()
	result := make([]dataobject.UsersDO, 0)
	for rows.Next() {
		var do dataobject.UsersDO
		if err := rows.Scan(userScanArgs(&do)...); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func userScanArgs(do *dataobject.UsersDO) []any {
	return []any{&do.Id, &do.UserType, &do.AccessHash, &do.SecretKeyId,
		&do.FirstName, &do.LastName, &do.Username, &do.Phone, &do.CountryCode,
		&do.Verified, &do.Support, &do.Scam, &do.Fake, &do.Premium,
		&do.PremiumExpireDate, &do.About, &do.State, &do.IsBot, &do.AccountDaysTtl,
		&do.PhotoId, &do.Restricted, &do.RestrictionReason,
		&do.ArchiveAndMuteNewNoncontactPeers, &do.EmojiStatusDocumentId,
		&do.EmojiStatusUntil, &do.StoriesMaxId, &do.Color, &do.ColorBackgroundEmojiId,
		&do.ProfileColor, &do.ProfileColorBackgroundEmojiId, &do.Birthday,
		&do.PersonalChannelId, &do.AuthorizationTtlDays, &do.SavedMusicId,
		&do.MainTab, &do.Deleted, &do.DeleteReason}
}

const userColumns = `id, user_type, access_hash, secret_key_id, first_name,
last_name, username, phone, country_code, verified, support, scam, fake,
premium, premium_expire_date, about, state, is_bot, account_days_ttl, photo_id,
restricted, restriction_reason, archive_and_mute_new_noncontact_peers,
emoji_status_document_id, emoji_status_until, stories_max_id, color,
color_background_emoji_id, profile_color, profile_color_background_emoji_id,
birthday, personal_channel_id, authorization_ttl_days, saved_music_id, main_tab,
deleted, delete_reason`

func scanContact(row pgx.Row) (*dataobject.UserContactsDO, error) {
	do := new(dataobject.UserContactsDO)
	err := row.Scan(&do.Id, &do.OwnerUserId, &do.ContactUserId, &do.ContactPhone,
		&do.ContactFirstName, &do.ContactLastName, &do.Mutual, &do.CloseFriend,
		&do.StoriesHidden, &do.Date2, &do.IsDeleted)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanContactRows(rows pgx.Rows) ([]dataobject.UserContactsDO, error) {
	defer rows.Close()
	result := make([]dataobject.UserContactsDO, 0)
	for rows.Next() {
		var do dataobject.UserContactsDO
		if err := rows.Scan(&do.Id, &do.OwnerUserId, &do.ContactUserId, &do.ContactPhone,
			&do.ContactFirstName, &do.ContactLastName, &do.Mutual, &do.CloseFriend,
			&do.StoriesHidden, &do.Date2, &do.IsDeleted); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}
