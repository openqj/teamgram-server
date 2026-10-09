package persist

import "testing"

func TestCompareAndSwapMemoryStore(t *testing.T) {
	previous := Default
	Default = &mem{}
	t.Cleanup(func() { Default = previous })

	if ok, err := CompareAndSwap("reset", "", "first"); err != nil || !ok {
		t.Fatalf("initialize: ok=%v err=%v", ok, err)
	}
	if ok, err := CompareAndSwap("reset", "", "lost"); err != nil || ok {
		t.Fatalf("stale update: ok=%v err=%v", ok, err)
	}
	if got, err := Default.Get("reset"); err != nil || got != "first" {
		t.Fatalf("value after stale update: %q err=%v", got, err)
	}
	if ok, err := CompareAndSwap("reset", "first", "second"); err != nil || !ok {
		t.Fatalf("replace: ok=%v err=%v", ok, err)
	}
}

func TestPostgresRequiresExplicitDSN(t *testing.T) {
	for _, dsn := range []string{"", " \t\n"} {
		if db, err := OpenPostgresDB(dsn); err == nil || db != nil {
			t.Fatalf("OpenPostgresDB(%q) = (%v, %v), want missing DSN error", dsn, db, err)
		}
	}
}
