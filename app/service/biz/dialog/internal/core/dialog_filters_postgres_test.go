package core

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

func TestDialogFiltersPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("DIALOG_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DIALOG_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}
	pg, err := dao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)

	ctx := context.Background()
	var version int
	if err := pg.Pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 18 {
		t.Fatalf("PostgreSQL version is %d, want 18.x", version)
	}

	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM dialog_filters WHERE user_id = $1`, userID)
	})
	c := New(ctx, &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}})
	filter := func(id int32) *mtproto.DialogFilter {
		return mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
			Id: id, Title_STRING: "filter", PinnedPeers: []*mtproto.InputPeer{},
			IncludePeers: []*mtproto.InputPeer{}, ExcludePeers: []*mtproto.InputPeer{},
		}).To_DialogFilter()
	}

	for _, id := range []int32{2, 3} {
		result, err := c.DialogInsertOrUpdateDialogFilter(&dialog.TLDialogInsertOrUpdateDialogFilter{
			UserId: userID, Id: id, DialogFilter: filter(id),
		})
		if err != nil || !mtproto.FromBool(result) {
			t.Fatalf("insert filter %d = (%v, %v), want true", id, result, err)
		}
	}
	filters, err := c.DialogGetDialogFilters(&dialog.TLDialogGetDialogFilters{UserId: userID})
	if err != nil || len(filters.GetDatas()) != 2 {
		t.Fatalf("initial filters = (%+v, %v), want two rows", filters, err)
	}
	other, err := c.DialogGetDialogFilters(&dialog.TLDialogGetDialogFilters{UserId: userID + 1})
	if err != nil || len(other.GetDatas()) != 0 {
		t.Fatalf("other user's filters = (%+v, %v), want empty", other, err)
	}

	if result, err := c.DialogUpdateDialogFiltersOrder(&dialog.TLDialogUpdateDialogFiltersOrder{UserId: userID, Order: []int32{2, 3}}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("reorder = (%v, %v), want true", result, err)
	}
	filters, err = c.DialogGetDialogFilters(&dialog.TLDialogGetDialogFilters{UserId: userID})
	if err != nil || len(filters.GetDatas()) != 2 || filters.GetDatas()[0].GetId() != 2 || filters.GetDatas()[1].GetId() != 3 {
		t.Fatalf("reordered filters = (%+v, %v), want [2,3]", filters, err)
	}

	if result, err := c.DialogDeleteDialogFilter(&dialog.TLDialogDeleteDialogFilter{UserId: userID, Id: 2}); err != nil || !mtproto.FromBool(result) {
		t.Fatalf("delete = (%v, %v), want true", result, err)
	}
	filters, err = c.DialogGetDialogFilters(&dialog.TLDialogGetDialogFilters{UserId: userID})
	if err != nil || len(filters.GetDatas()) != 1 || filters.GetDatas()[0].GetId() != 3 {
		t.Fatalf("filters after delete = (%+v, %v), want [3]", filters, err)
	}
}
