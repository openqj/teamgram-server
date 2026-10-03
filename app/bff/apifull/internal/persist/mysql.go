package persist

import (
	"database/sql"
	"sync"

	_ "github.com/go-sql-driver/mysql"
)

// sqlStore keeps every key in the teamgram MySQL database.
type sqlStore struct {
	db *sql.DB
}

func (s *sqlStore) Get(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT v FROM apifull_kv WHERE k = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *sqlStore) Set(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO apifull_kv (k, v) VALUES (?, ?) ON DUPLICATE KEY UPDATE v = VALUES(v)`,
		key, value,
	)
	return err
}

func (s *sqlStore) CompareAndDelete(key, expected string) (bool, error) {
	r, err := s.db.Exec(`DELETE FROM apifull_kv WHERE k = ? AND v = ?`, key, expected)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

var openOnce sync.Mutex

// OpenMySQL creates apifull_kv if needed and points Default at it.
// A second call with the same process replaces Default.
func OpenMySQL(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS apifull_kv (
		k VARCHAR(191) NOT NULL PRIMARY KEY,
		v MEDIUMTEXT NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		_ = db.Close()
		return err
	}
	openOnce.Lock()
	Default = &sqlStore{db: db}
	openOnce.Unlock()
	return nil
}
