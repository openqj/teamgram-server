package postgres_dao

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

type UserNotifySettingsDAO struct{ db DB }

func NewUserNotifySettingsDAO(db DB) *UserNotifySettingsDAO { return &UserNotifySettingsDAO{db: db} }

const notifyCols = `id,user_id,peer_type,peer_id,show_previews,silent,mute_until,sound,deleted`

func scanNotify(row interface{ Scan(...any) error }) (*dataobject.UserNotifySettingsDO, error) {
	var v dataobject.UserNotifySettingsDO
	if e := row.Scan(&v.Id, &v.UserId, &v.PeerType, &v.PeerId, &v.ShowPreviews, &v.Silent, &v.MuteUntil, &v.Sound, &v.Deleted); e != nil {
		return nil, e
	}
	return &v, nil
}
func (d *UserNotifySettingsDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserNotifySettingsDO) (int64, int64, error) {
	return d.upsertNotify(ctx, d.db, v)
}
func (d *UserNotifySettingsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserNotifySettingsDO) (int64, int64, error) {
	return d.upsertNotify(ctx, tx, v)
}
func (d *UserNotifySettingsDAO) upsertNotify(ctx context.Context, db DB, v *dataobject.UserNotifySettingsDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_notify_settings(user_id,peer_type,peer_id,show_previews,silent,mute_until,sound,deleted) VALUES($1,$2,$3,$4,$5,$6,$7,FALSE) ON CONFLICT(user_id,peer_type,peer_id) DO UPDATE SET show_previews=EXCLUDED.show_previews,silent=EXCLUDED.silent,mute_until=EXCLUDED.mute_until,sound=EXCLUDED.sound,deleted=FALSE RETURNING id`, v.UserId, v.PeerType, v.PeerId, v.ShowPreviews, v.Silent, v.MuteUntil, v.Sound).Scan(&v.Id)
	return v.Id, 1, e
}
func (d *UserNotifySettingsDAO) SelectAll(ctx context.Context, u int64) ([]dataobject.UserNotifySettingsDO, error) {
	rows, e := d.db.Query(ctx, `SELECT `+notifyCols+` FROM user_notify_settings WHERE user_id=$1 AND deleted=FALSE ORDER BY peer_type,peer_id`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := []dataobject.UserNotifySettingsDO{}
	for rows.Next() {
		var v dataobject.UserNotifySettingsDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.PeerType, &v.PeerId, &v.ShowPreviews, &v.Silent, &v.MuteUntil, &v.Sound, &v.Deleted); e != nil {
			return nil, e
		}
		o = append(o, v)
	}
	return o, rows.Err()
}
func (d *UserNotifySettingsDAO) SelectAllWithCB(ctx context.Context, u int64, cb func(int, int, *dataobject.UserNotifySettingsDO)) ([]dataobject.UserNotifySettingsDO, error) {
	o, e := d.SelectAll(ctx, u)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}

// SelectListWithCB preserves the generated API used by user.getNotifySettingsList
// while reading each requested peer from PostgreSQL.
func (d *UserNotifySettingsDAO) SelectListWithCB(ctx context.Context, u int64, peers []*mtproto.PeerUtil, cb func(int, *dataobject.UserNotifySettingsDO)) ([]dataobject.UserNotifySettingsDO, error) {
	result := make([]dataobject.UserNotifySettingsDO, 0, len(peers))
	for _, peer := range peers {
		if peer == nil {
			continue
		}
		value, err := d.Select(ctx, u, peer.PeerType, peer.PeerId)
		if err != nil {
			return nil, err
		}
		if value == nil {
			continue
		}
		result = append(result, *value)
		if cb != nil {
			cb(len(result)-1, value)
		}
	}
	return result, nil
}
func (d *UserNotifySettingsDAO) Select(ctx context.Context, u int64, pt int32, pid int64) (*dataobject.UserNotifySettingsDO, error) {
	v, e := scanNotify(d.db.QueryRow(ctx, `SELECT `+notifyCols+` FROM user_notify_settings WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND deleted=FALSE`, u, pt, pid))
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return v, e
}
func (d *UserNotifySettingsDAO) DeleteAll(ctx context.Context, u int64) (int64, error) {
	return d.deleteAll(ctx, d.db, u)
}
func (d *UserNotifySettingsDAO) DeleteAllTx(ctx context.Context, tx DB, u int64) (int64, error) {
	return d.deleteAll(ctx, tx, u)
}
func (d *UserNotifySettingsDAO) deleteAll(ctx context.Context, db DB, u int64) (int64, error) {
	t, e := db.Exec(ctx, `UPDATE user_notify_settings SET deleted=TRUE WHERE user_id=$1`, u)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}
func (d *UserNotifySettingsDAO) InsertOrUpdateExt(ctx context.Context, u int64, pt int32, pid int64, m map[string]any) (int64, int64, error) {
	return d.ext(ctx, d.db, u, pt, pid, m)
}
func (d *UserNotifySettingsDAO) InsertOrUpdateExtTx(ctx context.Context, tx DB, u int64, pt int32, pid int64, m map[string]any) (int64, int64, error) {
	return d.ext(ctx, tx, u, pt, pid, m)
}
func (d *UserNotifySettingsDAO) ext(ctx context.Context, db DB, u int64, pt int32, pid int64, m map[string]any) (int64, int64, error) {
	if len(m) == 0 {
		return 0, 0, nil
	}
	allowed := map[string]bool{"show_previews": true, "silent": true, "mute_until": true, "sound": true, "deleted": true}
	cols := []string{"user_id", "peer_type", "peer_id"}
	ph := []string{"$1", "$2", "$3"}
	args := []any{u, pt, pid}
	for k, v := range m {
		if !allowed[k] {
			return 0, 0, fmt.Errorf("unsupported user_notify_settings column %q", k)
		}
		cols = append(cols, k)
		args = append(args, v)
		ph = append(ph, fmt.Sprintf("$%d", len(args)))
	}
	updates := []string{}
	for _, c := range cols[3:] {
		updates = append(updates, fmt.Sprintf("%s=EXCLUDED.%s", c, c))
	}
	q := `INSERT INTO user_notify_settings(` + strings.Join(cols, ",") + ") VALUES (" + strings.Join(ph, ",") + ") ON CONFLICT(user_id,peer_type,peer_id) DO UPDATE SET " + strings.Join(updates, ",") + " RETURNING id"
	var id int64
	e := db.QueryRow(ctx, q, args...).Scan(&id)
	return id, 1, e
}

type UserGlobalPrivacySettingsDAO struct{ db DB }

func NewUserGlobalPrivacySettingsDAO(db DB) *UserGlobalPrivacySettingsDAO {
	return &UserGlobalPrivacySettingsDAO{db: db}
}
func (d *UserGlobalPrivacySettingsDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserGlobalPrivacySettingsDO) (int64, int64, error) {
	return d.upsertGlobal(ctx, d.db, v)
}
func (d *UserGlobalPrivacySettingsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserGlobalPrivacySettingsDO) (int64, int64, error) {
	return d.upsertGlobal(ctx, tx, v)
}
func (d *UserGlobalPrivacySettingsDAO) upsertGlobal(ctx context.Context, db DB, v *dataobject.UserGlobalPrivacySettingsDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_global_privacy_settings(user_id,archive_and_mute_new_noncontact_peers,keep_archived_unmuted,keep_archived_folders,hide_read_marks,new_noncontact_peers_require_premium,display_gifts_button,noncontact_peers_paid_stars,disallowed_gifts) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(user_id) DO UPDATE SET archive_and_mute_new_noncontact_peers=EXCLUDED.archive_and_mute_new_noncontact_peers,keep_archived_unmuted=EXCLUDED.keep_archived_unmuted,keep_archived_folders=EXCLUDED.keep_archived_folders,hide_read_marks=EXCLUDED.hide_read_marks,new_noncontact_peers_require_premium=EXCLUDED.new_noncontact_peers_require_premium,display_gifts_button=EXCLUDED.display_gifts_button,noncontact_peers_paid_stars=EXCLUDED.noncontact_peers_paid_stars,disallowed_gifts=EXCLUDED.disallowed_gifts RETURNING id`, v.UserId, v.ArchiveAndMuteNewNoncontactPeers, v.KeepArchivedUnmuted, v.KeepArchivedFolders, v.HideReadMarks, v.NewNoncontactPeersRequirePremium, v.DisplayGiftsButton, nullableInt64(v.NoncontactPeersPaidStars), nullableString(v.DisallowedGiftsJSON)).Scan(&v.Id)
	return v.Id, 1, e
}

