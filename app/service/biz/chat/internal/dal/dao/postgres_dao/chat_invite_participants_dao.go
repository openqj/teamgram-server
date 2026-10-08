package postgres_dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

const inviteParticipantColumns = `id, chat_id, link, user_id, requested,
approved_by, date2, deleted`

const inviteParticipantColumnsAliased = `p.id, p.chat_id, p.link, p.user_id,
p.requested, p.approved_by, p.date2, p.deleted`

type ChatInviteParticipantsDAO struct{ db DB }

func NewChatInviteParticipantsDAO(db DB) *ChatInviteParticipantsDAO {
	return &ChatInviteParticipantsDAO{db: db}
}

func scanInviteParticipant(row interface{ Scan(...any) error }) (*dataobject.ChatInviteParticipantsDO, error) {
	do := new(dataobject.ChatInviteParticipantsDO)
	err := row.Scan(&do.Id, &do.ChatId, &do.Link, &do.UserId, &do.Requested,
		&do.ApprovedBy, &do.Date2, &do.Deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanInviteParticipants(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]dataobject.ChatInviteParticipantsDO, error) {
	defer rows.Close()
	result := make([]dataobject.ChatInviteParticipantsDO, 0)
	for rows.Next() {
		do, err := scanInviteParticipant(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *do)
	}
	return result, rows.Err()
}

func (d *ChatInviteParticipantsDAO) Insert(ctx context.Context, do *dataobject.ChatInviteParticipantsDO) (int64, int64, error) {
	return d.insertOn(ctx, d.db, do)
}
func (d *ChatInviteParticipantsDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.ChatInviteParticipantsDO) (int64, int64, error) {
	return d.insertOn(ctx, tx, do)
}
func (d *ChatInviteParticipantsDAO) insertOn(ctx context.Context, db DB, do *dataobject.ChatInviteParticipantsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO chat_invite_participants
 (chat_id,link,user_id,requested,approved_by,date2,deleted)
 VALUES ($1,$2,$3,$4,$5,$6,$7)
 ON CONFLICT (link,user_id) DO UPDATE SET chat_id = EXCLUDED.chat_id
 RETURNING id`, do.ChatId, do.Link, do.UserId,
		do.Requested, do.ApprovedBy, do.Date2, do.Deleted).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *ChatInviteParticipantsDAO) SelectListByLink(ctx context.Context, link string, requested int32) ([]dataobject.ChatInviteParticipantsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+inviteParticipantColumns+`
 FROM chat_invite_participants WHERE link = $1 AND requested = $2 ORDER BY id`, link, requested != 0)
	if err != nil {
		return nil, err
	}
	return scanInviteParticipants(rows)
}

func (d *ChatInviteParticipantsDAO) CountByLink(ctx context.Context, link string, requested bool) (int64, error) {
	var count int64
	err := d.db.QueryRow(ctx, `SELECT COUNT(*) FROM chat_invite_participants WHERE link = $1 AND requested = $2`, link, requested).Scan(&count)
	return count, err
}

func (d *ChatInviteParticipantsDAO) Delete(ctx context.Context, chatID, userID int64) (int64, error) {
	return d.deleteOn(ctx, d.db, `DELETE FROM chat_invite_participants WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
}
func (d *ChatInviteParticipantsDAO) DeleteOn(ctx context.Context, tx DB, chatID, userID int64) (int64, error) {
	return d.deleteOn(ctx, tx, `DELETE FROM chat_invite_participants WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
}
func (d *ChatInviteParticipantsDAO) deleteOn(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return commandRows(tag), nil
}

func (d *ChatInviteParticipantsDAO) SelectRecentRequestedList(ctx context.Context, chatID int64) ([]dataobject.ChatInviteParticipantsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+inviteParticipantColumns+`
 FROM chat_invite_participants WHERE chat_id = $1 AND requested = TRUE ORDER BY date2 DESC, id DESC`, chatID)
	if err != nil {
		return nil, err
	}
	return scanInviteParticipants(rows)
}

func (d *ChatInviteParticipantsDAO) SelectListByQuery(ctx context.Context, chatID int64, link string, requested int32, q string) ([]dataobject.ChatInviteParticipantsDO, error) {
	query := `SELECT ` + inviteParticipantColumnsAliased + `
 FROM chat_invite_participants p
 INNER JOIN users u ON u.id = p.user_id
 WHERE p.chat_id = $1 AND p.requested = $2 AND u.deleted = FALSE
 AND (u.username ILIKE $3 OR u.first_name ILIKE $4 OR u.last_name ILIKE $5)`
	args := []any{chatID, requested != 0, q + "%", "%" + q + "%", "%" + q + "%"}
	if requested != 1 || link != "" {
		query += " AND p.link = $6"
		args = append(args, link)
	}
	query += " ORDER BY p.id"
	rows, err := d.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanInviteParticipants(rows)
}

func (d *ChatInviteParticipantsDAO) UpdateChatId(ctx context.Context, chatID int64, link string) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_invite_participants SET chat_id = $1 WHERE link = $2`, chatID, link)
}
func (d *ChatInviteParticipantsDAO) UpdateChatIdOn(ctx context.Context, tx DB, chatID int64, link string) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_invite_participants SET chat_id = $1 WHERE link = $2`, chatID, link)
}
func (d *ChatInviteParticipantsDAO) UpdateApprovedBy(ctx context.Context, approvedBy, chatID, userID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_invite_participants SET requested = FALSE, approved_by = $1 WHERE chat_id = $2 AND user_id = $3`, approvedBy, chatID, userID)
}
func (d *ChatInviteParticipantsDAO) UpdateApprovedByOn(ctx context.Context, tx DB, approvedBy, chatID, userID int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_invite_participants SET requested = FALSE, approved_by = $1 WHERE chat_id = $2 AND user_id = $3`, approvedBy, chatID, userID)
}
func (d *ChatInviteParticipantsDAO) update(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return commandRows(tag), nil
}

func (d *ChatInviteParticipantsDAO) DeleteByLink(ctx context.Context, chatID, userID int64, link string) (int64, error) {
	return d.deleteOn(ctx, d.db, `DELETE FROM chat_invite_participants WHERE chat_id = $1 AND user_id = $2 AND link = $3`, chatID, userID, link)
}
func (d *ChatInviteParticipantsDAO) DeleteByLinkOn(ctx context.Context, tx DB, chatID, userID int64, link string) (int64, error) {
	return d.deleteOn(ctx, tx, `DELETE FROM chat_invite_participants WHERE chat_id = $1 AND user_id = $2 AND link = $3`, chatID, userID, link)
}
func (d *ChatInviteParticipantsDAO) UpdateApprovedByLink(ctx context.Context, approvedBy, chatID, userID int64, link string) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_invite_participants SET requested = FALSE, approved_by = $1 WHERE chat_id = $2 AND user_id = $3 AND link = $4`, approvedBy, chatID, userID, link)
}
func (d *ChatInviteParticipantsDAO) UpdateApprovedByLinkOn(ctx context.Context, tx DB, approvedBy, chatID, userID int64, link string) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_invite_participants SET requested = FALSE, approved_by = $1 WHERE chat_id = $2 AND user_id = $3 AND link = $4`, approvedBy, chatID, userID, link)
}
