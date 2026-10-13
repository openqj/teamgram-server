package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi/store"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/testutil"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/status/status"
)

func TestSyncPushBotUpdatesPaymentRetryUsesCompletedReceipt(t *testing.T) {
	pool := testutil.PostgresPool(t)
	ctx := context.Background()
	_, source, _, _ := runtime.Caller(0)
	for _, migration := range []string{"003_biz_user.sql", "081_biz_bot_api.sql", "091_biz_bot_api_update_dedup.sql"} {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../../../teamgramd/deploy/sql/postgres", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	const botID int64 = 920301
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,phone,is_bot) VALUES($1,$2,TRUE)`, botID, "+19999203001"); err != nil {
		t.Fatal(err)
	}
	statusStub := &syncStatusStub{sessions: &status.UserSessionEntryList{}}
	core := New(ctx, &svc.ServiceContext{Dao: &dao.Dao{
		Postgres:     &dao.Postgres{Pool: pool, BotAPI: store.NewWithPool(pool)},
		StatusClient: statusStub,
	}})
	updates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateBotShippingQuery(&mtproto.Update{
		QueryId_INT64:   920401,
		UserId:          920302,
		Payload_BYTES:   []byte("invoice"),
		ShippingAddress: mtproto.MakeTLPostAddress(&mtproto.PostAddress{CountryIso2: "US"}).To_PostAddress(),
	}).To_Update())
	request := &sync.TLSyncPushBotUpdates{UserId: botID, Updates: updates}
	if result, err := core.SyncPushBotUpdates(request); err != nil || result == nil {
		t.Fatalf("first bot update result=%v err=%v", result, err)
	}
	if result, err := core.SyncPushBotUpdates(request); err != nil || result == nil {
		t.Fatalf("replayed bot update result=%v err=%v", result, err)
	}
	var queueRows, receiptRows, completed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bot_api_updates WHERE bot_id=$1`, botID).Scan(&queueRows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),CASE WHEN coalesce(bool_and(delivered),FALSE) THEN 1 ELSE 0 END FROM sync_delivery_receipts WHERE user_id=$1`, botID).Scan(&receiptRows, &completed); err != nil {
		t.Fatal(err)
	}
	if queueRows != 1 || receiptRows != 1 || completed != 1 {
		t.Fatalf("retry duplicated delivery queue=%d receipts=%d completed=%d", queueRows, receiptRows, completed)
	}

	statusStub.err = errors.New("session unavailable")
	failedUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateBotShippingQuery(&mtproto.Update{
		QueryId_INT64:   920402,
		UserId:          920302,
		Payload_BYTES:   []byte("invoice-2"),
		ShippingAddress: mtproto.MakeTLPostAddress(&mtproto.PostAddress{CountryIso2: "US"}).To_PostAddress(),
	}).To_Update())
	failedRequest := &sync.TLSyncPushBotUpdates{UserId: botID, Updates: failedUpdates}
	if result, err := core.SyncPushBotUpdates(failedRequest); err == nil || result != nil {
		t.Fatalf("failed fan-out result=%v err=%v", result, err)
	}
	statusStub.err = nil
	if result, err := core.SyncPushBotUpdates(failedRequest); err != nil || result == nil {
		t.Fatalf("failed fan-out retry result=%v err=%v", result, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),CASE WHEN coalesce(bool_and(delivered),FALSE) THEN 1 ELSE 0 END FROM sync_delivery_receipts WHERE user_id=$1`, botID).Scan(&receiptRows, &completed); err != nil {
		t.Fatal(err)
	}
	if receiptRows != 2 || completed != 1 {
		t.Fatalf("failed fan-out retry receipts=%d all_completed=%d", receiptRows, completed)
	}
}
