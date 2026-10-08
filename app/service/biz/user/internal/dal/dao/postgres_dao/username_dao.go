package postgres_dao

import (
	"context"
	"fmt"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

type UsernameDAO struct{ db DB }

func NewUsernameDAO(db DB) *UsernameDAO { return &UsernameDAO{db: db} }

func scanUsername(row interface{ Scan(...any) error }) (*dataobject.UsernameDO, error) {
	do := new(dataobject.UsernameDO)
	if err := row.Scan(&do.Id, &do.Username, &do.PeerType, &do.PeerId, &do.Editable, &do.Active, &do.Order2, &do.Deleted); err != nil {
		return nil, err
	}
	return do, nil
}

const usernameColumns = `id, username, peer_type, peer_id, editable, active, order2, deleted`

func (d *UsernameDAO) Insert(ctx context.Context, do *dataobject.UsernameDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
 VALUES ($1,$2,$3,$4,$5,$6,FALSE) RETURNING id`, do.Username, do.PeerType, do.PeerId, do.Editable, do.Active, do.Order2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *UsernameDAO) SelectByUsername(ctx context.Context, username string) (*dataobject.UsernameDO, error) {
	return scanUsername(d.db.QueryRow(ctx, `SELECT `+usernameColumns+` FROM username WHERE lower(username) = lower($1)`, username))
}

func (d *UsernameDAO) SelectByPeer(ctx context.Context, peerType int32, peerID int64) ([]dataobject.UsernameDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+usernameColumns+` FROM username WHERE peer_type = $1 AND peer_id = $2 AND editable = TRUE AND deleted = FALSE ORDER BY order2, id`, peerType, peerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.UsernameDO, 0)
	for rows.Next() {
		var do dataobject.UsernameDO
		if err := rows.Scan(&do.Id, &do.Username, &do.PeerType, &do.PeerId, &do.Editable, &do.Active, &do.Order2, &do.Deleted); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *UsernameDAO) SelectByUserID(ctx context.Context, userID int64) ([]dataobject.UsernameDO, error) {
	return d.SelectByPeer(ctx, 2, userID)
}

func (d *UsernameDAO) SelectList(ctx context.Context, names []string) ([]dataobject.UsernameDO, error) {
	if len(names) == 0 {
		return []dataobject.UsernameDO{}, nil
	}
	canonicalNames := make([]string, len(names))
	for i, name := range names {
		canonicalNames[i] = strings.ToLower(name)
	}
	rows, err := d.db.Query(ctx, `SELECT `+usernameColumns+` FROM username WHERE lower(username) = ANY($1::text[]) AND editable = TRUE AND deleted = FALSE ORDER BY id`, canonicalNames)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.UsernameDO, 0)
	for rows.Next() {
		var do dataobject.UsernameDO
		if err := rows.Scan(&do.Id, &do.Username, &do.PeerType, &do.PeerId, &do.Editable, &do.Active, &do.Order2, &do.Deleted); err != nil {
			return nil, err
		}
		result = append(result, do)
	}
	return result, rows.Err()
}

func (d *UsernameDAO) Update(ctx context.Context, values map[string]any, username string) (int64, error) {
	allowed := map[string]bool{"peer_type": true, "peer_id": true, "editable": true, "active": true, "order2": true, "deleted": true, "username": true}
	sets := make([]string, 0, len(values))
	args := make([]any, 0, len(values)+1)
	for name, value := range values {
		if !allowed[name] {
			return 0, fmt.Errorf("unsupported username column %q", name)
		}
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", name, len(args)))
	}
	if len(sets) == 0 {
		return 0, nil
	}
	args = append(args, username)
	tag, err := d.db.Exec(ctx, `UPDATE username SET `+strings.Join(sets, ", ")+` WHERE lower(username) = lower($`+fmt.Sprint(len(args))+`)`, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UsernameDAO) Delete(ctx context.Context, username string) (int64, error) {
	tag, err := d.db.Exec(ctx, `DELETE FROM username WHERE lower(username) = lower($1)`, username)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UsernameDAO) DeleteByPeer(ctx context.Context, peerType int32, peerID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `DELETE FROM username WHERE peer_type = $1 AND peer_id = $2 AND editable = TRUE`, peerType, peerID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
