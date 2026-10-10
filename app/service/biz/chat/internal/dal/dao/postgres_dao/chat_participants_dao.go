package postgres_dao

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

const participantColumns = `id, chat_id, user_id, participant_type, link,
usage2, admin_rights, inviter_user_id, invited_at, kicked_at, left_at,
groupcall_default_join_as_peer_type, groupcall_default_join_as_peer_id,
is_bot, state, date2, rank2`

type ChatParticipantsDAO struct{ db DB }

func NewChatParticipantsDAO(db DB) *ChatParticipantsDAO { return &ChatParticipantsDAO{db: db} }

func scanParticipant(row interface{ Scan(...any) error }) (*dataobject.ChatParticipantsDO, error) {
	do := new(dataobject.ChatParticipantsDO)
	err := row.Scan(&do.Id, &do.ChatId, &do.UserId, &do.ParticipantType, &do.Link,
		&do.Usage2, &do.AdminRights, &do.InviterUserId, &do.InvitedAt, &do.KickedAt,
		&do.LeftAt, &do.GroupcallDefaultJoinAsPeerType, &do.GroupcallDefaultJoinAsPeerId,
		&do.IsBot, &do.State, &do.Date2, &do.Rank2)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return do, nil
}

func scanParticipants(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]dataobject.ChatParticipantsDO, error) {
	defer rows.Close()
	list := make([]dataobject.ChatParticipantsDO, 0)
	for rows.Next() {
		do, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *do)
	}
	return list, rows.Err()
}

func (d *ChatParticipantsDAO) Insert(ctx context.Context, do *dataobject.ChatParticipantsDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO chat_participants
 (chat_id,user_id,participant_type,link,inviter_user_id,invited_at,is_bot,date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, do.ChatId, do.UserId,
		do.ParticipantType, do.Link, do.InviterUserId, do.InvitedAt, do.IsBot, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *ChatParticipantsDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.ChatParticipantsDO) (int64, int64, error) {
	return NewChatParticipantsDAO(tx).Insert(ctx, do)
}

func (d *ChatParticipantsDAO) InsertBulk(ctx context.Context, list []*dataobject.ChatParticipantsDO) (int64, int64, error) {
	return d.insertBulk(ctx, d.db, list)
}

func (d *ChatParticipantsDAO) InsertBulkOn(ctx context.Context, tx DB, list []*dataobject.ChatParticipantsDO) (int64, int64, error) {
	return d.insertBulk(ctx, tx, list)
}

func (d *ChatParticipantsDAO) insertBulk(ctx context.Context, db DB, list []*dataobject.ChatParticipantsDO) (int64, int64, error) {
	var affected int64
	for _, do := range list {
		_, rows, err := NewChatParticipantsDAO(db).Insert(ctx, do)
		if err != nil {
			return 0, affected, err
		}
		affected += rows
	}
	return 0, affected, nil
}

func (d *ChatParticipantsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.ChatParticipantsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, do)
}

func (d *ChatParticipantsDAO) InsertOrUpdateOn(ctx context.Context, tx DB, do *dataobject.ChatParticipantsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, do)
}

