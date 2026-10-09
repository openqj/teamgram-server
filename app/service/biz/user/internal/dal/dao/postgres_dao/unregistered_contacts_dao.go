package postgres_dao

import (
	"context"
	"fmt"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

type UnregisteredContactsDAO struct{ db DB }

func NewUnregisteredContactsDAO(db DB) *UnregisteredContactsDAO {
	return &UnregisteredContactsDAO{db: db}
}

func (d *UnregisteredContactsDAO) InsertOrUpdate(ctx context.Context, v *dataobject.UnregisteredContactsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, d.db, v)
}

func (d *UnregisteredContactsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, v *dataobject.UnregisteredContactsDO) (int64, int64, error) {
	return d.insertOrUpdate(ctx, tx, v)
}

func (d *UnregisteredContactsDAO) insertOrUpdate(ctx context.Context, db DB, v *dataobject.UnregisteredContactsDO) (int64, int64, error) {
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO unregistered_contacts(phone, importer_user_id, import_first_name, import_last_name, imported)
VALUES ($1,$2,$3,$4,FALSE)
ON CONFLICT (phone, importer_user_id) DO UPDATE SET import_first_name=EXCLUDED.import_first_name, import_last_name=EXCLUDED.import_last_name, imported=FALSE
RETURNING id`, v.Phone, v.ImporterUserId, v.ImportFirstName, v.ImportLastName).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	v.Id = id
	return id, 1, nil
}

func (d *UnregisteredContactsDAO) SelectImportersByPhoneWithCB(ctx context.Context, phone string, cb func(int, int, *dataobject.UnregisteredContactsDO)) ([]dataobject.UnregisteredContactsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT id, importer_user_id, phone, import_first_name, import_last_name, imported
FROM unregistered_contacts WHERE phone=$1 AND imported=FALSE ORDER BY id`, phone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dataobject.UnregisteredContactsDO, 0)
	for rows.Next() {
		var v dataobject.UnregisteredContactsDO
		if err := rows.Scan(&v.Id, &v.ImporterUserId, &v.Phone, &v.ImportFirstName, &v.ImportLastName, &v.Imported); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, nil
}

func (d *UnregisteredContactsDAO) DeleteImportersByPhone(ctx context.Context, phone string) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE unregistered_contacts SET imported=TRUE WHERE phone=$1`, phone)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UnregisteredContactsDAO) DeleteImporterByUserAndPhone(ctx context.Context, phone string, userID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE unregistered_contacts SET imported=TRUE WHERE phone=$1 AND importer_user_id=$2`, phone, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UnregisteredContactsDAO) SelectDistinctImporterCountsByPhoneList(ctx context.Context, phones []string) (map[string]int32, error) {
	result := make(map[string]int32)
	if len(phones) == 0 {
		return result, nil
	}
	rows, err := d.db.Query(ctx, `SELECT phone, COUNT(DISTINCT importer_user_id) FROM unregistered_contacts WHERE phone = ANY($1::text[]) AND imported=FALSE GROUP BY phone`, phones)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var phone string
		var count int32
		if err := rows.Scan(&phone, &count); err != nil {
			return nil, err
		}
		result[phone] = count
	}
	return result, rows.Err()
}

func (d *UnregisteredContactsDAO) validate() error {
	if d == nil || d.db == nil {
		return fmt.Errorf("postgres: unregistered contacts store is not configured")
	}
	return nil
}
