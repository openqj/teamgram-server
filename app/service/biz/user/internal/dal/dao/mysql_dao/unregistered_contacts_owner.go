package mysql_dao

import (
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteImporterByUserAndPhone marks only one user's active unregistered phone import as deleted.
func (dao *UnregisteredContactsDAO) DeleteImporterByUserAndPhone(ctx context.Context, phone string, userID int64) (int64, error) {
	result, err := dao.db.Exec(ctx, "update unregistered_contacts set imported = 1 where phone = ? and importer_user_id = ? and imported = 0", phone, userID)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in DeleteImporterByUserAndPhone(%q, %d), error: %v", phone, userID, err)
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in DeleteImporterByUserAndPhone(%q, %d), error: %v", phone, userID, err)
		return 0, err
	}
	return rowsAffected, nil
}
