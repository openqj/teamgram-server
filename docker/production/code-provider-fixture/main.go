// A local verification-code provider for isolated acceptance environments.
//
// This fixture accepts the same JSON Delivery payload used by the configured
// HTTP SMS/email provider and stores the most recent code in memory so an
// acceptance probe can read it back. It is intentionally test-only: the
// production Compose files never start this service and the backend keeps its
// fail-closed provider behavior when no endpoint is configured.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort = "23193"
	bodyLimit   = 1 << 20
)

type delivery struct {
	Channel     string `json:"channel"`
	DeliveryID  string `json:"delivery_id,omitempty"`
	Destination string `json:"destination"`
	Code        string `json:"code"`
	ChallengeID string `json:"challenge_id"`
	Purpose     string `json:"purpose"`
	MNC         string `json:"mnc,omitempty"`
}

type deliveryRecord struct {
	delivery
	ReceivedAt time.Time `json:"received_at"`
}

type idempotencyRecord struct {
	fingerprint string
	record      deliveryRecord
}

type fixture struct {
	key    string
	secret string

	mu          sync.RWMutex
	idempotency map[string]idempotencyRecord
	byChallenge map[string]deliveryRecord
	latest      map[string]deliveryRecord
}

func newFixture(key, secret string) *fixture {
	return &fixture{
		key:         strings.TrimSpace(key),
		secret:      strings.TrimSpace(secret),
		idempotency: make(map[string]idempotencyRecord),
		byChallenge: make(map[string]deliveryRecord),
		latest:      make(map[string]deliveryRecord),
	}
}

func (f *fixture) authorize(r *http.Request) bool {
	if f == nil || f.key == "" || r == nil {
		return false
	}
	if r.Header.Get("Authorization") != "Bearer "+f.key {
		return false
	}
	return f.secret == "" || r.Header.Get("X-Provider-Secret") == f.secret
}

func (f *fixture) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fixture) send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !f.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		http.Error(w, "idempotency key required", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) == 0 || len(body) > bodyLimit {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	var value delivery
	if err = json.Unmarshal(body, &value); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err = validateDelivery(value, idempotencyKey); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	digest := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(digest[:])
	record := deliveryRecord{delivery: value, ReceivedAt: time.Now().UTC()}

	f.mu.Lock()
	if previous, ok := f.idempotency[idempotencyKey]; ok {
		f.mu.Unlock()
		if previous.fingerprint != fingerprint {
			http.Error(w, "idempotency key reused with a different delivery", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	f.idempotency[idempotencyKey] = idempotencyRecord{fingerprint: fingerprint, record: record}
	f.byChallenge[value.ChallengeID] = record
	f.latest[deliveryKey(value)] = record
	f.mu.Unlock()

	w.WriteHeader(http.StatusAccepted)
}

func validateDelivery(value delivery, idempotencyKey string) error {
	channel := strings.ToLower(strings.TrimSpace(value.Channel))
	if channel != "sms" && channel != "email" {
		return errors.New("channel must be sms or email")
	}
	if value.DeliveryID == "" || value.DeliveryID != idempotencyKey {
		return errors.New("delivery_id must match Idempotency-Key")
	}
	if strings.TrimSpace(value.Destination) == "" {
		return errors.New("destination is required")
	}
	if strings.TrimSpace(value.Code) == "" {
		return errors.New("code is required")
	}
	if strings.TrimSpace(value.ChallengeID) == "" {
		return errors.New("challenge_id is required")
	}
	if strings.TrimSpace(value.Purpose) == "" {
		return errors.New("purpose is required")
	}
	return nil
}

func deliveryKey(value delivery) string {
	return strings.Join([]string{value.Channel, value.Purpose, value.Destination}, "\x00")
}

func (f *fixture) latestDelivery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !f.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	f.mu.RLock()
	var record deliveryRecord
	var ok bool
	if challengeID := strings.TrimSpace(r.URL.Query().Get("challenge_id")); challengeID != "" {
		record, ok = f.byChallenge[challengeID]
	} else {
		record, ok = f.latest[deliveryKey(delivery{
			Channel: r.URL.Query().Get("channel"), Purpose: r.URL.Query().Get("purpose"), Destination: r.URL.Query().Get("destination"),
		})]
	}
	f.mu.RUnlock()
	if !ok {
		http.Error(w, "delivery not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func loadFixture() (*fixture, int, error) {
	key := strings.TrimSpace(os.Getenv("CODE_PROVIDER_KEY"))
	secret := strings.TrimSpace(os.Getenv("CODE_PROVIDER_SECRET"))
	if key == "" || secret == "" {
		return nil, 0, errors.New("CODE_PROVIDER_KEY and CODE_PROVIDER_SECRET are required")
	}
	port := strings.TrimSpace(os.Getenv("CODE_PROVIDER_PORT"))
	if port == "" {
		port = defaultPort
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, 0, fmt.Errorf("CODE_PROVIDER_PORT must be from 1 to 65535")
	}
	return newFixture(key, secret), portNumber, nil
}

func main() {
	f, port, err := loadFixture()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		return
	}
	if len(os.Args) > 1 {
		log.Fatal("unsupported argument")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", f.healthz)
	mux.HandleFunc("/send", f.send)
	mux.HandleFunc("/latest", f.latestDelivery)
	server := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("code provider fixture listening on :%d", port)
	log.Fatal(server.ListenAndServe())
}
