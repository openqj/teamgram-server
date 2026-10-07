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

func (s *sqlStore) CompareAndSwap(key, expected, replacement string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRow(`SELECT v FROM apifull_kv WHERE k=? FOR UPDATE`, key).Scan(&current)
	if err == sql.ErrNoRows {
		if expected != "" {
			return false, nil
		}
		if _, err = tx.Exec(`INSERT INTO apifull_kv (k, v) VALUES (?, ?)`, key, replacement); err != nil {
			return false, err
		}
	} else if err != nil {
		return false, err
	} else {
		if current != expected {
			return false, nil
		}
		if _, err = tx.Exec(`UPDATE apifull_kv SET v=? WHERE k=?`, replacement, key); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

var openOnce sync.Mutex

// OpenMySQL creates apifull_kv if needed and points Default at it.
// A second call with the same process replaces Default.
func OpenMySQL(dsn string) error {
	return openMySQL(dsn, true)
}

// OpenMySQLReadOnly connects to an already-provisioned APIFull schema without
// issuing DDL. Production processes use this path so schema ownership stays
// with the deployment step rather than application startup.
func OpenMySQLReadOnly(dsn string) error {
	return openMySQL(dsn, false)
}

func openMySQL(dsn string, createSchema bool) error {
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
	if createSchema {
		if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS apifull_kv (
		k VARCHAR(191) NOT NULL PRIMARY KEY,
		v MEDIUMTEXT NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			_ = db.Close()
			return err
		}
	}
	openOnce.Lock()
	Default = &sqlStore{db: db}
	openOnce.Unlock()
	return nil
}
