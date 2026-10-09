package twofa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type redisKV interface {
	Get(key string) (string, error)
	Eval(script, key string, args ...any) (any, error)
}

type redisProofStore struct {
	kv redisKV
}

func NewRedisProofStore(kv redisKV) ProofStore {
	return redisProofStore{kv: kv}
}

func (s redisProofStore) Get(key string) (string, error) {
	return s.kv.Get(key)
}

func (s redisProofStore) CompareAndDelete(key, expected string) (bool, error) {
	v, err := s.kv.Eval(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end`, key, expected)
	if err != nil {
		return false, err
	}
	switch n := v.(type) {
	case int:
		return n == 1, nil
	case int64:
		return n == 1, nil
	default:
		return false, errors.New("unexpected Redis compare-and-delete result")
	}
}

var (
	postgresStoresMu sync.Mutex
	postgresStores   = map[string]*postgresProofStore{}
)

type postgresProofStore struct {
	db *sql.DB
}

// OpenPostgresProofStore opens the shared PostgreSQL APIFull KV store used by
// BFF authentication helpers. New deployments should use this path; the
// PostgreSQL is the sole database implementation for this store.
func OpenPostgresProofStore(dsn string) (ProofStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("twofa: PostgreSQL DSN is required")
	}
	postgresStoresMu.Lock()
	defer postgresStoresMu.Unlock()
	if store := postgresStores[dsn]; store != nil {
		return store, nil
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	var version int
	if err = db.QueryRowContext(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("twofa: verify PostgreSQL version: %w", err)
	}
	if version/10000 != 18 {
		_ = db.Close()
		return nil, fmt.Errorf("twofa: PostgreSQL 18 is required (server_version_num=%d)", version)
	}
	if _, err = db.ExecContext(ctx, `SELECT k,v FROM apifull_kv LIMIT 0`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("twofa: verify apifull_kv schema: %w", err)
	}
	store := &postgresProofStore{db: db}
	postgresStores[dsn] = store
	return store, nil
}

// ClosePostgresProofStores closes and forgets the process-wide PostgreSQL stores.
func ClosePostgresProofStores() error {
	postgresStoresMu.Lock()
	defer postgresStoresMu.Unlock()
	var closeErr error
	for dsn, store := range postgresStores {
		if store != nil && store.db != nil {
			if err := store.db.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		delete(postgresStores, dsn)
	}
	return closeErr
}

func (s *postgresProofStore) Get(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *postgresProofStore) CompareAndDelete(key, expected string) (bool, error) {
	r, err := s.db.Exec(`DELETE FROM apifull_kv WHERE k = $1 AND v = $2`, key, expected)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}
