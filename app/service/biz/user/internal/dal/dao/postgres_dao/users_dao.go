package postgres_dao

import (
	"context"
	"fmt"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

type UsersDAO struct{ db DB }

func NewUsersDAO(db DB) *UsersDAO { return &UsersDAO{db: db} }

func (d *UsersDAO) Insert(ctx context.Context, do *dataobject.UsersDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO users
 (user_type, access_hash, secret_key_id, first_name, last_name, username, phone, country_code, verified, about, is_bot)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		do.UserType, do.AccessHash, do.SecretKeyId, do.FirstName, do.LastName,
		do.Username, do.Phone, do.CountryCode, do.Verified, do.About, do.IsBot).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *UsersDAO) InsertTestUser(ctx context.Context, do *dataobject.UsersDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO users
 (id, user_type, access_hash, secret_key_id, first_name, last_name, username, phone, country_code, verified, about, is_bot)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		do.Id, do.UserType, do.AccessHash, do.SecretKeyId, do.FirstName, do.LastName,
		do.Username, do.Phone, do.CountryCode, do.Verified, do.About, do.IsBot).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *UsersDAO) SelectByID(ctx context.Context, id int64) (*dataobject.UsersDO, error) {
	return scanUser(d.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// SelectByIDOn reads a user through an existing PostgreSQL transaction. It is
// used by compound mutations that must validate and update the same user row
// atomically.
func (d *UsersDAO) SelectByIDOn(ctx context.Context, db DB, id int64) (*dataobject.UsersDO, error) {
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1 FOR UPDATE`, id))
}

func (d *UsersDAO) SelectById(ctx context.Context, id int64) (*dataobject.UsersDO, error) {
	return d.SelectByID(ctx, id)
}

func (d *UsersDAO) SelectByPhoneNumber(ctx context.Context, phone string) (*dataobject.UsersDO, error) {
	return scanUser(d.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE phone = $1 LIMIT 1`, phone))
}

func (d *UsersDAO) SelectByPhone(ctx context.Context, phone string) (*dataobject.UsersDO, error) {
	return d.SelectByPhoneNumber(ctx, phone)
}

func (d *UsersDAO) SelectUsersByIDList(ctx context.Context, ids []int64) ([]dataobject.UsersDO, error) {
	if len(ids) == 0 {
		return []dataobject.UsersDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return scanUserRows(rows)
}

func (d *UsersDAO) SelectUsersByIdList(ctx context.Context, ids []int64) ([]dataobject.UsersDO, error) {
	return d.SelectUsersByIDList(ctx, ids)
}

func (d *UsersDAO) SelectUsersByPhoneList(ctx context.Context, phones []string) ([]dataobject.UsersDO, error) {
	if len(phones) == 0 {
		return []dataobject.UsersDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE phone = ANY($1::text[]) ORDER BY id`, phones)
	if err != nil {
		return nil, err
	}
	return scanUserRows(rows)
}

func (d *UsersDAO) SelectNextTestUserID(ctx context.Context, maxID int64) (*dataobject.UsersDO, error) {
	return scanUser(d.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id < $1 ORDER BY id DESC LIMIT 1`, maxID))
}

func (d *UsersDAO) SelectNextTestUserId(ctx context.Context, maxID int64) (*dataobject.UsersDO, error) {
	return d.SelectNextTestUserID(ctx, maxID)
}

func (d *UsersDAO) Update(ctx context.Context, values map[string]any, id int64) (int64, error) {
	return d.update(ctx, d.db, values, id)
}

// UpdateTx applies a user update using the caller's transaction.
func (d *UsersDAO) UpdateTx(ctx context.Context, tx DB, values map[string]any, id int64) (int64, error) {
	return d.update(ctx, tx, values, id)
}

func (d *UsersDAO) update(ctx context.Context, db DB, values map[string]any, id int64) (int64, error) {
	allowed := map[string]bool{"first_name": true, "last_name": true, "username": true, "about": true,
		"verified": true, "support": true, "scam": true, "fake": true, "premium": true,
		"premium_expire_date": true, "state": true, "account_days_ttl": true, "photo_id": true,
		"restricted": true, "restriction_reason": true, "archive_and_mute_new_noncontact_peers": true,
		"emoji_status_document_id": true, "emoji_status_until": true, "stories_max_id": true,
		"color": true, "color_background_emoji_id": true, "profile_color": true,
		"profile_color_background_emoji_id": true, "birthday": true, "personal_channel_id": true,
		"authorization_ttl_days": true, "saved_music_id": true, "main_tab": true, "deleted": true,
		"delete_reason": true, "is_bot": true}
	sets := make([]string, 0, len(values))
	args := make([]any, 0, len(values)+1)
	for name, value := range values {
		if !allowed[name] {
			return 0, fmt.Errorf("unsupported users column %q", name)
		}
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", name, len(args)))
	}
	if len(sets) == 0 {
		return 0, nil
	}
	args = append(args, id)
	tag, err := db.Exec(ctx, `UPDATE users SET `+joinComma(sets)+` WHERE id = $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func joinComma(values []string) string {
	result := values[0]
	for _, value := range values[1:] {
		result += ", " + value
	}
	return result
}
