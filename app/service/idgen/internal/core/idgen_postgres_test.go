package core_test

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
	idgenconfig "github.com/teamgram/teamgram-server/app/service/idgen/internal/config"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func idgenPostgresRPCFixture(t *testing.T) (idgen.RPCIdgenClient, *counter.CounterStore) {
	t.Helper()
	dsn := os.Getenv("IDGEN_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("IDGEN_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
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
	schemaName := fmt.Sprintf("idgen_rpc_test_%d", time.Now().UnixNano())
	schema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean idgen RPC schema: %v", err)
		}
	})
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
	_, source, _, _ := runtime.Caller(0)
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../../../teamgramd/deploy/sql/postgres/018_idgen_counters.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	databaseURL, err := url.Parse(dsn)
	if err != nil || databaseURL.Host == "" {
		t.Fatalf("test requires PostgreSQL URL: %v", err)
	}
	query := databaseURL.Query()
	query.Set("search_path", schemaName)
	databaseURL.RawQuery = query.Encode()
	runtimeDao := dao.New(idgenconfig.Config{NodeId: 1, Postgres: postgres.Config{DSN: databaseURL.String()}})
	t.Cleanup(runtimeDao.Close)
	store := runtimeDao.CounterStore
	if _, err := pool.Exec(ctx, `ALTER TABLE idgen_counters DROP COLUMN updated_at`); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("IDGen startup accepted an incomplete counter schema")
			}
		}()
		unexpected := dao.New(idgenconfig.Config{NodeId: 1, Postgres: postgres.Config{DSN: databaseURL.String()}})
		unexpected.Close()
	}()
	if _, err := pool.Exec(ctx, `ALTER TABLE idgen_counters ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	idgen.RegisterRPCIdgenServer(server, service.New(&svc.ServiceContext{Dao: runtimeDao}))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return idgen.NewRPCIdgenClient(conn), store
}

func inputCounter(key string) *idgen.InputId {
	return idgen.MakeTLInputSeqId(&idgen.InputId{Key: key}).To_InputId()
}

func TestIdgenPostgresRPCReadsAndOrderedVector(t *testing.T) {
	client, store := idgenPostgresRPCFixture(t)
	ctx := context.Background()
	current, err := client.IdgenGetCurrentSeqId(ctx, &idgen.TLIdgenGetCurrentSeqId{Key: counter.PtsKey(101)})
	if err != nil || current.V != 0 {
		t.Fatalf("unused current=%v err=%v", current, err)
	}
	var count int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM idgen_counters`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("current wrote %d rows: %v", count, err)
	}
	if _, err := client.IdgenSetCurrentSeqId(ctx, &idgen.TLIdgenSetCurrentSeqId{Key: "a", Id: 11}); err != nil {
		t.Fatal(err)
	}
	if next, err := client.IdgenGetNextSeqId(ctx, &idgen.TLIdgenGetNextSeqId{Key: "a"}); err != nil || next.V != 12 {
		t.Fatalf("next=%v err=%v", next, err)
	}
	if next, err := client.IdgenGetNextNSeqId(ctx, &idgen.TLIdgenGetNextNSeqId{Key: "a", N: 3}); err != nil || next.V != 15 {
		t.Fatalf("next N=%v err=%v", next, err)
	}
	got, err := client.IdgenGetNextIdValList(ctx, &idgen.TLIdgenGetNextIdValList{Id: []*idgen.InputId{
		inputCounter("b"),
		idgen.MakeTLInputNSeqId(&idgen.InputId{Key: "a", N: 2}).To_InputId(),
		idgen.MakeTLInputId(nil).To_InputId(),
		inputCounter("b"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Datas) != 4 || got.Datas[0].Id_INT64 != 1 || got.Datas[1].Id_INT64 != 17 || got.Datas[2].Id_INT64 <= 0 || got.Datas[3].Id_INT64 != 2 {
		t.Fatalf("ordered vector=%v", got)
	}
	read, err := client.IdgenGetCurrentSeqIdList(ctx, &idgen.TLIdgenGetCurrentSeqIdList{Id: []*idgen.InputId{
		inputCounter("b"), inputCounter("missing"), inputCounter("a"), inputCounter("b"),
	}})
	if err != nil || len(read.Datas) != 4 || read.Datas[0].Id_INT64 != 2 || read.Datas[1].Id_INT64 != 0 || read.Datas[2].Id_INT64 != 17 || read.Datas[3].Id_INT64 != 2 {
		t.Fatalf("current vector=%v err=%v", read, err)
	}
}

func TestIdgenPostgresRPCVectorFailureRollsBack(t *testing.T) {
	client, store := idgenPostgresRPCFixture(t)
	ctx := context.Background()
	if err := store.Set(ctx, "z", math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IdgenGetNextIdValList(ctx, &idgen.TLIdgenGetNextIdValList{Id: []*idgen.InputId{inputCounter("a"), inputCounter("z")}}); err == nil {
		t.Fatal("overflow vector succeeded")
	}
	if value, err := store.Current(ctx, "a"); err != nil || value != 0 {
		t.Fatalf("earlier vector allocation survived: %d %v", value, err)
	}
	if _, err := client.IdgenGetNextIdValList(ctx, &idgen.TLIdgenGetNextIdValList{Id: []*idgen.InputId{
		inputCounter("a"), idgen.MakeTLInputIds(&idgen.InputId{Num: -1}).To_InputId(),
	}}); err == nil {
		t.Fatal("negative Snowflake vector succeeded")
	}
	if _, err := client.IdgenGetNextNSeqId(ctx, &idgen.TLIdgenGetNextNSeqId{Key: "a", N: 0}); err == nil {
		t.Fatal("zero sequence allocation succeeded")
	}
}

func TestIdgenPostgresRPCOverlappingVectors(t *testing.T) {
	client, store := idgenPostgresRPCFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const count = 16
	errs := make(chan error, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(reverse bool) {
			defer workers.Done()
			inputs := []*idgen.InputId{inputCounter("a"), inputCounter("b")}
			if reverse {
				inputs[0], inputs[1] = inputs[1], inputs[0]
			}
			got, err := client.IdgenGetNextIdValList(ctx, &idgen.TLIdgenGetNextIdValList{Id: inputs})
			if err != nil {
				errs <- err
				return
			}
			if got.Datas[0].Id_INT64 != got.Datas[1].Id_INT64 {
				errs <- fmt.Errorf("non-atomic vector: %v", got)
			}
		}(i%2 == 0)
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for _, key := range []string{"a", "b"} {
		if value, err := store.Current(ctx, key); err != nil || value != count {
			t.Fatalf("counter %s=%d err=%v", key, value, err)
		}
	}
}
