package persist

import "testing"

func TestRewritePostgresPlaceholdersPreservesQuotedSQL(t *testing.T) {
	query := `SELECT ?, 'literal ?', E'escaped \' ?' AS e, "identifier ?"
-- comment ?
/* outer ? /* nested ? */ still comment ? */
WHERE value = ?`
	want := `SELECT $1, 'literal ?', E'escaped \' ?' AS e, "identifier ?"
-- comment ?
/* outer ? /* nested ? */ still comment ? */
WHERE value = $2`
	if got := rewritePostgresPlaceholders(query); got != want {
		t.Fatalf("rewritePostgresPlaceholders() = %q, want %q", got, want)
	}
}

func TestRewritePostgresPlaceholdersPreservesDollarQuotedBodies(t *testing.T) {
	query := `SELECT $$?$$, $tag$body ? $tag$, ?`
	want := `SELECT $$?$$, $tag$body ? $tag$, $1`
	if got := rewritePostgresPlaceholders(query); got != want {
		t.Fatalf("rewritePostgresPlaceholders() = %q, want %q", got, want)
	}
}
