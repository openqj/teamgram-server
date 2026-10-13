package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCanonicalJSONSortsObjectsWithoutChangingNumbers(t *testing.T) {
	canonical, err := canonicalJSON([]byte(`{"z":{"b":2,"a":1},"amount":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"amount":9007199254740993,"z":{"a":1,"b":2}}`)
	if !bytes.Equal(canonical, want) {
		t.Fatalf("canonical JSON = %s, want %s", canonical, want)
	}
	var decoded map[string]any
	if err = json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, err = canonicalJSON([]byte(`{"valid":true}{"trailing":true}`)); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}

func TestControlPlaneRejectsProviderTransactionReuse(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_DSN must point to the provider PostgreSQL 18 control plane")
	}
	store, err := openControlPlane(dsn)
	if err != nil {
		t.Skipf("provider PostgreSQL control plane unavailable: %v", err)
	}
	t.Cleanup(func() { _ = store.close() })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	operation := "fixture-transaction-reuse-" + suffix
	userID := time.Now().UnixNano()
	transactionID := "fixture-transaction-" + suffix
	t.Cleanup(func() {
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_receipt WHERE operation=$1`, operation)
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_entitlement WHERE transaction_id=$1`, transactionID)
		_, _ = store.db.Exec(`DELETE FROM apifull_payment_provider_operation WHERE operation=$1`, operation)
	})

	response := []byte(`{"transaction_id":"fixture-transaction"}`)
	if _, err = store.saveOperation(operation, "request-1", userID, []byte(`{"amount":1}`), response,
		"X-Teamgram-Payment-Signature", transactionID, "premium_subscription", 1, 0); err != nil {
		t.Fatalf("first provider operation: %v", err)
	}
	if _, err = store.saveOperation(operation, "request-2", userID, []byte(`{"amount":2}`), []byte(`{"transaction_id":"other"}`),
		"X-Teamgram-Payment-Signature", transactionID, "premium_subscription", 1, 0); !errors.Is(err, errProviderTransactionConflict) {
		t.Fatalf("transaction reuse error = %v, want %v", err, errProviderTransactionConflict)
	}

	var storedUserID int64
	var storedRequestKey string
	var storedResponse []byte
	if err = store.db.QueryRow(`SELECT user_id, request_key, response
		FROM apifull_payment_provider_receipt WHERE operation=$1 AND transaction_id=$2`, operation, transactionID).
		Scan(&storedUserID, &storedRequestKey, &storedResponse); err != nil {
		t.Fatalf("load original receipt: %v", err)
	}
	canonicalResponse, canonicalErr := canonicalJSON(storedResponse)
	if canonicalErr != nil {
		t.Fatalf("canonicalize original receipt: %v", canonicalErr)
	}
	if storedUserID != userID || storedRequestKey != "request-1" || string(canonicalResponse) != string(response) {
		t.Fatalf("original receipt was changed: user=%d request=%q response=%s", storedUserID, storedRequestKey, storedResponse)
	}

	var operationCount int
	if err = store.db.QueryRow(`SELECT count(*) FROM apifull_payment_provider_operation WHERE operation=$1`, operation).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 {
		t.Fatalf("conflicting operation persisted: count=%d", operationCount)
	}
	var receiptErr error
	var ignored int
	receiptErr = store.db.QueryRow(`SELECT 1 FROM apifull_payment_provider_receipt WHERE operation=$1 AND transaction_id=$2`, operation, transactionID).Scan(&ignored)
	if errors.Is(receiptErr, sql.ErrNoRows) {
		t.Fatal("original receipt disappeared after conflict")
	}
	if receiptErr != nil {
		t.Fatal(receiptErr)
	}
}
