package dao

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/testutil"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func syncPostgresDAO(t *testing.T) *Dao {
	t.Helper()
	pool := testutil.PostgresPool(t)
	return &Dao{Postgres: &Postgres{Pool: pool, AuthSeqUpdatesDAO: postgres_dao.NewAuthSeqUpdatesDAO(pool), UserPtsUpdatesDAO: postgres_dao.NewUserPtsUpdatesDAO(pool)}}
}

func receiptContext(offset int64) context.Context {
	hash := sha256.Sum256([]byte(fmt.Sprint(offset)))
	return WithDeliveryReceipt(context.Background(), DeliveryReceipt{ConsumerGroup: "sync-test", Topic: "Sync-T", Partition: 0, Offset: offset, RequestHash: hash[:]})
}

func seqTestUpdate(userID int64) *mtproto.Update {
	return mtproto.MakeTLUpdatePeerSettings(&mtproto.Update{Peer_PEER: mtproto.MakePeerUser(userID), Settings: mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{}).To_PeerSettings()}).To_Update()
}

func TestPostgresSeqTransactionRollsBackOnJournalFailure(t *testing.T) {
	d := syncPostgresDAO(t)
	ctx := context.Background()
	if _, err := d.Pool.Exec(ctx, `ALTER TABLE auth_seq_updates ADD CONSTRAINT reject_auth CHECK (auth_id<>91)`); err != nil {
		t.Fatal(err)
	}
	data, _ := jsonx.Marshal(seqTestUpdate(7))
	if seq, err := d.AddSeqToUpdatesQueue(ctx, 91, 7, 0, data); err == nil || seq != 0 {
		t.Fatalf("failed journal returned seq=%d error=%v", seq, err)
	}
	value, err := counter.NewCounterStore(d.Pool).Current(ctx, counter.SeqKey(91))
	if err != nil || value != 0 {
		t.Fatalf("rolled-back counter=%d error=%v", value, err)
	}
	if _, err := d.Pool.Exec(ctx, `ALTER TABLE auth_seq_updates DROP CONSTRAINT reject_auth`); err != nil {
		t.Fatal(err)
	}
	if seq, err := d.AddSeqToUpdatesQueue(ctx, 91, 7, 0, data); err != nil || seq != 1 {
		t.Fatalf("retry seq=%d error=%v", seq, err)
	}
}