func (d *ChatParticipantsDAO) insertOrUpdate(ctx context.Context, db DB, do *dataobject.ChatParticipantsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO chat_participants
 (chat_id,user_id,participant_type,link,inviter_user_id,invited_at,is_bot,date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT (chat_id,user_id) DO UPDATE SET participant_type = EXCLUDED.participant_type,
 inviter_user_id = EXCLUDED.inviter_user_id, link = EXCLUDED.link,
 invited_at = EXCLUDED.invited_at, is_bot = EXCLUDED.is_bot, state = 0,
 kicked_at = 0, left_at = 0, date2 = EXCLUDED.date2
 RETURNING id`, do.ChatId, do.UserId, do.ParticipantType, do.Link,
		do.InviterUserId, do.InvitedAt, do.IsBot, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *ChatParticipantsDAO) SelectList(ctx context.Context, chatID int64) ([]dataobject.ChatParticipantsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+participantColumns+` FROM chat_participants WHERE chat_id = $1 ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	return scanParticipants(rows)
}

func (d *ChatParticipantsDAO) SelectByParticipantId(ctx context.Context, chatID, userID int64) (*dataobject.ChatParticipantsDO, error) {
	row := d.db.QueryRow(ctx, `SELECT `+participantColumns+` FROM chat_participants WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
	do, err := scanParticipant(row)
	if err != nil {
		return nil, err
	}
	return do, nil
}

func (d *ChatParticipantsDAO) SelectChatParticipantIdList(ctx context.Context, chatID int64) ([]int64, error) {
	rows, err := d.db.Query(ctx, `SELECT user_id FROM chat_participants WHERE chat_id = $1 ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}
	return list, rows.Err()
}

func (d *ChatParticipantsDAO) Update(ctx context.Context, participantType int32, inviterUserID, invitedAt int64, isBot bool, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET participant_type = $1,
 inviter_user_id = $2, invited_at = $3, state = 0, kicked_at = 0, left_at = 0, is_bot = $4 WHERE id = $5`, participantType, inviterUserID, invitedAt, isBot, id)
}
func (d *ChatParticipantsDAO) UpdateOn(ctx context.Context, tx DB, participantType int32, inviterUserID, invitedAt int64, isBot bool, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET participant_type = $1,
 inviter_user_id = $2, invited_at = $3, state = 0, kicked_at = 0, left_at = 0, is_bot = $4 WHERE id = $5`, participantType, inviterUserID, invitedAt, isBot, id)
}
func (d *ChatParticipantsDAO) UpdateKicked(ctx context.Context, kickedAt, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET kicked_at = $1, left_at = 0, state = 2 WHERE id = $2`, kickedAt, id)
}
func (d *ChatParticipantsDAO) UpdateKickedOn(ctx context.Context, tx DB, kickedAt, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET kicked_at = $1, left_at = 0, state = 2 WHERE id = $2`, kickedAt, id)
}
func (d *ChatParticipantsDAO) UpdateLeft(ctx context.Context, leftAt, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET kicked_at = 0, left_at = $1, state = 1 WHERE id = $2`, leftAt, id)
}
func (d *ChatParticipantsDAO) UpdateLeftOn(ctx context.Context, tx DB, leftAt, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET kicked_at = 0, left_at = $1, state = 1 WHERE id = $2`, leftAt, id)
}
func (d *ChatParticipantsDAO) UpdateParticipantType(ctx context.Context, participantType int32, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET participant_type = $1 WHERE id = $2`, participantType, id)
}
func (d *ChatParticipantsDAO) UpdateParticipantTypeOn(ctx context.Context, tx DB, participantType int32, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET participant_type = $1 WHERE id = $2`, participantType, id)
}
func (d *ChatParticipantsDAO) UpdateParticipantTypeAndRightsOn(ctx context.Context, tx DB, participantType, adminRights int32, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET participant_type = $1, admin_rights = $2 WHERE id = $3`, participantType, adminRights, id)
}
func (d *ChatParticipantsDAO) UpdateRank(ctx context.Context, rank string, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET rank2 = $1 WHERE id = $2`, rank, id)
}
func (d *ChatParticipantsDAO) UpdateRankOn(ctx context.Context, tx DB, rank string, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET rank2 = $1 WHERE id = $2`, rank, id)
}
func (d *ChatParticipantsDAO) UpdateStateByChatId(ctx context.Context, state int32, chatID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET state = $1 WHERE chat_id = $2 AND state = 0`, state, chatID)
}
func (d *ChatParticipantsDAO) UpdateStateByChatIdOn(ctx context.Context, tx DB, state int32, chatID int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET state = $1 WHERE chat_id = $2 AND state = 0`, state, chatID)
}
func (d *ChatParticipantsDAO) UpdateLink(ctx context.Context, link string, chatID, userID int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chat_participants SET link = $1 WHERE chat_id = $2 AND user_id = $3`, link, chatID, userID)
}
func (d *ChatParticipantsDAO) UpdateLinkOn(ctx context.Context, tx DB, link string, chatID, userID int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chat_participants SET link = $1 WHERE chat_id = $2 AND user_id = $3`, link, chatID, userID)
}

func (d *ChatParticipantsDAO) update(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return commandRows(tag), nil
}

func (d *ChatParticipantsDAO) SelectUsersChatIdList(ctx context.Context, userIDs []int64) ([]dataobject.ChatParticipantsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT p.chat_id, p.user_id FROM chat_participants p
 INNER JOIN chats c ON c.id = p.chat_id
 WHERE p.state = 0 AND p.user_id = ANY($1::bigint[]) AND c.deactivated = FALSE
 ORDER BY p.chat_id, p.user_id`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.ChatParticipantsDO, 0)
	for rows.Next() {
		var do dataobject.ChatParticipantsDO
		if err := rows.Scan(&do.ChatId, &do.UserId); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *ChatParticipantsDAO) SelectMyAdminList(ctx context.Context, userID int64) ([]int64, error) {
	return d.selectIDs(ctx, `SELECT chat_id FROM chat_participants WHERE user_id = $1 AND participant_type = 1 AND state = 0`, userID)
}
func (d *ChatParticipantsDAO) SelectMyAllList(ctx context.Context, userID int64) ([]int64, error) {
	return d.selectIDs(ctx, `SELECT chat_id FROM chat_participants WHERE user_id = $1 AND state = 0`, userID)
}
func (d *ChatParticipantsDAO) selectIDs(ctx context.Context, query string, userID int64) ([]int64, error) {
	rows, err := d.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
