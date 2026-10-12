package persist

import (
	"strings"
	"testing"
)

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

func TestReadOnlyPostgresGateRequiresProviderControlPlane(t *testing.T) {
	if len(requiredReadOnlyPostgresMigrations) != 16 {
		t.Fatalf("required migration count = %d, want 16", len(requiredReadOnlyPostgresMigrations))
	}
	for _, required := range []string{
		"078_apifull_payment_provider_control_plane",
		"089_apifull_forum_topic_icon_color",
		"apifull_payment_provider_operation",
		"apifull_payment_provider_receipt",
		"apifull_payment_provider_entitlement",
	} {
		found := false
		for _, migration := range requiredReadOnlyPostgresMigrations {
			if migration == required {
				found = true
				break
			}
		}
		if !found {
			for _, query := range requiredReadOnlyPostgresSchemaQueries {
				if strings.Contains(query, required) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("read-only PostgreSQL gate does not require %q", required)
		}
	}
}
