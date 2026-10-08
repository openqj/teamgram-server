package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
)

type MessageReadOutboxDAO struct{ db DB }

func NewMessageReadOutboxDAO(db DB) *MessageReadOutboxDAO { return &MessageReadOutboxDAO{db: db} }

func (d *MessageReadOutboxDAO) InsertOrUpdate(ctx context.Context, do *dataobject.MessageReadOutboxDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO message_read_outbox
 (user_id, peer_dialog_id, read_user_id, read_outbox_max_id, read_outbox_max_date)
 VALUES ($1,$2,$3,$4,$5)
 ON CONFLICT (user_id, peer_dialog_id, read_user_id) DO UPDATE SET
  read_outbox_max_id = GREATEST(message_read_outbox.read_outbox_max_id, EXCLUDED.read_outbox_max_id),
  read_outbox_max_date = GREATEST(message_read_outbox.read_outbox_max_date, EXCLUDED.read_outbox_max_date)
 RETURNING id`, do.UserId, do.PeerDialogId, do.ReadUserId, do.ReadOutboxMaxId, do.ReadOutboxMaxDate).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	return id, 1, nil
}

func (d *MessageReadOutboxDAO) SelectList(ctx context.Context, userID, readUserID int64, maxID int32) ([]dataobject.MessageReadOutboxDO, error) {
	rows, err := d.db.Query(ctx, `SELECT id, user_id, peer_dialog_id, read_user_id, read_outbox_max_id, read_outbox_max_date FROM message_read_outbox WHERE user_id = $1 AND read_user_id = $2 AND read_outbox_max_id >= $3 ORDER BY read_outbox_max_id ASC LIMIT 1`, userID, readUserID, maxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.MessageReadOutboxDO, 0)
	for rows.Next() {
		var do dataobject.MessageReadOutboxDO
		if err := rows.Scan(&do.Id, &do.UserId, &do.PeerDialogId, &do.ReadUserId, &do.ReadOutboxMaxId, &do.ReadOutboxMaxDate); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}
