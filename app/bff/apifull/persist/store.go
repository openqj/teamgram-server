package persist

import internal "github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"

// Store is the process-wide blob store shared with apifull.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Default follows the apifull store, including a later Use or database open.
var Default Store = live{}

type live struct{}

func (live) Get(key string) (string, error) { return internal.Default.Get(key) }
func (live) Set(key, value string) error    { return internal.Default.Set(key, value) }
func (live) CompareAndSwap(key, expected, replacement string) (bool, error) {
	return internal.CompareAndSwap(key, expected, replacement)
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
