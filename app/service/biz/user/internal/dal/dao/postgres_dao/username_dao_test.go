package postgres_dao

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

type usernameRowError struct{ err error }

func (r usernameRowError) Scan(...any) error { return r.err }

func TestScanUsernameNoRows(t *testing.T) {
	got, err := scanUsername(usernameRowError{err: pgx.ErrNoRows})
	if err != nil {
		t.Fatalf("scanUsername returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("scanUsername returned %#v, want nil", got)
	}
}
