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

func dialogTagsPostgres(t *testing.T) *dao.Postgres {
	t.Helper()
	dsn := os.Getenv("DIALOG_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DIALOG_POSTGRES_DSN must point to an isolated PostgreSQL 18 test database")
	}
	pg, err := dao.NewPostgres(postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	return pg
}

func TestDialogToggleDialogFilterTagsAcknowledgesDisable(t *testing.T) {
	pg := dialogTagsPostgres(t)
	ctx := context.Background()
	uid := time.Now().UnixNano()
	c := &DialogCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			Postgres: pg,
		}},
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, "delete from dialog_filter_tags where user_id = $1", uid)
	})

	for _, enabled := range []*mtproto.Bool{mtproto.BoolTrue, mtproto.BoolFalse} {
		result, err := c.DialogToggleDialogFilterTags(&dialog.TLDialogToggleDialogFilterTags{
			UserId:  uid,
			Enabled: enabled,
		})
		if err != nil || !mtproto.FromBool(result) {
			t.Fatalf("toggle enabled=%v = (%v, %v), want true", mtproto.FromBool(enabled), result, err)
		}
	}

	stored, err := c.DialogGetDialogFilterTags(&dialog.TLDialogGetDialogFilterTags{UserId: uid})
	if err != nil {
		t.Fatal(err)
	}
	if mtproto.FromBool(stored) {
		t.Fatal("disabled dialog-filter tags were read back as enabled")
	}
	result, err := c.DialogToggleDialogFilterTags(&dialog.TLDialogToggleDialogFilterTags{UserId: uid, Enabled: mtproto.BoolTrue})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("re-enable = (%v, %v), want true", result, err)
	}
	stored, err = c.DialogGetDialogFilterTags(&dialog.TLDialogGetDialogFilterTags{UserId: uid})
	if err != nil || !mtproto.FromBool(stored) {
		t.Fatalf("read after re-enable = (%v, %v), want true", stored, err)
	}
}
