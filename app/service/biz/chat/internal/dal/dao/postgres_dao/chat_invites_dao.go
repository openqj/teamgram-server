package postgres_dao

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

const inviteColumns = `id, chat_id, admin_id, migrated_to_id, link,
permanent, revoked, request_needed, start_date, expire_date, usage_limit,
usage2, requested, title, date2, state`

type ChatInvitesDAO struct{ db DB }

func NewChatInvitesDAO(db DB) *ChatInvitesDAO { return &ChatInvitesDAO{db: db} }

func scanInvite(row interface{ Scan(...any) error }) (*dataobject.ChatInvitesDO, error) {
	do := new(dataobject.ChatInvitesDO)
	err := row.Scan(&do.Id, &do.ChatId, &do.AdminId, &do.MigratedToId, &do.Link,
		&do.Permanent, &do.Revoked, &do.RequestNeeded, &do.StartDate, &do.ExpireDate,
		&do.UsageLimit, &do.Usage2, &do.Requested, &do.Title, &do.Date2, &do.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanInvites(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]dataobject.ChatInvitesDO, error) {
	defer rows.Close()
	list := make([]dataobject.ChatInvitesDO, 0)
	for rows.Next() {
		do, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *do)
	}
	return list, rows.Err()
}

func (d *ChatInvitesDAO) Insert(ctx context.Context, do *dataobject.ChatInvitesDO) (int64, int64, error) {
	return d.insertOn(ctx, d.db, do)
}

func (d *ChatInvitesDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.ChatInvitesDO) (int64, int64, error) {
	return d.insertOn(ctx, tx, do)
}

func (d *ChatInvitesDAO) insertOn(ctx context.Context, db DB, do *dataobject.ChatInvitesDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO chat_invites
 (chat_id,admin_id,migrated_to_id,link,permanent,revoked,request_needed,
 start_date,expire_date,usage_limit,usage2,requested,title,date2,state)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
 RETURNING id`, do.ChatId, do.AdminId, do.MigratedToId, do.Link, do.Permanent,
		do.Revoked, do.RequestNeeded, do.StartDate, do.ExpireDate, do.UsageLimit,
		do.Usage2, do.Requested, do.Title, do.Date2, do.State).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *ChatInvitesDAO) SelectListByAdminId(ctx context.Context, chatID, adminID int64) ([]dataobject.ChatInvitesDO, error) {
	return d.selectList(ctx, `SELECT `+inviteColumns+` FROM chat_invites WHERE chat_id = $1 AND admin_id = $2 ORDER BY id`, chatID, adminID)
}

func (d *ChatInvitesDAO) SelectByLink(ctx context.Context, link string) (*dataobject.ChatInvitesDO, error) {
	return scanInvite(d.db.QueryRow(ctx, `SELECT `+inviteColumns+` FROM chat_invites WHERE link = $1`, link))
}

func (d *ChatInvitesDAO) SelectAll(ctx context.Context) ([]dataobject.ChatInvitesDO, error) {
	return d.selectList(ctx, `SELECT `+inviteColumns+` FROM chat_invites ORDER BY id`)
}

func (d *ChatInvitesDAO) SelectListByChatId(ctx context.Context, chatID int64) ([]dataobject.ChatInvitesDO, error) {
	return d.selectList(ctx, `SELECT `+inviteColumns+` FROM chat_invites WHERE chat_id = $1 ORDER BY id`, chatID)
}

func (d *ChatInvitesDAO) selectList(ctx context.Context, query string, args ...any) ([]dataobject.ChatInvitesDO, error) {
	rows, err := d.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanInvites(rows)
}

// updateFields is deliberately a whitelist. These keys originate in a TL
// edit request and must never be interpolated directly into SQL.
var updateFields = map[string]string{
	"revoked":        "revoked",
	"request_needed": "request_needed",
	"usage_limit":    "usage_limit",
	"title":          "title",
	"permanent":      "permanent",
	"expire_date":    "expire_date",
	"start_date":     "start_date",
}

func (d *ChatInvitesDAO) Update(ctx context.Context, values map[string]interface{}, chatID int64, link string) (int64, error) {
	return d.updateOn(ctx, d.db, values, chatID, link)
}

func (d *ChatInvitesDAO) UpdateOn(ctx context.Context, tx DB, values map[string]interface{}, chatID int64, link string) (int64, error) {
	return d.updateOn(ctx, tx, values, chatID, link)
}

func (d *ChatInvitesDAO) updateOn(ctx context.Context, db DB, values map[string]interface{}, chatID int64, link string) (int64, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if _, ok := updateFields[key]; !ok {
			return 0, fmt.Errorf("chat invite: unsupported update field %q", key)
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return 0, nil
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys)+2)
	sets := make([]string, 0, len(keys))
	for index, key := range keys {
		sets = append(sets, updateFields[key]+fmt.Sprintf(" = $%d", index+1))
		args = append(args, values[key])
	}
	args = append(args, chatID, link)
	query := fmt.Sprintf("UPDATE chat_invites SET %s WHERE chat_id = $%d AND link = $%d", joinComma(sets), len(keys)+1, len(keys)+2)
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return commandRows(tag), nil
}

func joinComma(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ", "
		}
		result += value
	}
	return result
}

func (d *ChatInvitesDAO) DeleteByLink(ctx context.Context, chatID int64, link string) (int64, error) {
	return d.deleteOn(ctx, d.db, `DELETE FROM chat_invites WHERE chat_id = $1 AND link = $2`, chatID, link)
}
func (d *ChatInvitesDAO) DeleteByLinkOn(ctx context.Context, tx DB, chatID int64, link string) (int64, error) {
	return d.deleteOn(ctx, tx, `DELETE FROM chat_invites WHERE chat_id = $1 AND link = $2`, chatID, link)
}
func (d *ChatInvitesDAO) DeleteByRevoked(ctx context.Context, chatID, adminID int64) (int64, error) {
	return d.deleteOn(ctx, d.db, `DELETE FROM chat_invites WHERE chat_id = $1 AND admin_id = $2 AND revoked = TRUE`, chatID, adminID)
}
func (d *ChatInvitesDAO) DeleteByRevokedOn(ctx context.Context, tx DB, chatID, adminID int64) (int64, error) {
	return d.deleteOn(ctx, tx, `DELETE FROM chat_invites WHERE chat_id = $1 AND admin_id = $2 AND revoked = TRUE`, chatID, adminID)
}
func (d *ChatInvitesDAO) deleteOn(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return commandRows(tag), nil
}
