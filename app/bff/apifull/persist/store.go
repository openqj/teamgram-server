package persist

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/teamgram/proto/mtproto"
	internal "github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// Store is the process-wide blob store shared with apifull.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Default follows the apifull store, including a later Use or database open.
var Default Store = live{}

// OpenPostgres connects the shared process store to the deployment-owned
// PostgreSQL 18 APIFull schema. BFFs that keep authentication state in this
// store call it during startup so password and recovery state survives
// process restarts.
func OpenPostgres(dsn string) error {
	return internal.OpenPostgres(dsn)
}

// OpenPostgresReadOnly connects to an already-migrated APIFull schema without
// issuing DDL. Production services should use this startup gate.
func OpenPostgresReadOnly(dsn string) error {
	return internal.OpenPostgresReadOnly(dsn)
}

// OpenPostgresRequired validates and opens the shared PostgreSQL 18 store for
// a BFF service that owns handlers backed by apifull persistence.
func OpenPostgresRequired(service, dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("%s: PostgresDSN is required", service)
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		return fmt.Errorf("%s: open PostgreSQL store: %w", service, err)
	}
	return nil
}

// PostgresEnabled reports whether the shared process store is backed by the
// deployment-owned PostgreSQL schema. Standalone BFF services use this as a
// startup invariant so they cannot silently fall back to memory.
func PostgresEnabled() bool {
	return internal.PostgresEnabled()
}

// ClosePostgres releases the shared process store during standalone service
// shutdown. The monolithic BFF uses the same hook from its top-level server.
func ClosePostgres() error {
	return internal.ClosePostgres()
}

// SaveAppLogEvents appends help.saveAppLog events to the shared PostgreSQL
// store. The batch is committed atomically by the internal persistence layer.
func SaveAppLogEvents(ctx context.Context, userID int64, events []*mtproto.InputAppEvent) error {
	return internal.SaveAppLogEvents(ctx, userID, events)
}

type live struct{}

func (live) Get(key string) (string, error) { return internal.Default.Get(key) }
func (live) Set(key, value string) error    { return internal.Default.Set(key, value) }
func (live) CompareAndSwap(key, expected, replacement string) (bool, error) {
	return internal.CompareAndSwap(key, expected, replacement)
}
func (live) Update(key string, fn func(string) (string, error)) error {
	return internal.Update(key, fn)
}

// CompareAndSwap delegates to the shared APIFull store and is used for
// state transitions that must not lose a concurrent update.
func CompareAndSwap(key, expected, replacement string) (bool, error) {
	if s, ok := Default.(interface {
		CompareAndSwap(string, string, string) (bool, error)
	}); ok {
		return s.CompareAndSwap(key, expected, replacement)
	}
	current, err := Default.Get(key)
	if err != nil {
		return false, err
	}
	if current != expected {
		return false, nil
	}
	return true, Default.Set(key, replacement)
}

// Update executes a read/modify/write operation under the store's transaction
// boundary. PostgreSQL uses its row lock and transaction; test stores fall
// back to the compare-and-swap implementation.
func Update(key string, fn func(string) (string, error)) error {
	if s, ok := Default.(interface {
		Update(string, func(string) (string, error)) error
	}); ok {
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
