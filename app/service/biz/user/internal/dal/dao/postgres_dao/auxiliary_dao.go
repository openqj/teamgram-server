package postgres_dao

// PostgreSQL implementations for the user service's smaller aggregates. The
// methods intentionally retain the generated DAO call shapes so service code
// can migrate one aggregate at a time.
import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

func rowsAffected(tag interface{ RowsAffected() int64 }) int64 { return tag.RowsAffected() }

type UserPresencesDAO struct{ db DB }

func NewUserPresencesDAO(db DB) *UserPresencesDAO { return &UserPresencesDAO{db: db} }

const presenceCols = `id, user_id, last_seen_at, expires`

func scanPresence(row interface{ Scan(...any) error }) (*dataobject.UserPresencesDO, error) {
	var d dataobject.UserPresencesDO
	if err := row.Scan(&d.Id, &d.UserId, &d.LastSeenAt, &d.Expires); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}
func (d *UserPresencesDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserPresencesDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, v)
}
func (d *UserPresencesDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserPresencesDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, v)
}
func (d *UserPresencesDAO) insertOrUpdate(ctx context.Context, db DB, v *dataobject.UserPresencesDO) (int64, int64, error) {
	err := db.QueryRow(ctx, `INSERT INTO user_presences (user_id,last_seen_at,expires) VALUES ($1,$2,$3) ON CONFLICT (user_id) DO UPDATE SET last_seen_at=EXCLUDED.last_seen_at, expires=EXCLUDED.expires RETURNING id`, v.UserId, v.LastSeenAt, v.Expires).Scan(&v.Id)
	return v.Id, 1, err
}
func (d *UserPresencesDAO) Select(ctx context.Context, id int64) (*dataobject.UserPresencesDO, error) {
	return scanPresence(d.db.QueryRow(ctx, `SELECT `+presenceCols+` FROM user_presences WHERE user_id=$1`, id))
}
func (d *UserPresencesDAO) SelectList(ctx context.Context, ids []int64) ([]dataobject.UserPresencesDO, error) {
	if len(ids) == 0 {
		return []dataobject.UserPresencesDO{}, nil
	}
	rows, e := d.db.Query(ctx, `SELECT `+presenceCols+` FROM user_presences WHERE user_id=ANY($1::bigint[]) ORDER BY user_id`, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []dataobject.UserPresencesDO{}
	for rows.Next() {
		var v dataobject.UserPresencesDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.LastSeenAt, &v.Expires); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d *UserPresencesDAO) SelectListWithCB(ctx context.Context, ids []int64, cb func(int, int, *dataobject.UserPresencesDO)) ([]dataobject.UserPresencesDO, error) {
	out, e := d.SelectList(ctx, ids)
	if e == nil && cb != nil {
		for i := range out {
			cb(len(out), i, &out[i])
		}
	}
	return out, e
}
func (d *UserPresencesDAO) UpdateLastSeenAt(ctx context.Context, lastSeen int64, expires int32, id int64) (int64, error) {
	tag, e := d.db.Exec(ctx, `UPDATE user_presences SET last_seen_at=$1, expires=$2 WHERE user_id=$3`, lastSeen, expires, id)
	if e != nil {
		return 0, e
	}
	return tag.RowsAffected(), nil
}
func (d *UserPresencesDAO) UpdateLastSeenAtTx(ctx context.Context, tx DB, lastSeen int64, expires int32, id int64) (int64, error) {
	tag, e := tx.Exec(ctx, `UPDATE user_presences SET last_seen_at=$1, expires=$2 WHERE user_id=$3`, lastSeen, expires, id)
	if e != nil {
		return 0, e
	}
	return tag.RowsAffected(), nil
}

type UserPrivaciesDAO struct{ db DB }

func NewUserPrivaciesDAO(db DB) *UserPrivaciesDAO { return &UserPrivaciesDAO{db: db} }
func scanPrivacy(row interface{ Scan(...any) error }) (*dataobject.UserPrivaciesDO, error) {
	var v dataobject.UserPrivaciesDO
	if e := row.Scan(&v.Id, &v.UserId, &v.KeyType, &v.Rules); e != nil {
		if e == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, e
	}
	return &v, nil
}
func (d *UserPrivaciesDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserPrivaciesDO) (int64, int64, error) {
	return d.insertPrivacy(ctx, d.db, v)
}
func (d *UserPrivaciesDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserPrivaciesDO) (int64, int64, error) {
	return d.insertPrivacy(ctx, tx, v)
}
func (d *UserPrivaciesDAO) insertPrivacy(ctx context.Context, db DB, v *dataobject.UserPrivaciesDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_privacies(user_id,key_type,rules) VALUES($1,$2,$3) ON CONFLICT(user_id,key_type) DO UPDATE SET rules=EXCLUDED.rules RETURNING id`, v.UserId, v.KeyType, v.Rules).Scan(&v.Id)
	return v.Id, 1, e
}
func (d *UserPrivaciesDAO) InsertBulk(ctx context.Context, vs []*dataobject.UserPrivaciesDO) (int64, int64, error) {
	return d.insertPrivacyBulk(ctx, d.db, vs)
}
func (d *UserPrivaciesDAO) InsertBulkTx(ctx context.Context, tx DB, vs []*dataobject.UserPrivaciesDO) (int64, int64, error) {
	return d.insertPrivacyBulk(ctx, tx, vs)
}
func (d *UserPrivaciesDAO) insertPrivacyBulk(ctx context.Context, db DB, vs []*dataobject.UserPrivaciesDO) (int64, int64, error) {
	for _, v := range vs {
		if _, _, e := d.insertPrivacy(ctx, db, v); e != nil {
			return 0, 0, e
		}
	}
	return 0, int64(len(vs)), nil
}
func (d *UserPrivaciesDAO) SelectPrivacy(ctx context.Context, u int64, k int32) (*dataobject.UserPrivaciesDO, error) {
	return scanPrivacy(d.db.QueryRow(ctx, `SELECT id,user_id,key_type,rules FROM user_privacies WHERE user_id=$1 AND key_type=$2`, u, k))
}
func (d *UserPrivaciesDAO) SelectPrivacyList(ctx context.Context, u int64, ks []int32) ([]dataobject.UserPrivaciesDO, error) {
	if len(ks) == 0 {
		return []dataobject.UserPrivaciesDO{}, nil
	}
	rows, e := d.db.Query(ctx, `SELECT id,user_id,key_type,rules FROM user_privacies WHERE user_id=$1 AND key_type=ANY($2::int[]) ORDER BY key_type`, u, ks)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []dataobject.UserPrivaciesDO{}
	for rows.Next() {
		var v dataobject.UserPrivaciesDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.KeyType, &v.Rules); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d *UserPrivaciesDAO) SelectPrivacyListWithCB(ctx context.Context, u int64, ks []int32, cb func(int, int, *dataobject.UserPrivaciesDO)) ([]dataobject.UserPrivaciesDO, error) {
	o, e := d.SelectPrivacyList(ctx, u, ks)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}
func (d *UserPrivaciesDAO) SelectUsersPrivacyList(ctx context.Context, ids, ks []int32) ([]dataobject.UserPrivaciesDO, error) {
	if len(ids) == 0 || len(ks) == 0 {
		return []dataobject.UserPrivaciesDO{}, nil
	}
	rows, e := d.db.Query(ctx, `SELECT id,user_id,key_type,rules FROM user_privacies WHERE user_id=ANY($1::int[]) AND key_type=ANY($2::int[]) ORDER BY user_id,key_type`, ids, ks)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []dataobject.UserPrivaciesDO{}
	for rows.Next() {
		var v dataobject.UserPrivaciesDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.KeyType, &v.Rules); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d *UserPrivaciesDAO) SelectUsersPrivacyListWithCB(ctx context.Context, ids, ks []int32, cb func(int, int, *dataobject.UserPrivaciesDO)) ([]dataobject.UserPrivaciesDO, error) {
	o, e := d.SelectUsersPrivacyList(ctx, ids, ks)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}
func (d *UserPrivaciesDAO) SelectPrivacyAll(ctx context.Context, u int64) ([]dataobject.UserPrivaciesDO, error) {
	return d.SelectPrivacyListAll(ctx, u)
}
func (d *UserPrivaciesDAO) SelectPrivacyListAll(ctx context.Context, u int64) ([]dataobject.UserPrivaciesDO, error) {
	rows, e := d.db.Query(ctx, `SELECT id,user_id,key_type,rules FROM user_privacies WHERE user_id=$1 ORDER BY key_type`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := []dataobject.UserPrivaciesDO{}
	for rows.Next() {
		var v dataobject.UserPrivaciesDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.KeyType, &v.Rules); e != nil {
			return nil, e
		}
		o = append(o, v)
	}
	return o, rows.Err()
}
func (d *UserPrivaciesDAO) SelectPrivacyAllWithCB(ctx context.Context, u int64, cb func(int, int, *dataobject.UserPrivaciesDO)) ([]dataobject.UserPrivaciesDO, error) {
	o, e := d.SelectPrivacyAll(ctx, u)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}

type UserPeerBlocksDAO struct{ db DB }

func NewUserPeerBlocksDAO(db DB) *UserPeerBlocksDAO { return &UserPeerBlocksDAO{db: db} }
func (d *UserPeerBlocksDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserPeerBlocksDO) (int64, int64, error) {
	return d.upsertBlock(ctx, d.db, v)
}
func (d *UserPeerBlocksDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserPeerBlocksDO) (int64, int64, error) {
	return d.upsertBlock(ctx, tx, v)
}
func (d *UserPeerBlocksDAO) upsertBlock(ctx context.Context, db DB, v *dataobject.UserPeerBlocksDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_peer_blocks(user_id,peer_type,peer_id,date,deleted) VALUES($1,$2,$3,$4,FALSE) ON CONFLICT(user_id,peer_type,peer_id) DO UPDATE SET date=EXCLUDED.date,deleted=FALSE RETURNING id`, v.UserId, v.PeerType, v.PeerId, v.Date).Scan(&v.Id)
	return v.Id, 1, e
}
func (d *UserPeerBlocksDAO) SelectList(ctx context.Context, u int64, offset, limit int32) ([]dataobject.UserPeerBlocksDO, error) {
	rows, e := d.db.Query(ctx, `SELECT id,user_id,peer_type,peer_id,date,deleted FROM user_peer_blocks WHERE user_id=$1 AND deleted=FALSE ORDER BY id LIMIT $2 OFFSET $3`, u, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	o := []dataobject.UserPeerBlocksDO{}
	for rows.Next() {
		var v dataobject.UserPeerBlocksDO
		if e = rows.Scan(&v.Id, &v.UserId, &v.PeerType, &v.PeerId, &v.Date, &v.Deleted); e != nil {
			return nil, e
		}
		o = append(o, v)
	}
	return o, rows.Err()
}
func (d *UserPeerBlocksDAO) SelectListWithCB(ctx context.Context, u int64, off, lim int32, cb func(int, int, *dataobject.UserPeerBlocksDO)) ([]dataobject.UserPeerBlocksDO, error) {
	o, e := d.SelectList(ctx, u, off, lim)
	if e == nil && cb != nil {
		for i := range o {
			cb(len(o), i, &o[i])
		}
	}
	return o, e
}
func (d *UserPeerBlocksDAO) SelectCount(ctx context.Context, u int64) (int32, error) {
	var n int32
	e := d.db.QueryRow(ctx, `SELECT count(*) FROM user_peer_blocks WHERE user_id=$1 AND deleted=FALSE`, u).Scan(&n)
	return n, e
}
func (d *UserPeerBlocksDAO) SelectListByIdList(ctx context.Context, u int64, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	rows, e := d.db.Query(ctx, `SELECT peer_id FROM user_peer_blocks WHERE user_id=$1 AND peer_type=2 AND peer_id=ANY($2::bigint[]) AND deleted=FALSE ORDER BY id`, u, ids)
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
func (d *UserPeerBlocksDAO) SelectListByIdListWithCB(ctx context.Context, u int64, ids []int64, cb func(int, int, int64)) ([]int64, error) {
	o, e := d.SelectListByIdList(ctx, u, ids)
	if e == nil && cb != nil {
		for i, v := range o {
			cb(len(o), i, v)
		}
	}
	return o, e
}
func (d *UserPeerBlocksDAO) Select(ctx context.Context, u int64, pt int32, pid int64) (*dataobject.UserPeerBlocksDO, error) {
	var v dataobject.UserPeerBlocksDO
	e := d.db.QueryRow(ctx, `SELECT id,user_id,peer_type,peer_id,date,deleted FROM user_peer_blocks WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND deleted=FALSE`, u, pt, pid).Scan(&v.Id, &v.UserId, &v.PeerType, &v.PeerId, &v.Date, &v.Deleted)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, e
}
func (d *UserPeerBlocksDAO) Delete(ctx context.Context, u int64, pt int32, pid int64) (int64, error) {
	return d.delete(ctx, d.db, u, pt, pid)
}
func (d *UserPeerBlocksDAO) DeleteTx(ctx context.Context, tx DB, u int64, pt int32, pid int64) (int64, error) {
	return d.delete(ctx, tx, u, pt, pid)
}
func (d *UserPeerBlocksDAO) delete(ctx context.Context, db DB, u int64, pt int32, pid int64) (int64, error) {
	t, e := db.Exec(ctx, `UPDATE user_peer_blocks SET deleted=TRUE,date=0 WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, u, pt, pid)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}

type UserSettingsDAO struct{ db DB }

func NewUserSettingsDAO(db DB) *UserSettingsDAO { return &UserSettingsDAO{db: db} }
func (d *UserSettingsDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserSettingsDO) (int64, int64, error) {
	return d.upsertSetting(ctx, d.db, v)
}
func (d *UserSettingsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserSettingsDO) (int64, int64, error) {
	return d.upsertSetting(ctx, tx, v)
}
func (d *UserSettingsDAO) upsertSetting(ctx context.Context, db DB, v *dataobject.UserSettingsDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_settings(user_id,key2,value,deleted) VALUES($1,$2,$3,FALSE) ON CONFLICT(user_id,key2) DO UPDATE SET value=EXCLUDED.value,deleted=FALSE RETURNING id`, v.UserId, v.Key2, v.Value).Scan(&v.Id)
	return v.Id, 1, e
}
func (d *UserSettingsDAO) SelectByKey(ctx context.Context, u int64, k string) (*dataobject.UserSettingsDO, error) {
	var v dataobject.UserSettingsDO
	e := d.db.QueryRow(ctx, `SELECT id,user_id,key2,value,deleted FROM user_settings WHERE user_id=$1 AND key2=$2 AND deleted=FALSE`, u, k).Scan(&v.Id, &v.UserId, &v.Key2, &v.Value, &v.Deleted)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, e
}
func (d *UserSettingsDAO) Update(ctx context.Context, val string, u int64, k string) (int64, error) {
	return d.update(ctx, d.db, val, u, k)
}
func (d *UserSettingsDAO) UpdateTx(ctx context.Context, tx DB, val string, u int64, k string) (int64, error) {
	return d.update(ctx, tx, val, u, k)
}
func (d *UserSettingsDAO) update(ctx context.Context, db DB, val string, u int64, k string) (int64, error) {
	t, e := db.Exec(ctx, `UPDATE user_settings SET value=$1,deleted=FALSE WHERE user_id=$2 AND key2=$3`, val, u, k)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}

// User peer settings and notification settings use the same explicit upsert
// pattern. Dynamic update maps are constrained to known columns.
type UserPeerSettingsDAO struct{ db DB }

func NewUserPeerSettingsDAO(db DB) *UserPeerSettingsDAO { return &UserPeerSettingsDAO{db: db} }

const peerSettingCols = `id,user_id,peer_type,peer_id,hide,report_spam,add_contact,block_contact,share_contact,need_contacts_exception,report_geo,autoarchived,invite_members,geo_distance`

func (d *UserPeerSettingsDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UserPeerSettingsDO) (int64, int64, error) {
	return d.upsertPeer(ctx, d.db, v)
}
func (d *UserPeerSettingsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UserPeerSettingsDO) (int64, int64, error) {
	return d.upsertPeer(ctx, tx, v)
}
func (d *UserPeerSettingsDAO) upsertPeer(ctx context.Context, db DB, v *dataobject.UserPeerSettingsDO) (int64, int64, error) {
	e := db.QueryRow(ctx, `INSERT INTO user_peer_settings(user_id,peer_type,peer_id,hide,report_spam,add_contact,block_contact,share_contact,need_contacts_exception,report_geo,autoarchived,invite_members,geo_distance) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(user_id,peer_type,peer_id) DO UPDATE SET hide=EXCLUDED.hide,report_spam=EXCLUDED.report_spam,add_contact=EXCLUDED.add_contact,block_contact=EXCLUDED.block_contact,share_contact=EXCLUDED.share_contact,need_contacts_exception=EXCLUDED.need_contacts_exception,report_geo=EXCLUDED.report_geo,autoarchived=EXCLUDED.autoarchived,invite_members=EXCLUDED.invite_members,geo_distance=EXCLUDED.geo_distance RETURNING id`, v.UserId, v.PeerType, v.PeerId, v.Hide, v.ReportSpam, v.AddContact, v.BlockContact, v.ShareContact, v.NeedContactsException, v.ReportGeo, v.Autoarchived, v.InviteMembers, v.GeoDistance).Scan(&v.Id)
	return v.Id, 1, e
}
func (d *UserPeerSettingsDAO) Select(ctx context.Context, u int64, pt int32, pid int64) (*dataobject.UserPeerSettingsDO, error) {
	var v dataobject.UserPeerSettingsDO
	e := d.db.QueryRow(ctx, `SELECT `+peerSettingCols+` FROM user_peer_settings WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, u, pt, pid).Scan(&v.Id, &v.UserId, &v.PeerType, &v.PeerId, &v.Hide, &v.ReportSpam, &v.AddContact, &v.BlockContact, &v.ShareContact, &v.NeedContactsException, &v.ReportGeo, &v.Autoarchived, &v.InviteMembers, &v.GeoDistance)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, e
}
func (d *UserPeerSettingsDAO) Update(ctx context.Context, m map[string]any, u int64, pt int32, pid int64) (int64, error) {
	return d.updatePeer(ctx, d.db, m, u, pt, pid)
}
func (d *UserPeerSettingsDAO) UpdateTx(ctx context.Context, tx DB, m map[string]any, u int64, pt int32, pid int64) (int64, error) {
	return d.updatePeer(ctx, tx, m, u, pt, pid)
}
func (d *UserPeerSettingsDAO) updatePeer(ctx context.Context, db DB, m map[string]any, u int64, pt int32, pid int64) (int64, error) {
	allowed := map[string]bool{"hide": true, "report_spam": true, "add_contact": true, "block_contact": true, "share_contact": true, "need_contacts_exception": true, "report_geo": true, "autoarchived": true, "invite_members": true, "geo_distance": true}
	set := []string{}
	args := []any{}
	for k, v := range m {
		if !allowed[k] {
			return 0, fmt.Errorf("unsupported user_peer_settings column %q", k)
		}
		args = append(args, v)
		set = append(set, fmt.Sprintf("%s=$%d", k, len(args)))
	}
	if len(set) == 0 {
		return 0, nil
	}
	args = append(args, u, pt, pid)
	t, e := db.Exec(ctx, `UPDATE user_peer_settings SET `+strings.Join(set, ",")+fmt.Sprintf(" WHERE user_id=$%d AND peer_type=$%d AND peer_id=$%d", len(args)-2, len(args)-1, len(args)), args...)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}
func (d *UserPeerSettingsDAO) Delete(ctx context.Context, u int64, pt int32, pid int64) (int64, error) {
	t, e := d.db.Exec(ctx, `DELETE FROM user_peer_settings WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, u, pt, pid)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}
func (d *UserPeerSettingsDAO) DeleteTx(ctx context.Context, tx DB, u int64, pt int32, pid int64) (int64, error) {
	t, e := tx.Exec(ctx, `DELETE FROM user_peer_settings WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3`, u, pt, pid)
	if e != nil {
		return 0, e
	}
	return t.RowsAffected(), nil
}
