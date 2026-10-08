package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

// HashTagsDAO mirrors the generated messenger DAO while using PostgreSQL
// placeholders and an explicit conflict policy.
type HashTagsDAO struct{ db DB }

func NewHashTagsDAO(db DB) *HashTagsDAO { return &HashTagsDAO{db: db} }

func (d *HashTagsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.HashTagsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, do)
}

func (d *HashTagsDAO) InsertOrUpdateOn(ctx context.Context, tx DB, do *dataobject.HashTagsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, do)
}

func (d *HashTagsDAO) insertOrUpdate(ctx context.Context, db DB, do *dataobject.HashTagsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO hash_tags
 (user_id, peer_type, peer_id, hash_tag, hash_tag_message_id)
 VALUES ($1,$2,$3,$4,$5)
 ON CONFLICT (user_id, peer_type, peer_id, hash_tag, hash_tag_message_id)
 DO UPDATE SET deleted = FALSE RETURNING id`, do.UserId, do.PeerType, do.PeerId,
		do.HashTag, do.HashTagMessageId).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *HashTagsDAO) SelectPeerHashTagList(ctx context.Context, userID int64, peerType int32, peerID int64, hashTag string) ([]int32, error) {
	rows, err := d.db.Query(ctx, `SELECT hash_tag_message_id FROM hash_tags
 WHERE user_id = $1 AND peer_type = $2 AND peer_id = $3 AND hash_tag = $4
   AND deleted = FALSE ORDER BY hash_tag_message_id`, userID, peerType, peerID, hashTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (d *HashTagsDAO) SelectPeerHashTagListWithCB(ctx context.Context, userID int64, peerType int32, peerID int64, hashTag string, cb func(int, int, int32)) ([]int32, error) {
	result, err := d.SelectPeerHashTagList(ctx, userID, peerType, peerID, hashTag)
	if cb != nil {
		for i, id := range result {
			cb(len(result), i, id)
		}
	}
	return result, err
}

func (d *HashTagsDAO) DeleteHashTagMessageId(ctx context.Context, userID int64, messageID int32) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE hash_tags SET deleted = TRUE
 WHERE user_id = $1 AND hash_tag_message_id = $2`, userID, messageID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}

func (d *HashTagsDAO) DeleteHashTagMessageIdOn(ctx context.Context, tx DB, userID int64, messageID int32) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE hash_tags SET deleted = TRUE
 WHERE user_id = $1 AND hash_tag_message_id = $2`, userID, messageID)
	if err != nil {
		return 0, err
	}
	return rowsAffected(tag), nil
}
