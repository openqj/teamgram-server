package core

import (
	"context"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/svc"
)

func dialogHistoryTTLAuditDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN must point to the isolated teamgram_audit database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.DBName != "teamgram_audit" || config.Addr != "127.0.0.1:13306" {
		t.Fatalf("refusing non-audit MySQL target %q at %q", config.DBName, config.Addr)
	}
	return sqlx.NewMySQL(&sqlx.Config{DSN: dsn})
}

func TestDialogSetHistoryTTLRoundTrip(t *testing.T) {
	db := dialogHistoryTTLAuditDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS dialogs (
		user_id bigint NOT NULL,
		peer_type int NOT NULL,
		peer_id bigint NOT NULL,
		ttl_period int NOT NULL DEFAULT 0,
		PRIMARY KEY (user_id, peer_type, peer_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		t.Fatal(err)
	}
	userID := time.Now().UnixNano()
	peerID := userID + 1
	const peerType int32 = mtproto.PEER_USER
	for _, row := range [][3]interface{}{{userID, peerType, peerID}, {peerID, peerType, userID}} {
		if _, err := db.Exec(ctx, "INSERT INTO dialogs (user_id, peer_type, peer_id, ttl_period) VALUES (?, ?, ?, 0)", row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, "DELETE FROM dialogs WHERE (user_id = ? AND peer_id = ?) OR (user_id = ? AND peer_id = ?)", userID, peerID, peerID, userID)
	})

	core := &DialogCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Mysql: &dao.Mysql{
			DB:         db,
			DialogsDAO: mysql_dao.NewDialogsDAO(db),
		}}},
	}
	result, err := core.DialogSetHistoryTTL(&dialog.TLDialogSetHistoryTTL{
		UserId:    userID,
		PeerType:  peerType,
		PeerId:    peerID,
		TtlPeriod: 86400,
	})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set history TTL = (%v, %v), want BoolTrue", result, err)
	}
	var periods []int32
	if err := db.QueryRowsPartial(ctx, &periods, "SELECT ttl_period FROM dialogs WHERE (user_id = ? AND peer_id = ?) OR (user_id = ? AND peer_id = ?) ORDER BY user_id", userID, peerID, peerID, userID); err != nil {
		t.Fatal(err)
	}
	if len(periods) != 2 || periods[0] != 86400 || periods[1] != 86400 {
		t.Fatalf("persisted TTL periods = %v, want [86400 86400]", periods)
	}
}
