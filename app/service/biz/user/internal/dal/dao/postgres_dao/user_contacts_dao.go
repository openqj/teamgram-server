package postgres_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

type UserContactsDAO struct{ db DB }

func NewUserContactsDAO(db DB) *UserContactsDAO { return &UserContactsDAO{db: db} }

const contactColumns = `id, owner_user_id, contact_user_id, contact_phone,
contact_first_name, contact_last_name, mutual, close_friend, stories_hidden, date2, is_deleted`

func (d *UserContactsDAO) InsertOrUpdate(ctx context.Context, do *dataobject.UserContactsDO) (int64, int64, error) {
	var id int64
	err := d.db.QueryRow(ctx, `INSERT INTO user_contacts
 (owner_user_id, contact_user_id, contact_phone, contact_first_name, contact_last_name, mutual, date2)
	VALUES ($1,$2,$3,$4,$5,$6,$7)
	ON CONFLICT (owner_user_id, contact_user_id) DO UPDATE SET
	contact_phone = EXCLUDED.contact_phone, contact_first_name = EXCLUDED.contact_first_name,
	contact_last_name = EXCLUDED.contact_last_name, mutual = EXCLUDED.mutual,
	date2 = EXCLUDED.date2, is_deleted = FALSE RETURNING id`,
		do.OwnerUserId, do.ContactUserId, do.ContactPhone, do.ContactFirstName,
		do.ContactLastName, do.Mutual, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *UserContactsDAO) InsertOrUpdateTx(ctx context.Context, tx DB, do *dataobject.UserContactsDO) (int64, int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO user_contacts
 (owner_user_id, contact_user_id, contact_phone, contact_first_name, contact_last_name, mutual, date2)
 VALUES ($1,$2,$3,$4,$5,$6,$7)
 ON CONFLICT (owner_user_id, contact_user_id) DO UPDATE SET
 contact_phone = EXCLUDED.contact_phone, contact_first_name = EXCLUDED.contact_first_name,
 contact_last_name = EXCLUDED.contact_last_name, mutual = EXCLUDED.mutual,
 date2 = EXCLUDED.date2, is_deleted = FALSE RETURNING id`,
		do.OwnerUserId, do.ContactUserId, do.ContactPhone, do.ContactFirstName,
		do.ContactLastName, do.Mutual, do.Date2).Scan(&id)
	if err != nil {
		return 0, 0, err
	}
	do.Id = id
	return id, 1, nil
}

func (d *UserContactsDAO) SelectContact(ctx context.Context, ownerID, contactID int64) (*dataobject.UserContactsDO, error) {
	return scanContact(d.db.QueryRow(ctx, `SELECT `+contactColumns+` FROM user_contacts WHERE owner_user_id = $1 AND contact_user_id = $2 AND is_deleted = FALSE`, ownerID, contactID))
}

func (d *UserContactsDAO) SelectByContactId(ctx context.Context, ownerID, contactID int64) (*dataobject.UserContactsDO, error) {
	return d.SelectContact(ctx, ownerID, contactID)
}

func (d *UserContactsDAO) SelectListByPhoneList(ctx context.Context, ownerID int64, phones []string) ([]dataobject.UserContactsDO, error) {
	if len(phones) == 0 {
		return []dataobject.UserContactsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+contactColumns+` FROM user_contacts WHERE owner_user_id = $1 AND contact_phone = ANY($2::text[]) AND is_deleted = FALSE`, ownerID, phones)
	if err != nil {
		return nil, err
	}
	return scanContactRows(rows)
}

func (d *UserContactsDAO) SelectListByPhoneListWithCB(ctx context.Context, ownerID int64, phones []string, cb func(sz, i int, v *dataobject.UserContactsDO)) ([]dataobject.UserContactsDO, error) {
	result, err := d.SelectListByPhoneList(ctx, ownerID, phones)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, nil
}

func (d *UserContactsDAO) SelectAllUserContacts(ctx context.Context, ownerID int64) ([]dataobject.UserContactsDO, error) {
	rows, err := d.db.Query(ctx, `SELECT `+contactColumns+` FROM user_contacts WHERE owner_user_id = $1 AND is_deleted = FALSE ORDER BY contact_user_id`, ownerID)
	if err != nil {
		return nil, err
	}
	return scanContactRows(rows)
}

func (d *UserContactsDAO) SelectUserContacts(ctx context.Context, ownerID int64) ([]dataobject.UserContactsDO, error) {
	return d.SelectAllUserContacts(ctx, ownerID)
}

func (d *UserContactsDAO) SelectAllUserContactsWithCB(ctx context.Context, ownerID int64, cb func(sz, i int, v *dataobject.UserContactsDO)) ([]dataobject.UserContactsDO, error) {
	result, err := d.SelectAllUserContacts(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, nil
}

func (d *UserContactsDAO) SelectUserContactsWithCB(ctx context.Context, ownerID int64, cb func(sz, i int, v *dataobject.UserContactsDO)) ([]dataobject.UserContactsDO, error) {
	return d.SelectAllUserContactsWithCB(ctx, ownerID, cb)
}

func (d *UserContactsDAO) SelectUserContactIDList(ctx context.Context, ownerID int64) ([]int64, error) {
	rows, err := d.db.Query(ctx, `SELECT contact_user_id FROM user_contacts WHERE owner_user_id = $1 AND is_deleted = FALSE ORDER BY contact_user_id`, ownerID)
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

func (d *UserContactsDAO) SelectUserContactIdList(ctx context.Context, ownerID int64) ([]int64, error) {
	return d.SelectUserContactIDList(ctx, ownerID)
}

func (d *UserContactsDAO) SelectUserContactIdListWithCB(ctx context.Context, ownerID int64, cb func(sz, i int, v int64)) ([]int64, error) {
	result, err := d.SelectUserContactIDList(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i, id := range result {
			cb(len(result), i, id)
		}
	}
	return result, nil
}

func (d *UserContactsDAO) SelectListByIdList(ctx context.Context, ownerID int64, ids []int64) ([]dataobject.UserContactsDO, error) {
	if len(ids) == 0 {
		return []dataobject.UserContactsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+contactColumns+` FROM user_contacts WHERE owner_user_id = $1 AND contact_user_id = ANY($2::bigint[]) AND is_deleted = FALSE ORDER BY contact_user_id`, ownerID, ids)
	if err != nil {
		return nil, err
	}
	return scanContactRows(rows)
}

func (d *UserContactsDAO) SelectListByIdListWithCB(ctx context.Context, ownerID int64, ids []int64, cb func(sz, i int, v *dataobject.UserContactsDO)) ([]dataobject.UserContactsDO, error) {
	result, err := d.SelectListByIdList(ctx, ownerID, ids)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, nil
}

func (d *UserContactsDAO) SelectListByOwnerListAndContactList(ctx context.Context, owners, contacts []int64) ([]dataobject.UserContactsDO, error) {
	if len(owners) == 0 || len(contacts) == 0 {
		return []dataobject.UserContactsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+contactColumns+` FROM user_contacts WHERE owner_user_id = ANY($1::bigint[]) AND contact_user_id = ANY($2::bigint[]) AND is_deleted = FALSE`, owners, contacts)
	if err != nil {
		return nil, err
	}
	return scanContactRows(rows)
}

func (d *UserContactsDAO) SelectListByOwnerListAndContactListWithCB(ctx context.Context, owners, contacts []int64, cb func(sz, i int, v *dataobject.UserContactsDO)) ([]dataobject.UserContactsDO, error) {
	result, err := d.SelectListByOwnerListAndContactList(ctx, owners, contacts)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, nil
}

func (d *UserContactsDAO) SelectReverseContactIDList(ctx context.Context, contactID int64) ([]int64, error) {
	rows, err := d.db.Query(ctx, `SELECT owner_user_id FROM user_contacts WHERE contact_user_id = $1 AND is_deleted = FALSE`, contactID)
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

func (d *UserContactsDAO) SelectUserReverseContactIdList(ctx context.Context, contactID int64) ([]int64, error) {
	return d.SelectReverseContactIDList(ctx, contactID)
}

func (d *UserContactsDAO) SelectUserReverseContactIdListWithCB(ctx context.Context, contactID int64, cb func(sz, i int, v int64)) ([]int64, error) {
	result, err := d.SelectReverseContactIDList(ctx, contactID)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i, id := range result {
			cb(len(result), i, id)
		}
	}
	return result, nil
}

func (d *UserContactsDAO) SelectReverseListByIdList(ctx context.Context, contactID int64, owners []int64) ([]dataobject.UserContactsDO, error) {
	if len(owners) == 0 {
		return []dataobject.UserContactsDO{}, nil
	}
	rows, err := d.db.Query(ctx, `SELECT `+contactColumns+` FROM user_contacts WHERE contact_user_id = $1 AND owner_user_id = ANY($2::bigint[]) AND is_deleted = FALSE`, contactID, owners)
	if err != nil {
		return nil, err
	}
	return scanContactRows(rows)
}

func (d *UserContactsDAO) SelectReverseListByIdListWithCB(ctx context.Context, contactID int64, owners []int64, cb func(sz, i int, v *dataobject.UserContactsDO)) ([]dataobject.UserContactsDO, error) {
	result, err := d.SelectReverseListByIdList(ctx, contactID, owners)
	if err != nil {
		return nil, err
	}
	if cb != nil {
		for i := range result {
			cb(len(result), i, &result[i])
		}
	}
	return result, nil
}

func (d *UserContactsDAO) UpdateMutual(ctx context.Context, mutual bool, ownerID, contactID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET mutual = $1 WHERE owner_user_id = $2 AND contact_user_id = $3 AND contact_user_id <> 0`, mutual, ownerID, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateMutualTx(ctx context.Context, tx DB, mutual bool, ownerID, contactID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET mutual = $1 WHERE owner_user_id = $2 AND contact_user_id = $3 AND contact_user_id <> 0`, mutual, ownerID, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateContactNameById(ctx context.Context, firstName, lastName string, id int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET contact_first_name = $1, contact_last_name = $2, is_deleted = FALSE WHERE id = $3`, firstName, lastName, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateContactNameByIdTx(ctx context.Context, tx DB, firstName, lastName string, id int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET contact_first_name = $1, contact_last_name = $2, is_deleted = FALSE WHERE id = $3`, firstName, lastName, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateContactName(ctx context.Context, firstName, lastName string, ownerID, contactID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET contact_first_name = $1, contact_last_name = $2, is_deleted = FALSE WHERE contact_user_id <> 0 AND owner_user_id = $3 AND contact_user_id = $4`, firstName, lastName, ownerID, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateContactNameTx(ctx context.Context, tx DB, firstName, lastName string, ownerID, contactID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET contact_first_name = $1, contact_last_name = $2, is_deleted = FALSE WHERE contact_user_id <> 0 AND owner_user_id = $3 AND contact_user_id = $4`, firstName, lastName, ownerID, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdatePhoneByContactId(ctx context.Context, phone string, contactID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET contact_phone = $1 WHERE contact_user_id = $2`, phone, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdatePhoneByContactIdTx(ctx context.Context, tx DB, phone string, contactID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET contact_phone = $1 WHERE contact_user_id = $2`, phone, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateCloseFriend(ctx context.Context, closeFriend bool, ownerID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET close_friend = $1 WHERE owner_user_id = $2 AND contact_user_id = ANY($3::bigint[])`, closeFriend, ownerID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateCloseFriendTx(ctx context.Context, tx DB, closeFriend bool, ownerID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET close_friend = $1 WHERE owner_user_id = $2 AND contact_user_id = ANY($3::bigint[])`, closeFriend, ownerID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateStoriesHidden(ctx context.Context, hidden bool, ownerID, contactID int64) (int64, error) {
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET stories_hidden = $1 WHERE owner_user_id = $2 AND contact_user_id = $3`, hidden, ownerID, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) UpdateStoriesHiddenTx(ctx context.Context, tx DB, hidden bool, ownerID, contactID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET stories_hidden = $1 WHERE owner_user_id = $2 AND contact_user_id = $3`, hidden, ownerID, contactID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) DeleteContacts(ctx context.Context, ownerID int64, contactIDs []int64) (int64, error) {
	if len(contactIDs) == 0 {
		return 0, nil
	}
	tag, err := d.db.Exec(ctx, `UPDATE user_contacts SET is_deleted = TRUE, mutual = FALSE, close_friend = FALSE, stories_hidden = FALSE WHERE owner_user_id = $1 AND contact_user_id = ANY($2::bigint[]) AND contact_user_id <> 0`, ownerID, contactIDs)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *UserContactsDAO) DeleteContactsTx(ctx context.Context, tx DB, ownerID int64, contactIDs []int64) (int64, error) {
	if len(contactIDs) == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE user_contacts SET is_deleted = TRUE, mutual = FALSE, close_friend = FALSE, stories_hidden = FALSE WHERE owner_user_id = $1 AND contact_user_id = ANY($2::bigint[]) AND contact_user_id <> 0`, ownerID, contactIDs)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
