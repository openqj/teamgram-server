package persist

import "testing"

func TestOpenPostgresRequiredRejectsEmptyDSN(t *testing.T) {
	if err := OpenPostgresRequired("drafts", "  "); err == nil {
		t.Fatal("OpenPostgresRequired accepted an empty DSN")
	}
}
