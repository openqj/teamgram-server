package mysql_dao

import (
	"context"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

type unregisteredContactImporterCount struct {
	Phone     string `db:"phone"`
	Importers int64  `db:"importers"`
}

// SelectDistinctImporterCountsByPhoneList returns active, distinct importers for each phone.
func (dao *UnregisteredContactsDAO) SelectDistinctImporterCountsByPhoneList(ctx context.Context, phoneList []string) (map[string]int32, error) {
	counts := make(map[string]int32, len(phoneList))
	if len(phoneList) == 0 {
		return counts, nil
	}

	query := fmt.Sprintf("select phone, count(distinct importer_user_id) as importers from unregistered_contacts where imported = 0 and phone in (%s) group by phone", sqlx.InStringList(phoneList))
	var rows []unregisteredContactImporterCount
	if err := dao.db.QueryRowsPartial(ctx, &rows, query); err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.Phone] = int32(row.Importers)
	}
	return counts, nil
}