func TestPostgresConcurrentSeqCommitOrdering(t *testing.T) {
	d := syncPostgresDAO(t)
	const writers = 24
	data, _ := jsonx.Marshal(seqTestUpdate(7))
	var wg sync.WaitGroup
	results := make(chan int32, writers)
	errors := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seq, err := d.AddSeqToUpdatesQueue(context.Background(), 91, 7, 0, data)
			results <- seq
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var seqs []int
	for seq := range results {
		seqs = append(seqs, int(seq))
	}
	sort.Ints(seqs)
	for i, seq := range seqs {
		if seq != i+1 {
			t.Fatalf("noncontiguous seq=%v", seqs)
		}
	}
	var count, backwards int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER (WHERE date2<previous)
 FROM (SELECT date2,lag(date2) OVER (ORDER BY seq) AS previous FROM auth_seq_updates WHERE auth_id=91) events`).Scan(&count, &backwards); err != nil || count != writers || backwards != 0 {
		t.Fatalf("rows=%d backwards=%d error=%v", count, backwards, err)
	}
}

func TestPostgresSeqReaderLockAndCancellation(t *testing.T) {
	d := syncPostgresDAO(t)
	ctx := context.Background()
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	key := counter.SeqKey(91)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1,0))`, key); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock_shared(hashtextextended($1,0))`, key)
	}()
	data, _ := jsonx.Marshal(seqTestUpdate(7))
	waitCtx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	if seq, err := d.AddSeqToUpdatesQueue(waitCtx, 91, 7, 0, data); err == nil || seq != 0 {
		t.Fatalf("blocked writer seq=%d error=%v", seq, err)
	}
	value, err := counter.NewCounterStore(d.Pool).Current(ctx, key)
	if err != nil || value != 0 {
		t.Fatalf("cancelled writer advanced counter=%d error=%v", value, err)
	}
}

func TestPostgresSeqOverflowDoesNotWriteJournal(t *testing.T) {
	d := syncPostgresDAO(t)
	if err := counter.NewCounterStore(d.Pool).Set(context.Background(), counter.SeqKey(91), math.MaxInt32); err != nil {
		t.Fatal(err)
	}
	data, _ := jsonx.Marshal(seqTestUpdate(7))
	if seq, err := d.AddSeqToUpdatesQueue(context.Background(), 91, 7, 0, data); err == nil || seq != 0 {
		t.Fatalf("overflow seq=%d error=%v", seq, err)
	}
}

func TestPostgresPrepareUpdatesOfflineAuthAndReceiptReplay(t *testing.T) {
	d := syncPostgresDAO(t)
	for _, fixture := range []struct {
		userID, authID int64
		authType       int32
		deleted        bool
	}{
		{7, 91, mtproto.AuthKeyTypePerm, false}, {7, -92, mtproto.AuthKeyTypePerm, false},
		{7, 93, mtproto.AuthKeyTypeTemp, false}, {7, 94, mtproto.AuthKeyTypePerm, true}, {8, 95, mtproto.AuthKeyTypePerm, false},
	} {
		testutil.SeedAuth(t, d.Pool, fixture.userID, fixture.authID, fixture.authType, fixture.deleted)
	}
	ups := mtproto.MakeUpdatesByUpdates(seqTestUpdate(7), seqTestUpdate(8))
	original := proto.Clone(ups)
	ctx := receiptContext(1)
	prepared, err := d.PrepareUpdates(ctx, 7, ups, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.AuthUpdates) != 2 || prepared.Completed {
		t.Fatalf("active permanent auth map: %+v", prepared)
	}
	for authID, out := range prepared.AuthUpdates {
		if out.GetSeqStart() != 1 || out.GetSeq() != 2 || out.GetPredicateName() != mtproto.Predicate_updatesCombined || out.GetDate() <= 0 {
			t.Fatalf("auth %d sequence window: %s", authID, out)
		}
		buffer := mtproto.NewEncodeBuf(256)
		if err := out.Encode(buffer, 229); err != nil {
			t.Fatalf("Layer229 updates encode: %v", err)
		}
	}
	if !proto.Equal(original, ups) {
		t.Fatal("shared incoming update mutated")
	}
	replayed, err := d.PrepareUpdates(ctx, 7, ups, 0, nil)
	if err != nil || replayed.Completed {
		t.Fatalf("pending receipt replay error=%v completed=%v", err, replayed.Completed)
	}
	for authID, out := range prepared.AuthUpdates {
		if !proto.Equal(out, replayed.AuthUpdates[authID]) {
			t.Fatal("replay changed committed seq/date")
		}
	}
	if err := d.MarkDeliveryComplete(ctx, 7); err != nil {
		t.Fatal(err)
	}
	replayed, err = d.PrepareUpdates(ctx, 7, ups, 0, nil)
	if err != nil || !replayed.Completed {
		t.Fatalf("completed receipt error=%v", err)
	}
	var rows int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM auth_seq_updates`).Scan(&rows); err != nil || rows != 4 {
		t.Fatalf("replay duplicated rows=%d error=%v", rows, err)
	}
}

