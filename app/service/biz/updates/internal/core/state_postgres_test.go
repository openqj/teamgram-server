package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func updateStatePostgresFixture(t *testing.T) (*UpdatesCore, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("UPDATES_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("UPDATES_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var version int
	if err := admin.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil || version/10000 != 18 {
		t.Fatalf("PostgreSQL 18 required: version=%d err=%v", version, err)
	}
	schema := pgx.Identifier{fmt.Sprintf("updates_state_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean state test schema: %v", err)
		}
	})
	for _, table := range []string{"user_pts_updates", "auth_seq_updates", "apifull_secret_user_state", "idgen_counters"} {
		name := pgx.Identifier{table}.Sanitize()
		if _, err := admin.Exec(ctx, `CREATE TABLE `+schema+`.`+name+` (LIKE public.`+name+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := &dao.Dao{Postgres: &dao.Postgres{Pool: pool}}
	return New(ctx, &svc.ServiceContext{Dao: store}), pool
}

func persistStateTestUpdate(t *testing.T, pool *pgxpool.Pool, userID, authID int64, position, date int32) {
	t.Helper()
	var update *mtproto.Update
	if authID == 0 {
		update = mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Pts_INT32: position, PtsCount: 1,
			Message_MESSAGE: mtproto.MakeTLMessage(&mtproto.Message{
				Id: position, Date: date, PeerId: mtproto.MakePeerUser(202), Message: fmt.Sprintf("message %d", position),
			}).To_Message(),
		}).To_Update()
	} else {
		update = mtproto.MakeTLUpdateUserStatus(&mtproto.Update{
			UserId: userID, Status_USERSTATUS: mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: date + 30}).To_UserStatus(),
		}).To_Update()
	}
	data, err := jsonx.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	if authID == 0 {
		_, err = pool.Exec(context.Background(), `INSERT INTO user_pts_updates (user_id,pts,pts_count,update_type,update_data,date2)
	 VALUES ($1,$2,1,$3,$4,$5)`, userID, position, mtproto.GetUpdateType(update), string(data), date)
	} else {
		_, err = pool.Exec(context.Background(), `INSERT INTO auth_seq_updates (auth_id,user_id,seq,update_type,update_data,date2)
	 VALUES ($1,$2,$3,$4,$5,$6)`, authID, userID, position, mtproto.GetUpdateType(update), string(data), date)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdatesPostgresStateIsReadOnlyAndCommitted(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	ctx := context.Background()
	first, err := core.UpdatesGetStateV2(&updates.TLUpdatesGetStateV2{UserId: 101, AuthKeyId: 11})
	if err != nil || first.Pts != 0 || first.Qts != 0 || first.Seq != -1 || first.Date <= 0 {
		t.Fatalf("initial state=%v err=%v", first, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM idgen_counters`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("getState wrote counters=%d err=%v", count, err)
	}
	persistStateTestUpdate(t, pool, 101, 0, 2, 100)
	persistStateTestUpdate(t, pool, 101, 11, 3, 100)
	persistStateTestUpdate(t, pool, 202, 0, 8, 100)
	persistStateTestUpdate(t, pool, 101, 12, 9, 100)
	if _, err := pool.Exec(ctx, `INSERT INTO apifull_secret_user_state (user_id,last_qts,confirmed_qts) VALUES (101,4,0);
	 INSERT INTO idgen_counters(key,value) VALUES ('pts_updates_ngen_101',99),('seq_updates_ngen_11',99)`); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO user_pts_updates (user_id,pts,pts_count,update_type,update_data,date2) VALUES (101,50,1,0,'{}',100)`); err != nil {
		t.Fatal(err)
	}
	got, err := core.UpdatesGetStateV2(&updates.TLUpdatesGetStateV2{UserId: 101, AuthKeyId: 11})
	if err != nil || got.Pts != 2 || got.Seq != 3 || got.Qts != 4 || got.Date < int32(time.Now().Unix())-1 {
		t.Fatalf("committed state=%v err=%v", got, err)
	}
	if err := got.Encode(mtproto.NewEncodeBuf(64), 229); err != nil {
		t.Fatalf("Layer 229 state: %v", err)
	}
}

func TestUpdatesPostgresDifferencePaginatesWithoutAdvancingUnseenPts(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	for i := int32(1); i <= 3; i++ {
		persistStateTestUpdate(t, pool, 101, 0, i, 100+i)
	}
	first, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{
		UserId: 101, AuthKeyId: 11, Pts: 0, Date: 99, PtsLimit: wrapperspb.Int32(1),
	})
	if err != nil || first.GetPredicateName() != updates.Predicate_differenceSlice || first.IntermediateState.Pts != 1 || first.IntermediateState.Date != 99 || len(first.NewMessages) != 1 {
		t.Fatalf("first page=%v err=%v", first, err)
	}
	last, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Pts: 1, Date: 99})
	if err != nil || last.GetPredicateName() != updates.Predicate_difference || last.State.Pts != 3 || len(last.NewMessages) != 2 {
		t.Fatalf("last page=%v err=%v", last, err)
	}
	empty, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Pts: 3, Date: int64(last.State.Date)})
	if err != nil || empty.GetPredicateName() != updates.Predicate_differenceEmpty || empty.State.Pts != 3 {
		t.Fatalf("empty page=%v err=%v", empty, err)
	}
	if _, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Pts: 4, Date: 99}); !errors.Is(err, mtproto.ErrPersistentTimestampInvalid) {
		t.Fatalf("future pts accepted: %v", err)
	}
	if _, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Pts: -1, Date: 99}); !errors.Is(err, mtproto.ErrPersistentTimestampInvalid) {
		t.Fatalf("negative pts accepted: %v", err)
	}
}

