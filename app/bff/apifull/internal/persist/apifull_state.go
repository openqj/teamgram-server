package persist

import (
	"database/sql"
	"fmt"
	"sync"
)

// ErrMessageProviderUnavailable is returned by the takeout range provider
// when APIFull is running with a test/legacy Store instead of PostgreSQL.
var ErrMessageProviderUnavailable = fmt.Errorf("message range provider requires PostgreSQL")

type MessageSplitRange struct {
	MinID int32
	MaxID int32
}

// GetMessageSplitRanges reads the authoritative message ID span for one user
// and partitions it into bounded ranges suitable for takeout workers. Holes
// are intentionally covered by the enclosing ranges; workers still apply the
// normal per-message visibility checks.
func GetMessageSplitRanges(userID int64) ([]MessageSplitRange, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, ErrMessageProviderUnavailable
	}
	var minID, maxID sql.NullInt64
	err := store.db.QueryRow(`SELECT MIN(user_message_box_id), MAX(user_message_box_id)
		FROM messages WHERE user_id = $1 AND deleted = FALSE`, userID).Scan(&minID, &maxID)
	if err != nil {
		return nil, err
	}
	if !minID.Valid || !maxID.Valid || minID.Int64 <= 0 || maxID.Int64 < minID.Int64 {
		return []MessageSplitRange{}, nil
	}
	const maxRangeSize int64 = 100000
	ranges := make([]MessageSplitRange, 0, (maxID.Int64-minID.Int64)/maxRangeSize+1)
	for start := minID.Int64; start <= maxID.Int64; {
		end := start + maxRangeSize - 1
		if end < start || end > maxID.Int64 {
			end = maxID.Int64
		}
		if start > int64(^uint32(0)>>1) || end > int64(^uint32(0)>>1) {
			return nil, fmt.Errorf("message range exceeds TL int32: %d-%d", start, end)
		}
		ranges = append(ranges, MessageSplitRange{MinID: int32(start), MaxID: int32(end)})
		if end == maxID.Int64 {
			break
		}
		start = end + 1
	}
	return ranges, nil
}

const (
	takeoutKey           = "takeout:"
	wallpaperKey         = "wallpaper:"
	wallpaperUploadedKey = "wallpaper:uploaded:"
)

// The in-memory store is retained for focused core tests. Keep its takeout
// read/modify/write operations atomic so tests exercise the same single-active
// session invariant as the PostgreSQL implementation.
var takeoutMemoryMu sync.Mutex

func wallpaperStateKey(userID int64, uploaded bool) string {
	if uploaded {
		return fmt.Sprintf("%s%d", wallpaperUploadedKey, userID)
	}
	return fmt.Sprintf("%s%d", wallpaperKey, userID)
}

// LoadWallpaperList returns the JSON encoded TL WallPaper vector. PostgreSQL
// callers read the row directly; memory/legacy stores retain test behavior.
func LoadWallpaperList(userID int64, uploaded bool) (string, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.loadWallpaperList(userID, uploaded)
	}
	return Default.Get(wallpaperStateKey(userID, uploaded))
}

// StoreWallpaperList replaces a user's list under a transaction in PostgreSQL.
func StoreWallpaperList(userID int64, uploaded bool, value string) error {
	if store, ok := Default.(*postgresStore); ok {
		return store.storeWallpaperList(userID, uploaded, value)
	}
	return Default.Set(wallpaperStateKey(userID, uploaded), value)
}

// MutateWallpaperList applies a read-modify-write while holding a database
// advisory lock. This is the production path for save/install/upload/reset.
func MutateWallpaperList(userID int64, uploaded bool, mutate func(string) (string, error)) (string, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.mutateWallpaperList(userID, uploaded, mutate)
	}
	current, err := Default.Get(wallpaperStateKey(userID, uploaded))
	if err != nil {
		return "", err
	}
	next, err := mutate(current)
	if err != nil {
		return "", err
	}
	if err := Default.Set(wallpaperStateKey(userID, uploaded), next); err != nil {
		return "", err
	}
	return next, nil
}

func wallpaperLockID(userID int64, uploaded bool) string {
	return fmt.Sprintf("apifull-wallpaper:%d:%t", userID, uploaded)
}

