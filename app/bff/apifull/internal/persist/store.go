package persist

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type compareAndDeleter interface {
	CompareAndDelete(key, expected string) (bool, error)
}

type compareAndSwapper interface {
	CompareAndSwap(key, expected, replacement string) (bool, error)
}

type evalStore interface {
	Eval(script, key string, args ...any) (any, error)
}

type updater interface {
	Update(key string, fn func(string) (string, error)) error
}

// Store is the process-wide blob store. Tests keep the memory default.
// The BFF process switches it to Redis in dao.New.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

type mem struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *mem) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		return "", nil
	}
	return s.m[key], nil
}

func (s *mem) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

func (s *mem) Update(key string, fn func(string) (string, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := ""
	if s.m != nil {
		current = s.m[key]
	}
	next, err := fn(current)
	if err != nil {
		return err
	}
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = next
	return nil
}

func (s *mem) CompareAndDelete(key, expected string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[key] != expected {
		return false, nil
	}
	delete(s.m, key)
	return true, nil
}

func (s *mem) CompareAndSwap(key, expected, replacement string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.m[key]
	if (!ok && expected != "") || (ok && current != expected) {
		return false, nil
	}
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = replacement
	return true, nil
}

// Default is memory until Use points it at Redis.
var Default Store = &mem{}

// Use replaces Default. Nil is ignored so tests stay on memory.
func Use(s Store) {
	if s != nil {
		Default = s
	}
}

func CompareAndDelete(key, expected string) (bool, error) {
	s, ok := Default.(compareAndDeleter)
	if ok {
		return s.CompareAndDelete(key, expected)
	}
	if redis, ok := Default.(evalStore); ok {
		v, err := redis.Eval(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end`, key, expected)
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
	return false, nil
}

// CompareAndSwap atomically replaces key when its current value matches
// expected. An empty expected value also matches an absent key, which lets
// callers initialize state without a separate existence race.
func CompareAndSwap(key, expected, replacement string) (bool, error) {
	if s, ok := Default.(compareAndSwapper); ok {
		return s.CompareAndSwap(key, expected, replacement)
	}
	if redis, ok := Default.(evalStore); ok {
		v, err := redis.Eval(`local current = redis.call('GET', KEYS[1]); if (not current and ARGV[1] == '') or current == ARGV[1] then redis.call('SET', KEYS[1], ARGV[2]); return 1 else return 0 end`, key, expected, replacement)
		if err != nil {
			return false, err
		}
		switch n := v.(type) {
		case int:
			return n == 1, nil
		case int64:
			return n == 1, nil
		default:
			return false, errors.New("unexpected Redis compare-and-swap result")
		}
	}
	// Test doubles and legacy stores may expose only Get/Set. Keep their
	// behavior compatible; production Redis stores implement the atomic
	// path above or in the database-specific store implementation.
	current, err := Default.Get(key)
	if err != nil {
		return false, err
	}
	if current != expected {
		return false, nil
	}
	return true, Default.Set(key, replacement)
}

// Update executes a read/modify/write operation under the store's
// transaction boundary. PostgreSQL and the in-memory test store implement the
// operation atomically; legacy stores fall back to a compare-and-swap loop.
func Update(key string, fn func(string) (string, error)) error {
	if s, ok := Default.(updater); ok {
		return s.Update(key, fn)
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := Default.Get(key)
		if err != nil {
			return err
		}
		next, err := fn(current)
		if err != nil {
			return err
		}
		ok, err := CompareAndSwap(key, current, next)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return errors.New("apifull: concurrent state update failed")
}

// GetDiscussionReadMaxID reads a discussion read cursor. PostgreSQL-backed
// processes use the typed cursor table; test and legacy stores retain the
// canonical KV key behavior.
func GetDiscussionReadMaxID(key string) (int32, error) {
	if store, ok := Default.(*postgresStore); ok {
		userID, peerType, peerID, messageID, ok := parseDiscussionReadKey(key)
		if !ok {
			return 0, fmt.Errorf("apifull: invalid discussion read key %q", key)
		}
		var value int32
		err := store.db.QueryRow(`SELECT read_max_id FROM apifull_discussion_read_state
			WHERE user_id=$1 AND peer_type=$2 AND peer_id=$3 AND message_id=$4`,
			userID, peerType, peerID, messageID).Scan(&value)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return value, err
	}
	raw, err := Default.Get(key)
	if err != nil || raw == "" {
		return 0, err
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}

// AdvanceDiscussionReadMaxID advances a discussion cursor atomically. The
// PostgreSQL path performs the monotonic update in one transaction so two
// BFF instances cannot overwrite a newer read marker with an older one.
func AdvanceDiscussionReadMaxID(key string, readMaxID int32) error {
	if readMaxID < 0 {
		return fmt.Errorf("apifull: negative discussion read cursor")
	}
	if store, ok := Default.(*postgresStore); ok {
		userID, peerType, peerID, messageID, ok := parseDiscussionReadKey(key)
		if !ok {
			return fmt.Errorf("apifull: invalid discussion read key %q", key)
		}
		tx, err := store.db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err = tx.Exec(`INSERT INTO apifull_discussion_read_state
			(user_id, peer_type, peer_id, message_id, read_max_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (user_id, peer_type, peer_id, message_id) DO UPDATE
			SET read_max_id=GREATEST(apifull_discussion_read_state.read_max_id, EXCLUDED.read_max_id),
				updated_at=CURRENT_TIMESTAMP`,
			userID, peerType, peerID, messageID, readMaxID); err != nil {
			return err
		}
		return tx.Commit()
	}
	return Update(key, func(raw string) (string, error) {
		current := int64(0)
		if raw != "" {
			value, err := strconv.ParseInt(raw, 10, 32)
			if err != nil {
				return "", err
			}
			current = value
		}
		if int64(readMaxID) <= current {
			return strconv.FormatInt(current, 10), nil
		}
		return strconv.FormatInt(int64(readMaxID), 10), nil
	})
}

func parseDiscussionReadKey(key string) (userID int64, peerType int32, peerID int64, messageID int32, ok bool) {
	parts := strings.Split(key, ":")
	if len(parts) != 6 || parts[0] != "discussion" || parts[1] != "read" {
		return 0, 0, 0, 0, false
	}
	u, err1 := strconv.ParseInt(parts[2], 10, 64)
	p, err2 := strconv.ParseInt(parts[3], 10, 32)
	i, err3 := strconv.ParseInt(parts[4], 10, 64)
	m, err4 := strconv.ParseInt(parts[5], 10, 32)
	return u, int32(p), i, int32(m), err1 == nil && err2 == nil && err3 == nil && err4 == nil && u > 0 && p > 0 && i > 0 && m > 0
}