func TestUpdatesPostgresDifferenceKeepsTimestampGroups(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	for i := int32(1); i <= 4; i++ {
		date := int32(100)
		if i == 4 {
			date = 101
		}
		persistStateTestUpdate(t, pool, 101, 11, i, date)
	}
	first, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{
		UserId: 101, AuthKeyId: 11, Date: 99, PtsLimit: wrapperspb.Int32(1),
	})
	if err != nil || first.GetPredicateName() != updates.Predicate_differenceSlice || first.IntermediateState.Date != 100 || first.IntermediateState.Seq != 3 || len(first.OtherUpdates) != 3 {
		t.Fatalf("timestamp group page=%v err=%v", first, err)
	}
	last, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: 100})
	if err != nil || last.GetPredicateName() != updates.Predicate_difference || last.State.Seq != 4 || len(last.OtherUpdates) != 4 {
		t.Fatalf("timestamp group tail=%v err=%v", last, err)
	}
}

func TestUpdatesPostgresDifferenceFailsOnCorruptCommittedUpdate(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO user_pts_updates (user_id,pts,pts_count,update_type,update_data,date2)
	 VALUES (101,1,1,0,'corrupt',100)`); err != nil {
		t.Fatal(err)
	}
	if got, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: 99}); err == nil || got != nil {
		t.Fatalf("corrupt record skipped while state advances: %v %v", got, err)
	}
}

func TestUpdatesPostgresDifferenceRecoversSameSecondCommit(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	var date int32
	if err := pool.QueryRow(context.Background(), `SELECT floor(extract(epoch FROM clock_timestamp()))::integer`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	persistStateTestUpdate(t, pool, 101, 11, 1, date)
	first, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: int64(date - 1)})
	if err != nil || first.State.GetDate() < date || first.State.GetSeq() != 1 {
		t.Fatalf("first snapshot=%v err=%v", first, err)
	}
	boundary := first.State.GetDate()
	persistStateTestUpdate(t, pool, 101, 11, 2, boundary)
	late, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: int64(boundary), PtsLimit: wrapperspb.Int32(1)})
	if err != nil || late.GetPredicateName() != updates.Predicate_difference || late.State.GetSeq() != 2 || len(late.OtherUpdates) < 1 {
		t.Fatalf("same-second commit lost: %v err=%v", late, err)
	}
	persistStateTestUpdate(t, pool, 101, 11, 3, boundary+1)
	persistStateTestUpdate(t, pool, 101, 11, 4, boundary+2)
	next, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: int64(boundary), PtsLimit: wrapperspb.Int32(1)})
	if err != nil || next.GetPredicateName() != updates.Predicate_differenceSlice || next.IntermediateState.GetDate() != boundary+1 || next.IntermediateState.GetSeq() != 3 || len(next.OtherUpdates) != len(late.OtherUpdates)+1 {
		t.Fatalf("boundary replay prevented progress: %v err=%v", next, err)
	}
}

func TestUpdatesPostgresDateWaitsForPendingWriterAndConverges(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, counter.SeqKey(11)); err != nil {
		t.Fatal(err)
	}
	var date int32
	if err := tx.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp()))::integer-1`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	update := mtproto.MakeTLUpdateUserStatus(&mtproto.Update{
		UserId: 101, Status_USERSTATUS: mtproto.MakeTLUserStatusOnline(&mtproto.UserStatus{Expires: date + 30}).To_UserStatus(),
	}).To_Update()
	data, err := jsonx.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO auth_seq_updates (auth_id,user_id,seq,update_type,update_data,date2)
	 VALUES (11,101,1,$1,$2,$3)`, mtproto.GetUpdateType(update), string(data), date); err != nil {
		t.Fatal(err)
	}
	type reply struct {
		difference *updates.Difference
		err        error
	}
	done := make(chan reply, 1)
	go func() {
		reader := New(ctx, core.svcCtx)
		difference, err := reader.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: int64(date - 1)})
		done <- reply{difference, err}
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks
	 WHERE locktype='advisory' AND mode='ShareLock' AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case got := <-done:
			t.Fatalf("snapshot advanced while writer was pending: %v %v", got.difference, got.err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var got reply
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got.err != nil || got.difference.GetPredicateName() != updates.Predicate_difference || got.difference.State.GetSeq() != 1 || got.difference.State.GetDate() <= date || len(got.difference.OtherUpdates) != 1 {
		t.Fatalf("committed writer missing from snapshot: %v %v", got.difference, got.err)
	}
	empty, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: int64(got.difference.State.GetDate())})
	if err != nil || empty.GetPredicateName() != updates.Predicate_differenceEmpty || empty.State.GetSeq() != 1 || empty.State.GetDate() < got.difference.State.GetDate() {
		t.Fatalf("idle difference did not converge: %v %v", empty, err)
	}
	var unlocked bool
	if err := pool.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, counter.SeqKey(11)).Scan(&unlocked); err != nil || !unlocked {
		t.Fatalf("reader lock cleanup: unlocked=%v err=%v", unlocked, err)
	}
}

func TestUpdatesPostgresDifferenceTotalLimit(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	for i := int32(1); i <= 3; i++ {
		persistStateTestUpdate(t, pool, 101, 0, i, 100)
	}
	tooLong, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{
		UserId: 101, AuthKeyId: 11, Date: 99, PtsTotalLimit: wrapperspb.Int32(2),
	})
	if err != nil || tooLong.GetPredicateName() != updates.Predicate_differenceTooLong || tooLong.GetPts() != 3 {
		t.Fatalf("total limit=%v err=%v", tooLong, err)
	}
	atLimit, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{
		UserId: 101, AuthKeyId: 11, Date: 99, Pts: 1, PtsTotalLimit: wrapperspb.Int32(2),
	})
	if err != nil || atLimit.GetPredicateName() != updates.Predicate_difference || len(atLimit.NewMessages) != 2 {
		t.Fatalf("inclusive total limit=%v err=%v", atLimit, err)
	}
	for _, in := range []*updates.TLUpdatesGetDifferenceV2{
		{UserId: 101, AuthKeyId: 11, Date: 0},
		{UserId: 101, AuthKeyId: 11, Date: 99, PtsTotalLimit: wrapperspb.Int32(-1)},
	} {
		if got, err := core.UpdatesGetDifferenceV2(in); err == nil || got != nil {
			t.Fatalf("invalid timestamp/limit accepted: %v %v", got, err)
		}
	}
}

func TestUpdatesPostgresDifferencePreservesAuthoritativeUpdateOrder(t *testing.T) {
	core, pool := updateStatePostgresFixture(t)
	inputs := []*mtproto.Update{
		mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Pinned: true, Peer_PEER: mtproto.MakePeerUser(202), Messages: []int32{1}, Pts_INT32: 1, PtsCount: 1}).To_Update(),
		mtproto.MakeTLUpdatePinnedMessages(&mtproto.Update{Pinned: true, Peer_PEER: mtproto.MakePeerUser(303), Messages: []int32{2}, Pts_INT32: 2, PtsCount: 1}).To_Update(),
		mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{FolderPeers: []*mtproto.FolderPeer{{Peer: mtproto.MakePeerUser(202), FolderId: 1}}, Pts_INT32: 3, PtsCount: 1}).To_Update(),
		mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{FolderPeers: []*mtproto.FolderPeer{{Peer: mtproto.MakePeerUser(303), FolderId: 2}}, Pts_INT32: 4, PtsCount: 1}).To_Update(),
		mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{Peer_PEER: mtproto.MakePeerUser(202), MaxId: 5, Pts_INT32: 5, PtsCount: 1}).To_Update(),
		mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{Peer_PEER: mtproto.MakePeerUser(202), MaxId: 6, Pts_INT32: 6, PtsCount: 1}).To_Update(),
	}
	for _, update := range inputs {
		data, err := jsonx.Marshal(update)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), `INSERT INTO user_pts_updates (user_id,pts,pts_count,update_type,update_data,date2)
	 VALUES (101,$1,$2,$3,$4,100)`, update.Pts_INT32, update.PtsCount, mtproto.GetUpdateType(update), string(data)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := core.UpdatesGetDifferenceV2(&updates.TLUpdatesGetDifferenceV2{UserId: 101, AuthKeyId: 11, Date: 99})
	if err != nil || len(got.OtherUpdates) != len(inputs) || got.State.GetPts() != 6 {
		t.Fatalf("authority updates lost: %v err=%v", got, err)
	}
	for i, update := range got.OtherUpdates {
		if !proto.Equal(update, inputs[i]) {
			t.Fatalf("update %d changed: got=%v want=%v", i, update, inputs[i])
		}
	}
}

func TestUpdatesPostgresStartupRequiresCompleteSchema(t *testing.T) {
	_, pool := updateStatePostgresFixture(t)
	databaseURL, err := url.Parse(os.Getenv("UPDATES_POSTGRES_DSN"))
	if err != nil || databaseURL.Host == "" {
		t.Fatalf("test requires PostgreSQL URL: %v", err)
	}
	query := databaseURL.Query()
	query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
	databaseURL.RawQuery = query.Encode()
	startup := func() *dao.Dao { return dao.New(config.Config{Postgres: postgres.Config{DSN: databaseURL.String()}}) }
	store := startup()
	store.Postgres.Close()
	if _, err := pool.Exec(context.Background(), `ALTER TABLE auth_seq_updates DROP COLUMN update_data`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("updates startup accepted an incomplete update schema")
		}
	}()
	store = startup()
	store.Postgres.Close()
}
