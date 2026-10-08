package persist

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// postgresStore keeps APIFull process state in PostgreSQL. The KV table is
// intentionally small: larger domain records are owned by the domain store.
type postgresStore struct {
	db *sql.DB
}

func (s *postgresStore) Get(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *postgresStore) Set(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)
		 ON CONFLICT (k) DO UPDATE SET v = EXCLUDED.v`,
		key,
		value,
	)
	return err
}

func (s *postgresStore) CompareAndDelete(key, expected string) (bool, error) {
	result, err := s.db.Exec(`DELETE FROM apifull_kv WHERE k = $1 AND v = $2`, key, expected)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// keyLockID maps a key to PostgreSQL's transaction-scoped advisory lock
// namespace. A hash is sufficient here because collisions only serialize
// unrelated keys; they cannot change their values or correctness.
func keyLockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}

func (s *postgresStore) CompareAndSwap(key, expected, replacement string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// SELECT FOR UPDATE cannot lock an absent row in PostgreSQL. The
	// transaction-scoped advisory lock closes that race across processes before
	// the row is read or inserted.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock($1)`, keyLockID(key)); err != nil {
		return false, err
	}

	var current string
	err = tx.QueryRow(`SELECT v FROM apifull_kv WHERE k = $1 FOR UPDATE`, key).Scan(&current)
	if err == sql.ErrNoRows {
		if expected != "" {
			return false, nil
		}
		_, err = tx.Exec(`INSERT INTO apifull_kv (k, v) VALUES ($1, $2)`, key, replacement)
	} else if err != nil {
		return false, err
	} else if current != expected {
		return false, nil
	} else {
		_, err = tx.Exec(`UPDATE apifull_kv SET v = $1 WHERE k = $2`, replacement, key)
	}
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

var postgresOpenOnce sync.Mutex

var (
	postgresInsertIgnorePattern = regexp.MustCompile(`(?is)^\s*INSERT\s+IGNORE\s+INTO\b`)
	postgresDuplicatePattern    = regexp.MustCompile(`(?is)\s+ON\s+DUPLICATE\s+KEY\s+UPDATE\s+(.+?)\s*;?\s*$`)
	postgresValuesPattern       = regexp.MustCompile(`(?i)VALUES\s*\(\s*([a-z0-9_]+)\s*\)`)
	postgresInsertTablePattern  = regexp.MustCompile(`(?is)\bINSERT\s+INTO\s+([a-z0-9_]+)`)
	postgresGetLockPattern      = regexp.MustCompile(`(?is)GET_LOCK\s*\(\s*([^,()]+)\s*,\s*([^)]*)\)`)
	postgresReleaseLockPattern  = regexp.MustCompile(`(?is)RELEASE_LOCK\s*\(\s*([^()]+)\s*\)`)
)

var postgresConflictTargets = map[string]string{
	"apifull_channel":                 "id",
	"apifull_channel_member":          "channel_id, user_id",
	"apifull_channel_message":         "channel_id, message_id",
	"apifull_channel_read_state":      "user_id, channel_id",
	"apifull_call":                    "id",
	"apifull_group_call":              "id",
	"apifull_group_call_conference":   "call_id",
	"apifull_group_call_invite":       "call_id",
	"apifull_group_call_participant":  "call_id, user_id",
	"apifull_group_call_send_as":      "call_id, user_id",
	"apifull_group_call_settings":     "call_id",
	"apifull_group_call_subscription": "call_id, user_id",
	"apifull_payment_ledger":          "request_id, state",
	"apifull_report":                  "dedupe_key",
	"apifull_stars":                   "user_id",
	"apifull_stars_offer":             "kind, stars, store_product, currency, amount",
	"apifull_star_tx":                 "user_id, idem",
	"apifull_username":                "username",
}

// postgresCompatConnector keeps the existing APIFull query surface portable
// while its stores move from MySQL to PostgreSQL. The legacy code uses the
// database/sql `?` placeholder form; pgx requires PostgreSQL's `$1` form.
// Translating at the driver boundary lets the domain code migrate in focused
// slices without maintaining two copies of every query.
type postgresCompatConnector struct {
	connector driver.Connector
}

func (c postgresCompatConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return postgresCompatConn{Conn: conn}, nil
}

func (c postgresCompatConnector) Driver() driver.Driver { return c.connector.Driver() }

type postgresCompatConn struct {
	driver.Conn
}

func (c postgresCompatConn) Prepare(query string) (driver.Stmt, error) {
	return c.Conn.Prepare(rewritePostgresSQL(query))
}

func (c postgresCompatConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if conn, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return conn.PrepareContext(ctx, rewritePostgresSQL(query))
	}
	return c.Prepare(query)
}

func (c postgresCompatConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if conn, ok := c.Conn.(driver.ConnBeginTx); ok {
		return conn.BeginTx(ctx, opts)
	}
	return nil, driver.ErrSkip
}

func (c postgresCompatConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if conn, ok := c.Conn.(driver.ExecerContext); ok {
		return conn.ExecContext(ctx, rewritePostgresSQL(query), args)
	}
	return nil, driver.ErrSkip
}

func (c postgresCompatConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if conn, ok := c.Conn.(driver.QueryerContext); ok {
		return conn.QueryContext(ctx, rewritePostgresSQL(query), args)
	}
	return nil, driver.ErrSkip
}

