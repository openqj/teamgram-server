package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

const chatColumns = `id, creator_user_id, access_hash, participant_count,
title, about, photo_id, default_banned_rights, migrated_to_id,
migrated_to_access_hash, noforwards, available_reactions_type,
available_reactions, deactivated, ttl_period, version, date`

func (d *ChatsDAO) Insert(ctx context.Context, do *dataobject.ChatsDO) (lastInsertID, rowsAffected int64, err error) {
	err = d.db.QueryRow(ctx, `INSERT INTO chats
 (creator_user_id, access_hash, random_id, participant_count, title, about, default_banned_rights, date)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		do.CreatorUserId, do.AccessHash, do.RandomId, do.ParticipantCount,
		do.Title, do.About, do.DefaultBannedRights, do.Date).Scan(&lastInsertID)
	if err == nil {
		rowsAffected = 1
	}
	return
}

// InsertOn executes Insert against an explicit transaction or connection.
func (d *ChatsDAO) InsertOn(ctx context.Context, tx DB, do *dataobject.ChatsDO) (lastInsertID, rowsAffected int64, err error) {
	return NewChatsDAO(tx).Insert(ctx, do)
}

func (d *ChatsDAO) Select(ctx context.Context, id int64) (*dataobject.ChatsDO, error) {
	return scanChat(d.db.QueryRow(ctx, `SELECT `+chatColumns+` FROM chats WHERE id = $1`, id))
}

func (d *ChatsDAO) SelectLastCreator(ctx context.Context, creatorUserID int64) (*dataobject.ChatsDO, error) {
	return scanChat(d.db.QueryRow(ctx, `SELECT `+chatColumns+`
 FROM chats WHERE creator_user_id = $1 ORDER BY date DESC, id DESC LIMIT 1`, creatorUserID))
}

func (d *ChatsDAO) SelectByIDList(ctx context.Context, ids []int64) ([]dataobject.ChatsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+chatColumns+` FROM chats
 WHERE id = ANY($1::bigint[]) ORDER BY id`, ids)
	if err != nil {
		return nil, err
	}
	return scanChatRows(rows)
}

// SelectByIdList keeps the generated Teamgram spelling available at the
// migration boundary while new callers can use the initialism form above.
func (d *ChatsDAO) SelectByIdList(ctx context.Context, ids []int64) ([]dataobject.ChatsDO, error) {
	return d.SelectByIDList(ctx, ids)
}

func (d *ChatsDAO) update(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	tag, err := db.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return commandRows(tag), nil
}

func (d *ChatsDAO) UpdateTitle(ctx context.Context, title string, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET title = $1, version = version + 1 WHERE id = $2`, title, id)
}
func (d *ChatsDAO) UpdateTitleOn(ctx context.Context, tx DB, title string, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET title = $1, version = version + 1 WHERE id = $2`, title, id)
}

func (d *ChatsDAO) UpdateAbout(ctx context.Context, about string, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET about = $1 WHERE id = $2`, about, id)
}
func (d *ChatsDAO) UpdateAboutOn(ctx context.Context, tx DB, about string, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET about = $1, version = version + 1 WHERE id = $2`, about, id)
}
func (d *ChatsDAO) UpdateParticipantCount(ctx context.Context, count int32, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET participant_count = $1, version = version + 1 WHERE id = $2`, count, id)
}
func (d *ChatsDAO) UpdateParticipantCountOn(ctx context.Context, tx DB, count int32, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET participant_count = $1, version = version + 1 WHERE id = $2`, count, id)
}

func (d *ChatsDAO) UpdatePhotoID(ctx context.Context, photoID, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET photo_id = $1, version = version + 1 WHERE id = $2`, photoID, id)
}
func (d *ChatsDAO) UpdatePhotoIDOn(ctx context.Context, tx DB, photoID, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET photo_id = $1, version = version + 1 WHERE id = $2`, photoID, id)
}

func (d *ChatsDAO) UpdatePhotoId(ctx context.Context, photoID, id int64) (int64, error) {
	return d.UpdatePhotoID(ctx, photoID, id)
}
func (d *ChatsDAO) UpdateDefaultBannedRights(ctx context.Context, rights, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET default_banned_rights = $1, version = version + 1 WHERE id = $2`, rights, id)
}
func (d *ChatsDAO) UpdateDefaultBannedRightsOn(ctx context.Context, tx DB, rights, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET default_banned_rights = $1, version = version + 1 WHERE id = $2`, rights, id)
}
func (d *ChatsDAO) UpdateVersion(ctx context.Context, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET version = version + 1 WHERE id = $1`, id)
}
func (d *ChatsDAO) UpdateVersionOn(ctx context.Context, tx DB, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET version = version + 1 WHERE id = $1`, id)
}
func (d *ChatsDAO) UpdateDeactivated(ctx context.Context, deactivated bool, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET deactivated = $1, version = version + 1 WHERE id = $2`, deactivated, id)
}
func (d *ChatsDAO) UpdateDeactivatedOn(ctx context.Context, tx DB, deactivated bool, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET deactivated = $1, version = version + 1 WHERE id = $2`, deactivated, id)
}
func (d *ChatsDAO) UpdateMigratedTo(ctx context.Context, migratedID, accessHash, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET migrated_to_id = $1, migrated_to_access_hash = $2,
 participant_count = 0, deactivated = TRUE, version = version + 1 WHERE id = $3`, migratedID, accessHash, id)
}
func (d *ChatsDAO) UpdateMigratedToOn(ctx context.Context, tx DB, migratedID, accessHash, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET migrated_to_id = $1, migrated_to_access_hash = $2,
 participant_count = 0, deactivated = TRUE, version = version + 1 WHERE id = $3`, migratedID, accessHash, id)
}
func (d *ChatsDAO) UpdateAvailableReactions(ctx context.Context, reactionType int32, reactions string, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET available_reactions_type = $1, available_reactions = $2 WHERE id = $3`, reactionType, reactions, id)
}
func (d *ChatsDAO) UpdateAvailableReactionsOn(ctx context.Context, tx DB, reactionType int32, reactions string, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET available_reactions_type = $1, available_reactions = $2, version = version + 1 WHERE id = $3`, reactionType, reactions, id)
}
func (d *ChatsDAO) UpdateNoforwards(ctx context.Context, value bool, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET noforwards = $1, version = version + 1 WHERE id = $2`, value, id)
}
func (d *ChatsDAO) UpdateNoforwardsOn(ctx context.Context, tx DB, value bool, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET noforwards = $1, version = version + 1 WHERE id = $2`, value, id)
}
func (d *ChatsDAO) UpdateTTLPeriod(ctx context.Context, period int32, id int64) (int64, error) {
	return d.update(ctx, d.db, `UPDATE chats SET ttl_period = $1 WHERE id = $2`, period, id)
}
func (d *ChatsDAO) UpdateTTLPeriodOn(ctx context.Context, tx DB, period int32, id int64) (int64, error) {
	return d.update(ctx, tx, `UPDATE chats SET ttl_period = $1 WHERE id = $2`, period, id)
}

func (d *ChatsDAO) SearchByQueryString(ctx context.Context, q string, limit int32) ([]int64, error) {
	return d.search(ctx, d.db, `SELECT id FROM chats WHERE title ILIKE $1 ORDER BY id DESC LIMIT $2`, q, limit)
}

func (d *ChatsDAO) SearchByQueryStringForUser(ctx context.Context, userID int64, q string, limit int32) ([]int64, error) {
	return d.SearchByQueryStringForUserOffset(ctx, userID, q, 0, limit)
}

func (d *ChatsDAO) SearchByQueryStringForUserOffset(ctx context.Context, userID int64, q string, offset int64, limit int32) ([]int64, error) {
	return d.search(ctx, d.db, `SELECT c.id FROM chats c
 INNER JOIN chat_participants p ON p.chat_id = c.id
 WHERE p.user_id = $1 AND p.state = 0 AND c.deactivated = FALSE AND c.title ILIKE $2
 ORDER BY c.id DESC LIMIT $3 OFFSET $4`, userID, q, limit, offset)
}

func (d *ChatsDAO) search(ctx context.Context, db DB, query string, args ...any) ([]int64, error) {
	rows, err := db.Query(ctx, query, args...)
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