func (s *postgresStore) loadWallpaperList(userID int64, uploaded bool) (string, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT wallpapers FROM apifull_wallpaper_state
		WHERE user_id = $1 AND uploaded = $2`, userID, uploaded).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *postgresStore) storeWallpaperList(userID int64, uploaded bool, value string) error {
	_, err := s.mutateWallpaperList(userID, uploaded, func(string) (string, error) { return value, nil })
	return err
}

func (s *postgresStore) mutateWallpaperList(userID int64, uploaded bool, mutate func(string) (string, error)) (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, wallpaperLockID(userID, uploaded)); err != nil {
		return "", err
	}
	var raw []byte
	err = tx.QueryRow(`SELECT wallpapers FROM apifull_wallpaper_state
		WHERE user_id = $1 AND uploaded = $2 FOR UPDATE`, userID, uploaded).Scan(&raw)
	if err == sql.ErrNoRows {
		raw = []byte("[]")
	} else if err != nil {
		return "", err
	}
	next, err := mutate(string(raw))
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(`INSERT INTO apifull_wallpaper_state(user_id, uploaded, wallpapers, updated_at)
		VALUES ($1, $2, $3::jsonb, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, uploaded) DO UPDATE SET wallpapers = EXCLUDED.wallpapers,
		updated_at = CURRENT_TIMESTAMP`, userID, uploaded, next)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return next, nil
}

// MutateTakeout updates one user's active takeout session. PostgreSQL allocates
// the session ID from the identity sequence while holding a user-scoped lock.
func MutateTakeout(userID int64, mutate func(id int64, current string) (string, error)) (int64, string, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.mutateTakeout(userID, mutate)
	}
	takeoutMemoryMu.Lock()
	defer takeoutMemoryMu.Unlock()
	key := takeoutKey + fmt.Sprintf("%d", userID)
	current, err := Default.Get(key)
	if err != nil {
		return 0, "", err
	}
	var id int64
	if current != "" {
		// The callback owns decoding the payload. A zero ID asks it to allocate.
		// The memory path only exists for tests and uses the existing allocator.
		id = 0
	}
	next, err := mutate(id, current)
	if err != nil {
		return 0, "", err
	}
	if err := Default.Set(key, next); err != nil {
		return 0, "", err
	}
	return id, next, nil
}

func LoadTakeout(userID int64) (string, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.loadTakeout(userID)
	}
	return Default.Get(takeoutKey + fmt.Sprintf("%d", userID))
}

func DeleteTakeout(userID int64) error {
	if store, ok := Default.(*postgresStore); ok {
		return store.deleteTakeout(userID)
	}
	return Default.Set(takeoutKey+fmt.Sprintf("%d", userID), "")
}

// ConsumeTakeout validates and removes the active session while holding the
// same user-scoped lock used by MutateTakeout. A false result means no active
// session existed; callback errors roll back the delete.
func ConsumeTakeout(userID int64, consume func(string) error) (bool, error) {
	if store, ok := Default.(*postgresStore); ok {
		return store.consumeTakeout(userID, consume)
	}
	takeoutMemoryMu.Lock()
	defer takeoutMemoryMu.Unlock()
	raw, err := LoadTakeout(userID)
	if err != nil {
		return false, err
	}
	if raw == "" {
		return false, nil
	}
	if err := consume(raw); err != nil {
		return false, err
	}
	if err := DeleteTakeout(userID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *postgresStore) loadTakeout(userID int64) (string, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT payload FROM apifull_takeout_session WHERE user_id = $1`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *postgresStore) mutateTakeout(userID int64, mutate func(id int64, current string) (string, error)) (int64, string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-takeout:%d", userID)); err != nil {
		return 0, "", err
	}
	var id int64
	var raw []byte
	err = tx.QueryRow(`SELECT takeout_id, payload FROM apifull_takeout_session WHERE user_id = $1 FOR UPDATE`, userID).Scan(&id, &raw)
	if err == sql.ErrNoRows {
		err = tx.QueryRow(`INSERT INTO apifull_takeout_session(user_id, payload)
			VALUES ($1, '{}'::jsonb) RETURNING takeout_id`, userID).Scan(&id)
		if err != nil {
			return 0, "", err
		}
		raw = nil
	} else if err != nil {
		return 0, "", err
	}
	next, err := mutate(id, string(raw))
	if err != nil {
		return 0, "", err
	}
	if _, err = tx.Exec(`UPDATE apifull_takeout_session SET payload = $1::jsonb, updated_at = CURRENT_TIMESTAMP WHERE user_id = $2`, next, userID); err != nil {
		return 0, "", err
	}
	if err = tx.Commit(); err != nil {
		return 0, "", err
	}
	return id, next, nil
}

func (s *postgresStore) deleteTakeout(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-takeout:%d", userID)); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_takeout_session WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) consumeTakeout(userID int64, consume func(string) error) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("apifull-takeout:%d", userID)); err != nil {
		return false, err
	}
	var raw []byte
	if err = tx.QueryRow(`SELECT payload FROM apifull_takeout_session WHERE user_id = $1 FOR UPDATE`, userID).Scan(&raw); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err = consume(string(raw)); err != nil {
		return false, err
	}
	if _, err = tx.Exec(`DELETE FROM apifull_takeout_session WHERE user_id = $1`, userID); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