func (c postgresCompatConn) Ping(ctx context.Context) error {
	if conn, ok := c.Conn.(driver.Pinger); ok {
		return conn.Ping(ctx)
	}
	return driver.ErrSkip
}

func (c postgresCompatConn) ResetSession(ctx context.Context) error {
	if conn, ok := c.Conn.(driver.SessionResetter); ok {
		return conn.ResetSession(ctx)
	}
	return nil
}

func (c postgresCompatConn) CheckNamedValue(value *driver.NamedValue) error {
	if conn, ok := c.Conn.(driver.NamedValueChecker); ok {
		return conn.CheckNamedValue(value)
	}
	return nil
}

// rewritePostgresSQL handles the small set of MySQL expressions still present
// in the transitional APIFull domain and then changes unquoted `?` markers to
// positional markers. Quoted values and SQL comments are copied verbatim so
// question marks in message text and LIKE patterns remain literal.
func rewritePostgresSQL(query string) string {
	// MySQL treats '\\' as one escaped backslash in a string literal. With
	// PostgreSQL standard_conforming_strings enabled, use an escape string so
	// LIKE ... ESCAPE continues to receive exactly one character.
	query = strings.ReplaceAll(query, `ESCAPE '\\'`, `ESCAPE E'\\'`)
	query = postgresGetLockPattern.ReplaceAllStringFunc(query, func(expression string) string {
		match := postgresGetLockPattern.FindStringSubmatch(expression)
		return "CASE WHEN " + match[2] + " < 0 THEN NULL::bigint WHEN pg_try_advisory_lock(hashtextextended(" + match[1] + ", 0)) THEN 1 ELSE 0 END"
	})
	query = postgresReleaseLockPattern.ReplaceAllStringFunc(query, func(expression string) string {
		match := postgresReleaseLockPattern.FindStringSubmatch(expression)
		return "CASE WHEN pg_advisory_unlock(hashtextextended(" + match[1] + ", 0)) THEN 1 ELSE 0 END"
	})
	if postgresInsertIgnorePattern.MatchString(query) {
		query = postgresInsertIgnorePattern.ReplaceAllString(query, "INSERT INTO")
		query = appendPostgresConflict(query, "DO NOTHING")
	}
	if match := postgresDuplicatePattern.FindStringSubmatch(query); len(match) == 2 {
		assignments := postgresValuesPattern.ReplaceAllString(match[1], "EXCLUDED.$1")
		target := ""
		if table := postgresInsertTablePattern.FindStringSubmatch(query); len(table) == 2 {
			target = postgresConflictTargets[strings.ToLower(table[1])]
		}
		if target != "" {
			target = " (" + target + ")"
		}
		query = postgresDuplicatePattern.ReplaceAllString(query, " ON CONFLICT"+target+" DO UPDATE SET "+assignments)
	}
	return rewritePostgresPlaceholders(query)
}

func appendPostgresConflict(query, action string) string {
	trimmed := strings.TrimSpace(query)
	semicolon := strings.HasSuffix(trimmed, ";")
	if semicolon {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	}
	trimmed += " ON CONFLICT " + action
	if semicolon {
		trimmed += ";"
	}
	return trimmed
}

func rewritePostgresPlaceholders(query string) string {
	var out strings.Builder
	out.Grow(len(query) + 8)
	placeholder := 1
	state := byte(0) // 0 normal, 1 single quote, 2 double quote, 3 line comment, 4 block comment
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch state {
		case 0:
			switch ch {
			case '\'':
				state = 1
			case '"':
				state = 2
			case '-':
				if i+1 < len(query) && query[i+1] == '-' {
					state = 3
				}
			case '/':
				if i+1 < len(query) && query[i+1] == '*' {
					state = 4
				}
			case '?':
				out.WriteByte('$')
				out.WriteString(strconv.Itoa(placeholder))
				placeholder++
				continue
			}
		case 1:
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					out.WriteByte(ch)
					i++
					out.WriteByte(query[i])
					continue
				}
				state = 0
			}
		case 2:
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					out.WriteByte(ch)
					i++
					out.WriteByte(query[i])
					continue
				}
				state = 0
			}
		case 3:
			if ch == '\n' {
				state = 0
			}
		case 4:
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				out.WriteByte(ch)
				i++
				out.WriteByte(query[i])
				state = 0
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

// OpenPostgres creates the APIFull KV table if needed and points Default at
// it. A second call with the same process replaces Default.
func OpenPostgres(dsn string) error {
	return openPostgres(dsn, true)
}

// OpenPostgresReadOnly connects to an already-provisioned APIFull schema
// without issuing DDL. Production processes can use this path when migrations
// are owned by deployment.
func OpenPostgresReadOnly(dsn string) error {
	return openPostgres(dsn, false)
}

func openPostgres(dsn string, createSchema bool) error {
	db, err := OpenPostgresDB(dsn)
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
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL
		)`); err != nil {
			_ = db.Close()
			return err
		}
	}
	postgresOpenOnce.Lock()
	Default = &postgresStore{db: db}
	postgresOpenOnce.Unlock()
	return nil
}

// OpenPostgresDB opens a PostgreSQL database/sql handle with APIFull's
// placeholder compatibility layer. Callers that own their schema can use it
// without changing the process-wide Store.
func OpenPostgresDB(dsn string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if config.ConnectTimeout <= 0 {
		config.ConnectTimeout = 10 * time.Second
	}
	db := sql.OpenDB(postgresCompatConnector{connector: stdlib.GetConnector(*config)})
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	return db, nil
}
