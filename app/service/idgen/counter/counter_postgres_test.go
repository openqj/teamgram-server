package counter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func counterPostgresFixture(t *testing.T) *CounterStore {
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
	schema := pgx.Identifier{fmt.Sprintf("idgen_counter_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("clean counter test schema: %v", err)
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
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../../../teamgramd/deploy/sql/postgres/018_idgen_counters.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return NewCounterStore(pool)
}

func TestCounterPostgresCurrentDoesNotWrite(t *testing.T) {
	s := counterPostgresFixture(t)
	ctx := context.Background()
	if value, err := s.Current(ctx, PtsKey(101)); err != nil || value != 0 {
		t.Fatalf("unused current=%d err=%v", value, err)
	}
	values, err := s.CurrentList(ctx, []string{PtsKey(101), MessageBoxKey(101)})
	if err != nil || len(values) != 2 || values[0] != 0 || values[1] != 0 {
		t.Fatalf("unused vector=%v err=%v", values, err)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM idgen_counters`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("read initialized %d rows, err=%v", count, err)
	}
}

func TestCounterPostgresRollbackAndRange(t *testing.T) {
	s := counterPostgresFixture(t)
	ctx := context.Background()
	cause := errors.New("rollback mutation")
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		if value, err := NextOn(ctx, tx, PtsKey(101), 2); err != nil || value != 2 {
			t.Fatalf("allocation=%d err=%v", value, err)
		}
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("rollback error=%v", err)
	}
	if value, err := s.Next(ctx, PtsKey(101), 1); err != nil || value != 1 {
		t.Fatalf("next after rollback=%d err=%v", value, err)
	}
	for _, key := range []string{PtsKey(101), MessageBoxKey(101), SeqKey(201), "unbounded-counter"} {
		limit := int64(math.MaxInt32)
		if key == "unbounded-counter" {
			limit = math.MaxInt64
		}
		if err := s.Set(ctx, key, limit-1); err != nil {
			t.Fatal(err)
		}
		if value, err := s.Next(ctx, key, 1); err != nil || value != limit {
			t.Fatalf("last positive %s=%d err=%v", key, value, err)
		}
		if _, err := s.Next(ctx, key, 1); !errors.Is(err, ErrCounterOverflow) {
			t.Fatalf("overflow %s err=%v", key, err)
		}
		if value, err := s.Current(ctx, key); err != nil || value != limit {
			t.Fatalf("overflow changed %s=%d err=%v", key, value, err)
		}
	}
	for _, delta := range []int64{0, -1} {
		if _, err := s.Next(ctx, "invalid-delta", delta); !errors.Is(err, ErrInvalidCounter) {
			t.Fatalf("delta=%d err=%v", delta, err)
		}
	}
}

func TestCounterPostgresConcurrentAllocation(t *testing.T) {
	s := counterPostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const count = 32
	values := make(chan int, count)
	errors := make(chan error, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			value, err := s.Next(ctx, PtsKey(101), 1)
			if err != nil {
				errors <- err
				return
			}
			values <- int(value)
		}()
	}
	workers.Wait()
	close(values)
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	got := make([]int, 0, count)
	for value := range values {
		got = append(got, value)
	}
	sort.Ints(got)
	if len(got) != count {
		t.Fatalf("received %d counters", len(got))
	}
	for i, value := range got {
		if value != i+1 {
			t.Fatalf("noncontiguous allocation=%v", got)
		}
	}
}
