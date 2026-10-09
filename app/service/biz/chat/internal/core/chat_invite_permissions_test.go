package core

import (
	"errors"
	"testing"
)

func TestApifullTableMissing(t *testing.T) {
	if !apifullTableMissing(errors.New(`relation "apifull_channel" does not exist`)) {
		t.Fatal("PostgreSQL undefined-table error was not recognized")
	}
	if apifullTableMissing(errors.New("connection refused")) {
		t.Fatal("unrelated database error was recognized as a missing table")
	}
}
