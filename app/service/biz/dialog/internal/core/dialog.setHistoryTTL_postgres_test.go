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

func TestDialogSetHistoryTTLPostgresRoundTrip(t *testing.T) {
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
	peerID := userID + 1
	const peerType int32 = mtproto.PEER_USER
	for _, row := range [][2]int64{{userID, peerID}, {peerID, userID}} {
		if _, err := pg.Pool.Exec(ctx, `INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id) VALUES ($1, $2, $3, $4)`, row[0], peerType, row[1], row[1]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pg.Pool.Exec(ctx, `DELETE FROM dialogs WHERE (user_id = $1 AND peer_id = $2) OR (user_id = $2 AND peer_id = $1)`, userID, peerID)
	})

	c := &DialogCore{ctx: ctx, svcCtx: &svc.ServiceContext{Dao: &dao.Dao{Postgres: pg}}}
	result, err := c.DialogSetHistoryTTL(&dialog.TLDialogSetHistoryTTL{
		UserId:    userID,
		PeerType:  peerType,
		PeerId:    peerID,
		TtlPeriod: 86400,
	})
	if err != nil || !mtproto.FromBool(result) {
		t.Fatalf("set history TTL = (%v, %v), want BoolTrue", result, err)
	}

	var periods []int32
	rows, err := pg.Pool.Query(ctx, `SELECT ttl_period FROM dialogs WHERE (user_id = $1 AND peer_id = $2) OR (user_id = $2 AND peer_id = $1) ORDER BY user_id`, userID, peerID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var period int32
		if err := rows.Scan(&period); err != nil {
			t.Fatal(err)
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(periods) != 2 || periods[0] != 86400 || periods[1] != 86400 {
		t.Fatalf("persisted TTL periods = %v, want [86400 86400]", periods)
	}
}