func TestPostgresPrepareBatchFailureRollsBackPtsSeqAndReceipt(t *testing.T) {
	d := syncPostgresDAO(t)
	testutil.SeedAuth(t, d.Pool, 7, 91, mtproto.AuthKeyTypePerm, false)
	testutil.SeedAuth(t, d.Pool, 7, 92, mtproto.AuthKeyTypePerm, false)
	if _, err := d.Pool.Exec(context.Background(), `ALTER TABLE auth_seq_updates ADD CONSTRAINT reject_auth CHECK(auth_id<>92)`); err != nil {
		t.Fatal(err)
	}
	pts := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{Messages: []int32{1}, Pts_INT32: 1, PtsCount: 1}).To_Update()
	if _, err := d.PrepareUpdates(receiptContext(2), 7, mtproto.MakeUpdatesByUpdates(pts, seqTestUpdate(7)), 0, nil); err == nil {
		t.Fatal("failed batch returned success")
	}
	for _, table := range []string{"auth_seq_updates", "user_pts_updates", "idgen_counters", "sync_delivery_receipts"} {
		var count int
		if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s has committed rows=%d error=%v", table, count, err)
		}
	}
}

func TestPostgresPtsRetryPreservesAuthoritativeMessage(t *testing.T) {
	d := syncPostgresDAO(t)
	original := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: mtproto.MakeTLMessage(&mtproto.Message{Id: 1, Message: "authoritative"}).To_Message(), Pts_INT32: 1, PtsCount: 1}).To_Update()
	if _, err := d.AddToPtsQueue(context.Background(), 7, 1, 1, original); err != nil {
		t.Fatal(err)
	}
	retry := proto.Clone(original).(*mtproto.Update)
	retry.Message_MESSAGE.Message = "different replay"
	if _, err := d.AddToPtsQueue(context.Background(), 7, 1, 1, retry); err != nil {
		t.Fatal(err)
	}
	row, err := d.UserPtsUpdatesDAO.SelectLastPts(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(mtproto.Update)
	if err := jsonx.UnmarshalFromString(row.UpdateData, decoded); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(original, decoded) {
		t.Fatal("Sync overwrote the authoritative message event")
	}
}

func TestPostgresReceiptIdentityConflictRejected(t *testing.T) {
	d := syncPostgresDAO(t)
	testutil.SeedAuth(t, d.Pool, 7, 91, mtproto.AuthKeyTypePerm, false)
	ups := mtproto.MakeUpdatesByUpdates(seqTestUpdate(7))
	if _, err := d.PrepareUpdates(receiptContext(3), 7, ups, 0, nil); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("other request"))
	ctx := WithDeliveryReceipt(context.Background(), DeliveryReceipt{ConsumerGroup: "sync-test", Topic: "Sync-T", Partition: 0, Offset: 3, RequestHash: hash[:]})
	if _, err := d.PrepareUpdates(ctx, 7, ups, 0, nil); err == nil {
		t.Fatal("reused identity accepted changed request")
	}
}

