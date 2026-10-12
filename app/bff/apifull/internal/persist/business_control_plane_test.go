package persist

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestBusinessControlPlanePostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL control-plane schema unavailable: %v", err)
	}
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		t.Fatal("postgres store was not installed")
	}
	operation := "test.businessControl"
	scope := "round-trip"
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_business_control_plane WHERE operation = $1 AND scope_key IN ($2, 'expired', 'revoked')`, operation, scope)
		_ = ClosePostgres()
		Default = previous
	})
	if _, err := store.db.Exec(`DELETE FROM apifull_business_control_plane WHERE operation = $1 AND scope_key IN ($2, 'expired', 'revoked')`, operation, scope); err != nil {
		t.Fatal(err)
	}

	global := json.RawMessage(`{"predicate_name":"updates","pts":10,"pts_count":1,"date":1}`)
	if err := PutBusinessControlRecord(BusinessControlRecord{Operation: operation, ScopeKey: scope, Result: global}); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadBusinessControlResult(operation, scope, 1001)
	if err != nil || !found || !businessJSONEqual(got, global) {
		t.Fatalf("global result = (%s, %v, %v)", got, found, err)
	}
	if _, found, err = LoadBusinessControlResultForCaller(operation, scope, 1001); err != nil || found {
		t.Fatalf("caller-scoped lookup fell back to global result (found=%v, err=%v)", found, err)
	}

	callerResult := json.RawMessage(`{"predicate_name":"updates","pts":11,"pts_count":1,"date":2}`)
	if err := PutBusinessControlRecord(BusinessControlRecord{Operation: operation, ScopeKey: scope, CallerUserID: 1001, Result: callerResult}); err != nil {
		t.Fatal(err)
	}
	got, found, err = LoadBusinessControlResult(operation, scope, 1001)
	if err != nil || !found || !businessJSONEqual(got, callerResult) {
		t.Fatalf("caller result = (%s, %v, %v)", got, found, err)
	}
	got, found, err = LoadBusinessControlResultForCaller(operation, scope, 1001)
	if err != nil || !found || !businessJSONEqual(got, callerResult) {
		t.Fatalf("caller-scoped result = (%s, %v, %v)", got, found, err)
	}
	if _, found, err = LoadBusinessControlResultForCaller(operation, scope, 1002); err != nil || found {
		t.Fatalf("other caller saw caller-scoped result (found=%v, err=%v)", found, err)
	}

	expired := time.Now().UTC().Add(-time.Minute)
	if err := PutBusinessControlRecord(BusinessControlRecord{Operation: operation, ScopeKey: "expired", Result: global, ExpiresAt: &expired}); err != nil {
		t.Fatal(err)
	}
	if _, found, err = LoadBusinessControlResult(operation, "expired", 1001); err != nil || found {
		t.Fatalf("expired result = (found=%v, err=%v)", found, err)
	}
	if err := PutBusinessControlRecord(BusinessControlRecord{Operation: operation, ScopeKey: "revoked", State: "revoked", Result: global}); err != nil {
		t.Fatal(err)
	}
	if _, found, err = LoadBusinessControlResult(operation, "revoked", 1001); err != nil || found {
		t.Fatalf("revoked result = (found=%v, err=%v)", found, err)
	}
}

func businessJSONEqual(left, right []byte) bool {
	var l, r any
	if json.Unmarshal(left, &l) != nil || json.Unmarshal(right, &r) != nil {
		return false
	}
	return string(mustBusinessJSON(l)) == string(mustBusinessJSON(r))
}

func mustBusinessJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}
