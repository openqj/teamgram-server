package twofa

import (
	"database/sql"
	"errors"
	"sync"

	_ "github.com/go-sql-driver/mysql"
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

type mysqlProofStore struct {
	db *sql.DB
}

var (
	mysqlStoresMu sync.Mutex
	mysqlStores   = map[string]*mysqlProofStore{}
)

func OpenMySQLProofStore(dsn string) (ProofStore, error) {
	mysqlStoresMu.Lock()
	defer mysqlStoresMu.Unlock()
	if store := mysqlStores[dsn]; store != nil {
		return store, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &mysqlProofStore{db: db}
	mysqlStores[dsn] = store
	return store, nil
}

func (s *mysqlProofStore) Get(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM apifull_kv WHERE k = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *mysqlProofStore) CompareAndDelete(key, expected string) (bool, error) {
	r, err := s.db.Exec(`DELETE FROM apifull_kv WHERE k = ? AND BINARY v = BINARY ?`, key, expected)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}