func TestPostgresConcurrentReceiptReplayAllocatesOnce(t *testing.T) {
	d := syncPostgresDAO(t)
	testutil.SeedAuth(t, d.Pool, 7, 91, mtproto.AuthKeyTypePerm, false)
	ups := mtproto.MakeUpdatesByUpdates(seqTestUpdate(7))
	const writers = 16
	var wg sync.WaitGroup
	results := make(chan *PreparedUpdates, writers)
	errors := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prepared, err := d.PrepareUpdates(receiptContext(4), 7, ups, 0, nil)
			results <- prepared
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *mtproto.Updates
	for result := range results {
		out := result.AuthUpdates[91]
		if first == nil {
			first = out
		}
		if out.GetSeq() != 1 || !proto.Equal(out, first) {
			t.Fatalf("concurrent replay changed sequence: %v", out)
		}
	}
	var rows int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM auth_seq_updates`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("concurrent replay rows=%d error=%v", rows, err)
	}
}

func TestPostgresForwardedUpdatesReuseCommittedWindow(t *testing.T) {
	d := syncPostgresDAO(t)
	testutil.SeedAuth(t, d.Pool, 7, 91, mtproto.AuthKeyTypePerm, false)
	prepared, err := d.PrepareUpdates(receiptContext(5), 7, mtproto.MakeUpdatesByUpdates(seqTestUpdate(7), seqTestUpdate(8)), 91, nil)
	if err != nil {
		t.Fatal(err)
	}
	forward := proto.Clone(prepared.AuthUpdates[91]).(*mtproto.Updates)
	forward.AuthKeyId = 91
	replayed, err := d.PrepareUpdates(receiptContext(6), 7, forward, 91, nil)
	if err != nil || !proto.Equal(replayed.AuthUpdates[91], forward) {
		t.Fatalf("forwarded seq window changed: %v", err)
	}
	forward.Updates[0] = seqTestUpdate(9)
	if _, err := d.PrepareUpdates(receiptContext(7), 7, forward, 91, nil); err == nil {
		t.Fatal("forwarded update without matching committed row accepted")
	}
	var rows int
	if err := d.Pool.QueryRow(context.Background(), `SELECT count(*) FROM auth_seq_updates`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("forwarded replay rows=%d error=%v", rows, err)
	}
}

func TestPostgresSeqClassificationAndShortPromotion(t *testing.T) {
	d := syncPostgresDAO(t)
	testutil.SeedAuth(t, d.Pool, 7, 91, mtproto.AuthKeyTypePerm, false)
	for i, update := range []*mtproto.Update{
		mtproto.MakeTLUpdateUserTyping(&mtproto.Update{UserId: 8}).To_Update(),
		mtproto.MakeTLUpdateChannelTooLong(&mtproto.Update{ChannelId: 1}).To_Update(),
		mtproto.MakeTLUpdateChannelTooLong(&mtproto.Update{ChannelId: 1, Pts_FLAGINT32: wrapperspb.Int32(2)}).To_Update(),
		mtproto.MakeTLUpdateReadChannelInbox(&mtproto.Update{ChannelId: 1, Pts_INT32: 2}).To_Update(),
		mtproto.MakeTLUpdateNewEncryptedMessage(&mtproto.Update{Qts: 2}).To_Update(),
	} {
		prepared, err := d.PrepareUpdates(receiptContext(int64(10+i)), 7, mtproto.MakeUpdatesByUpdates(update), 0, nil)
		if err != nil || prepared.AuthUpdates[91].GetSeq() != 0 {
			t.Fatalf("independent/ephemeral update %s consumed seq: %v", update.GetPredicateName(), err)
		}
	}
	short := mtproto.MakeTLUpdateShort(&mtproto.Updates{Update: seqTestUpdate(7), Date: 1}).To_Updates()
	prepared, err := d.PrepareUpdates(receiptContext(15), 7, short, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := prepared.AuthUpdates[91]
	if out.GetPredicateName() != mtproto.Predicate_updates || out.GetSeq() != 1 || len(out.GetUpdates()) != 1 || out.GetUpdate() != nil {
		t.Fatalf("short durable event lacks seq wrapper: %s", out)
	}
}

func TestPostgresSeqDateCannotMoveBackwards(t *testing.T) {
	d := syncPostgresDAO(t)
	data, _ := jsonx.Marshal(seqTestUpdate(7))
	if _, err := d.AddSeqToUpdatesQueue(context.Background(), 91, 7, 0, data); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Unix() + 120
	if _, err := d.Pool.Exec(context.Background(), `UPDATE auth_seq_updates SET date2=$1`, future); err != nil {
		t.Fatal(err)
	}
	if seq, err := d.AddSeqToUpdatesQueue(context.Background(), 91, 7, 0, data); err != nil || seq != 2 {
		t.Fatalf("next seq=%d error=%v", seq, err)
	}
	var date int64
	if err := d.Pool.QueryRow(context.Background(), `SELECT date2 FROM auth_seq_updates WHERE seq=2`).Scan(&date); err != nil || date < future {
		t.Fatalf("sequence date moved backwards=%d future=%d error=%v", date, future, err)
	}
}