func nullableInt64(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullableString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}
func (d *UserGlobalPrivacySettingsDAO) Select(ctx context.Context, u int64) (*dataobject.UserGlobalPrivacySettingsDO, error) {
	v := new(dataobject.UserGlobalPrivacySettingsDO)
	e := d.db.QueryRow(ctx, `SELECT id,user_id,archive_and_mute_new_noncontact_peers,keep_archived_unmuted,keep_archived_folders,hide_read_marks,new_noncontact_peers_require_premium,display_gifts_button,noncontact_peers_paid_stars,disallowed_gifts FROM user_global_privacy_settings WHERE user_id=$1`, u).Scan(&v.Id, &v.UserId, &v.ArchiveAndMuteNewNoncontactPeers, &v.KeepArchivedUnmuted, &v.KeepArchivedFolders, &v.HideReadMarks, &v.NewNoncontactPeersRequirePremium, &v.DisplayGiftsButton, &v.NoncontactPeersPaidStars, &v.DisallowedGiftsJSON)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return v, e
}

type UserProfilePhotosDAO struct{ db DB }

func NewUserProfilePhotosDAO(db DB) *UserProfilePhotosDAO { return &UserProfilePhotosDAO{db: db} }
func (d *UserProfilePhotosDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserProfilePhotosDO) (int64, int64, error) {
	return d.upsertPhoto(ctx, d.db, v)
}
func (d *UserProfilePhotosDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserProfilePhotosDO) (int64, int64, error) {
	return d.upsertPhoto(ctx, tx, v)
}
func (d *UserProfilePhotosDAO) upsertPhoto(ctx context.Context, db DB, v *dataobject.UserProfilePhotosDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_profile_photos(user_id,photo_id,date2,deleted) VALUES($1,$2,$3,FALSE) ON CONFLICT(user_id,photo_id) DO UPDATE SET date2=EXCLUDED.date2,deleted=FALSE RETURNING id`, v.UserId, v.PhotoId, v.Date2).Scan(&v.Id)
	return v.Id, 1, e
}
func (d *UserProfilePhotosDAO) SelectList(ctx context.Context, u int64) ([]int64, error) {
	rows, e := d.db.Query(ctx, `SELECT photo_id FROM user_profile_photos WHERE user_id=$1 AND deleted=FALSE ORDER BY date2 DESC, id DESC`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		o = append(o, id)
	}
	return o, rows.Err()
}
func (d *UserProfilePhotosDAO) SelectListWithCB(ctx context.Context, u int64, cb func(int, int, int64)) ([]int64, error) {
	o, e := d.SelectList(ctx, u)
	if e == nil && cb != nil {
		for i, v := range o {
			cb(len(o), i, v)
		}
	}
	return o, e
}
func (d *UserProfilePhotosDAO) SelectNext(ctx context.Context, u int64, ids []int64) (int64, error) {
	var id int64
	var e error
	if len(ids) == 0 {
		e = d.db.QueryRow(ctx, `SELECT photo_id FROM user_profile_photos WHERE user_id=$1 AND deleted=FALSE ORDER BY date2 DESC, id DESC LIMIT 1`, u).Scan(&id)
	} else {
		e = d.db.QueryRow(ctx, `SELECT photo_id FROM user_profile_photos WHERE user_id=$1 AND photo_id <> ALL($2::bigint[]) AND deleted=FALSE ORDER BY date2 DESC, id DESC LIMIT 1`, u, ids).Scan(&id)
	}
	if e == pgx.ErrNoRows {
		return 0, nil
	}
	return id, e
}
func (d *UserProfilePhotosDAO) Delete(ctx context.Context, u int64, ids []int64) (int64, error) {
	return d.deletePhoto(ctx, d.db, u, ids)
}
func (d *UserProfilePhotosDAO) DeleteTx(ctx context.Context, tx DB, u int64, ids []int64) (int64, error) {
	return d.deletePhoto(ctx, tx, u, ids)
}
func (d *UserProfilePhotosDAO) deletePhoto(ctx context.Context, db DB, u int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	t, e := db.Exec(ctx, `UPDATE user_profile_photos SET deleted=TRUE WHERE user_id=$1 AND photo_id=ANY($2::bigint[])`, u, ids)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}

type UserSavedMusicDAO struct{ db DB }

func NewUserSavedMusicDAO(db DB) *UserSavedMusicDAO { return &UserSavedMusicDAO{db: db} }
func (d *UserSavedMusicDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserSavedMusicDO) (int64, int64, error) {
	return d.upsertMusic(ctx, d.db, v)
}
func (d *UserSavedMusicDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserSavedMusicDO) (int64, int64, error) {
	return d.upsertMusic(ctx, tx, v)
}
func (d *UserSavedMusicDAO) upsertMusic(ctx context.Context, db DB, v *dataobject.UserSavedMusicDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_saved_music(user_id,saved_music_id,order2,deleted) VALUES($1,$2,$3,FALSE) ON CONFLICT(user_id,saved_music_id) DO UPDATE SET order2=EXCLUDED.order2,deleted=FALSE RETURNING id`, v.UserId, v.SavedMusicId, v.Order2).Scan(&v.Id)
	return int64(v.Id), 1, e
}
func (d *UserSavedMusicDAO) SelectList(ctx context.Context, u int64) ([]dataobject.UserSavedMusicDO, error) {
	return d.selectMusic(ctx, u, nil)
}
func (d *UserSavedMusicDAO) SelectListTx(ctx context.Context, tx DB, u int64) ([]dataobject.UserSavedMusicDO, error) {
	return d.selectMusicOn(ctx, tx, u, nil)
}
func (d *UserSavedMusicDAO) SelectListWithCB(ctx context.Context, u int64, cb func(int, int, *dataobject.UserSavedMusicDO)) ([]dataobject.UserSavedMusicDO, error) {
	o, e := d.SelectList(ctx, u)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}
