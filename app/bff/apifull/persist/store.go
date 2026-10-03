package persist

import internal "github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"

// Store is the process-wide blob store shared with apifull.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Default follows the apifull store, including a later Use or OpenMySQL.
var Default Store = live{}

type live struct{}

func (live) Get(key string) (string, error) { return internal.Default.Get(key) }
func (live) Set(key, value string) error    { return internal.Default.Set(key, value) }