func (d *UserSavedMusicDAO) SelectListByIdList(ctx context.Context, u int64, ids []int64) ([]dataobject.UserSavedMusicDO, error) {
	if len(ids) == 0 {
		return []dataobject.UserSavedMusicDO{}, nil
	}
	return d.selectMusic(ctx, u, ids)
}
func (d *UserSavedMusicDAO) SelectListByIdListWithCB(ctx context.Context, u int64, ids []int64, cb func(int, int, *dataobject.UserSavedMusicDO)) ([]dataobject.UserSavedMusicDO, error) {
	o, e := d.SelectListByIdList(ctx, u, ids)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}
func (d *UserSavedMusicDAO) selectMusic(ctx context.Context, u int64, ids []int64) ([]dataobject.UserSavedMusicDO, error) {
	return d.selectMusicOn(ctx, d.db, u, ids)
}
func (d *UserSavedMusicDAO) selectMusicOn(ctx context.Context, db DB, u int64, ids []int64) ([]dataobject.UserSavedMusicDO, error) {
	q := `SELECT id,user_id,saved_music_id,order2,deleted FROM user_saved_music WHERE user_id=$1 AND deleted=FALSE`
	args := []any{u}
	if ids != nil {
		q += ` AND saved_music_id=ANY($2::bigint[])`
		args = append(args, ids)
	}
	q += ` ORDER BY order2,id`
	rows, e := db.Query(ctx, q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := []dataobject.UserSavedMusicDO{}
	for rows.Next() {
		var v dataobject.UserSavedMusicDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.SavedMusicId, &v.Order2, &v.Deleted); e != nil {
			return nil, e
		}
		o = append(o, v)
	}
	return o, rows.Err()
}
func (d *UserSavedMusicDAO) Delete(ctx context.Context, u, mid int64) (int64, error) {
	return d.deleteMusic(ctx, d.db, u, mid)
}
func (d *UserSavedMusicDAO) DeleteTx(ctx context.Context, tx DB, u, mid int64) (int64, error) {
	return d.deleteMusic(ctx, tx, u, mid)
}
func (d *UserSavedMusicDAO) deleteMusic(ctx context.Context, db DB, u, mid int64) (int64, error) {
	t, e := db.Exec(ctx, `UPDATE user_saved_music SET deleted=TRUE WHERE user_id=$1 AND saved_music_id=$2`, u, mid)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}
