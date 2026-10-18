// Signed PostgreSQL-backed business provider for isolated Layer 229 probes.
//
// This service deliberately answers only operations backed by rows in the
// provider control-plane tables.  It is useful for a repeatable deployment
// probe, but it is not an application-side success fallback: missing rows and
// malformed requests are returned as provider errors.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/pkg/providercontract"
)

const (
	defaultPort = "23190"
	bodyLimit   = 1 << 20
)

type requestEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	UserID    int64           `json:"user_id"`
	Payload   json.RawMessage `json:"payload"`
}

type responseEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

type businessStore interface {
	StartLive(context.Context, int64, int64, []byte) ([]byte, error)
	Collectible(context.Context, string) ([]byte, error)
	Callback(context.Context, int64, string, int64, []byte) ([]byte, error)
	GiftChatThemes(context.Context, int64, []byte) ([]byte, error)
	BotOperation(context.Context, int64, string, []byte) ([]byte, error)
	PaymentOperation(context.Context, int64, string, []byte) ([]byte, error)
}

type providerHealthChecker interface {
	Ping(context.Context) error
}

func providerReadiness(ctx context.Context, store businessStore) error {
	if store == nil {
		return errors.New("provider store is unavailable")
	}
	if checker, ok := store.(providerHealthChecker); ok {
		return checker.Ping(ctx)
	}
	return nil
}

// postgresStore keeps all mutable provider state in PostgreSQL.  The fixture
// never creates missing schema at runtime; deployment migrations own it.
type postgresStore struct{ db *pgxpool.Pool }

func (s *postgresStore) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("provider database is unavailable")
	}
	if err := s.db.Ping(ctx); err != nil {
		return err
	}
	return validateProviderSchema(ctx, s.db)
}

func (s *postgresStore) StartLive(ctx context.Context, uid, randomID int64, payload []byte) ([]byte, error) {
	fingerprint := sha256.Sum256(payload)
	var storedFingerprint []byte
	var stored json.RawMessage
	err := s.db.QueryRow(ctx, `SELECT request_fingerprint, updates FROM teamgram_business_live_session WHERE user_id=$1 AND random_id=$2`, uid, randomID).Scan(&storedFingerprint, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("stories.startLive session not provisioned")
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(storedFingerprint, fingerprint[:]) {
		return nil, errors.New("stories.startLive random_id reused with different request")
	}
	return append([]byte(nil), stored...), nil
}

func (s *postgresStore) Collectible(ctx context.Context, key string) ([]byte, error) {
	var purchaseDate int32
	var currency, cryptoCurrency, url string
	var amount, cryptoAmount int64
	var active bool
	var expires *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT purchase_date, currency, amount, crypto_currency, crypto_amount, url, active, expires_at
		FROM teamgram_business_collectible
		WHERE lookup_key=$1`, key).Scan(&purchaseDate, &currency, &amount, &cryptoCurrency, &cryptoAmount, &url, &active, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("collectible not provisioned")
	}
	if err != nil {
		return nil, err
	}
	if !active || (expires != nil && !expires.After(time.Now())) {
		return nil, errors.New("collectible is expired or inactive")
	}
	return json.Marshal(map[string]any{
		"predicate_name":  "fragment_collectibleInfo",
		"purchase_date":   purchaseDate,
		"currency":        currency,
		"amount":          amount,
		"crypto_currency": cryptoCurrency,
		"crypto_amount":   cryptoAmount,
		"url":             url,
	})
}

func (s *postgresStore) Callback(ctx context.Context, uid int64, peerKey string, messageID int64, data []byte) ([]byte, error) {
	digest := sha256.Sum256(data)
	var answer json.RawMessage
	err := s.db.QueryRow(ctx, `
		SELECT answer FROM teamgram_business_callback_answer
		WHERE user_id=$1 AND peer_key=$2 AND message_id=$3 AND data_digest=$4 AND expires_at > CURRENT_TIMESTAMP`, uid, peerKey, messageID, digest[:]).Scan(&answer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New("callback answer not provisioned")
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), answer...), nil
}

func (s *postgresStore) GiftChatThemes(ctx context.Context, _ int64, payload []byte) ([]byte, error) {
	var request struct {
		Offset    string `json:"offset"`
		OffsetInt int    `json:"offset_int"`
		Limit     int    `json:"limit"`
		Hash      int64  `json:"hash"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, err
	}
	if request.Limit < 0 || request.Limit > 100 {
		return nil, errors.New("gift chat theme limit is invalid")
	}
	offset := request.OffsetInt
	if request.Offset != "" {
		if _, err := fmt.Sscanf(request.Offset, "%d", &offset); err != nil || offset < 0 {
			return nil, errors.New("gift chat theme offset is invalid")
		}
	}
	rows, err := s.db.Query(ctx, `
		SELECT g.id, g.slug, g.stars
		  FROM apifull_gift_chat_theme t
		  JOIN apifull_gift g ON g.id=t.gift_id
		 ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type giftRow struct {
		ID    int64
		Slug  string
		Stars int64
	}
	gifts := make([]giftRow, 0)
	for rows.Next() {
		var gift giftRow
		if err := rows.Scan(&gift.ID, &gift.Slug, &gift.Stars); err != nil {
			return nil, err
		}
		gifts = append(gifts, gift)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	hash := fnv.New64a()
	for _, gift := range gifts {
		_, _ = hash.Write([]byte(fmt.Sprintf("%d", gift.ID)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(gift.Slug))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(fmt.Sprintf("%d", gift.Stars)))
	}
	catalogHash := int64(hash.Sum64() & 0x7fffffffffffffff)
	if request.Hash != 0 && request.Hash == catalogHash {
		return json.Marshal(map[string]any{
			"predicate_name": "account_chatThemesNotModified",
			"hash":           catalogHash,
			"themes":         []any{},
			"chats":          []any{},
			"users":          []any{},
		})
	}
	if offset < 0 || offset > len(gifts) {
		return nil, errors.New("gift chat theme offset is invalid")
	}
	limit := request.Limit
	if limit == 0 {
		limit = 100
	}
	end := offset + limit
	if end > len(gifts) {
		end = len(gifts)
	}
	themes := make([]any, 0, end-offset)
	for _, gift := range gifts[offset:end] {
		themes = append(themes, map[string]any{
			"predicate_name": "chatThemeUniqueGift",
			"gift": map[string]any{
				"predicate_name":  "starGift",
				"id":              gift.ID,
				"sticker":         map[string]any{"predicate_name": "documentEmpty", "id": gift.ID},
				"stars":           gift.Stars,
				"theme_available": true,
				"gift_id":         gift.ID,
				"slug":            gift.Slug,
				"title_STRING":    gift.Slug,
			},
			"theme_settings": []any{},
		})
	}
	result := map[string]any{
		"predicate_name": "account_chatThemes",
		"hash":           catalogHash,
		"themes":         themes,
		"chats":          []any{},
		"users":          []any{},
	}
	if end < len(gifts) {
		result["next_offset_FLAGSTRING"] = fmt.Sprintf("%d", end)
	}
	return json.Marshal(result)
}

func (s *postgresStore) BotOperation(ctx context.Context, uid int64, operation string, payload []byte) ([]byte, error) {
	fingerprint := sha256.Sum256(payload)
	var result json.RawMessage
	err := s.db.QueryRow(ctx, `
		SELECT result FROM teamgram_business_bot_operation
		WHERE operation=$1 AND user_id=$2 AND request_fingerprint=$3
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`,
		operation, uid, fingerprint[:]).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("bot operation %s is not provisioned", operation)
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), result...), nil
}

func (s *postgresStore) PaymentOperation(ctx context.Context, uid int64, operation string, payload []byte) ([]byte, error) {
	fingerprint := sha256.Sum256(payload)
	var result json.RawMessage
	err := s.db.QueryRow(ctx, `
		SELECT result FROM teamgram_business_payment_operation
		WHERE operation=$1 AND user_id=$2 AND request_fingerprint=$3
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)`,
		operation, uid, fingerprint[:]).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("payment operation %s is not provisioned", operation)
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), result...), nil
}

// memoryStore is used only by unit tests.  Production startup requires a
// PostgreSQL DSN and uses postgresStore.
type memoryStore struct {
	mu          sync.Mutex
	live        map[string]memoryLive
	collectible map[string][]byte
	callback    map[string][]byte
	botOps      map[string][]byte
	paymentOps  map[string][]byte
}

type memoryLive struct{ fingerprint, updates []byte }

func newMemoryStore() *memoryStore {
	return &memoryStore{live: map[string]memoryLive{}, collectible: map[string][]byte{}, callback: map[string][]byte{}, botOps: map[string][]byte{}, paymentOps: map[string][]byte{}}
}

func (s *memoryStore) StartLive(_ context.Context, uid, randomID int64, payload []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fp := sha256.Sum256(payload)
	key := fmt.Sprintf("%d:%d", uid, randomID)
	if current, ok := s.live[key]; ok {
		if !bytes.Equal(current.fingerprint, fp[:]) {
			return nil, errors.New("stories.startLive random_id reused with different request")
		}
		return append([]byte(nil), current.updates...), nil
	}
	updates := canonicalUpdates(time.Now().Unix())
	s.live[key] = memoryLive{fingerprint: append([]byte(nil), fp[:]...), updates: updates}
	return append([]byte(nil), updates...), nil
}

func (s *memoryStore) Collectible(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.collectible[key]
	if !ok {
		return nil, errors.New("collectible not provisioned")
	}
	return append([]byte(nil), value...), nil
}

func (s *memoryStore) Callback(_ context.Context, uid int64, peerKey string, messageID int64, data []byte) ([]byte, error) {
	digest := sha256.Sum256(data)
	key := fmt.Sprintf("%d:%s:%d:%x", uid, peerKey, messageID, digest[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.callback[key]
	if !ok {
		return nil, errors.New("callback answer not provisioned")
	}
	return append([]byte(nil), value...), nil
}

func (s *memoryStore) GiftChatThemes(context.Context, int64, []byte) ([]byte, error) {
	return nil, errors.New("gift chat themes not provisioned")
}

func (s *memoryStore) BotOperation(_ context.Context, uid int64, operation string, payload []byte) ([]byte, error) {
	fingerprint := sha256.Sum256(payload)
	key := fmt.Sprintf("%d:%s:%x", uid, operation, fingerprint[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.botOps[key]
	if !ok {
		return nil, fmt.Errorf("bot operation %s is not provisioned", operation)
	}
	return append([]byte(nil), value...), nil
}

func (s *memoryStore) PaymentOperation(_ context.Context, uid int64, operation string, payload []byte) ([]byte, error) {
	fingerprint := sha256.Sum256(payload)
	key := fmt.Sprintf("%d:%s:%x", uid, operation, fingerprint[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.paymentOps[key]
	if !ok {
		return nil, fmt.Errorf("payment operation %s is not provisioned", operation)
	}
	return append([]byte(nil), value...), nil
}

type server struct {
	key             []byte
	authKey         string
	store           businessStore
	maxAge          time.Duration
	clock           func() time.Time
	webViewDataMode string
}

func (s *server) sign(body []byte) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifySignature(key []byte, value string, body []byte) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "sha256=")
	provided, err := hex.DecodeString(value)
	if err != nil || len(provided) != sha256.Size || len(key) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func validBearerAuthorization(value, expected string) bool {
	if strings.TrimSpace(expected) == "" || !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(value, "Bearer ")
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authKey != "" && !validBearerAuthorization(r.Header.Get("Authorization"), s.authKey) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) == 0 || len(body) > bodyLimit || !verifySignature(s.key, r.Header.Get("X-Teamgram-Business-Provider-Request-Signature"), body) {
		http.Error(w, "invalid signed request", http.StatusUnauthorized)
		return
	}
	var request requestEnvelope
	if err = json.Unmarshal(body, &request); err != nil || request.RequestID == "" || request.UserID <= 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	expectedIdempotencyKey := providercontract.IdempotencyKey(request.UserID, request.Operation, request.Payload)
	providedIdempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(providedIdempotencyKey) != len(expectedIdempotencyKey) ||
		subtle.ConstantTimeCompare([]byte(providedIdempotencyKey), []byte(expectedIdempotencyKey)) != 1 {
		http.Error(w, "invalid idempotency key", http.StatusBadRequest)
		return
	}
	payloadFingerprint := sha256.Sum256(request.Payload)
	log.Printf("business provider operation=%s user_id=%d request_id=%s payload_sha256=%s", request.Operation, request.UserID, request.RequestID, hex.EncodeToString(payloadFingerprint[:]))
	ctx, cancel := context.WithTimeout(r.Context(), s.maxAge)
	defer cancel()
	result, providerErr := s.dispatch(ctx, request)
	response := responseEnvelope{RequestID: request.RequestID, Operation: request.Operation, Verified: providerErr == nil, UserID: request.UserID}
	if providerErr != nil {
		response.Error = providerErr.Error()
	} else {
		response.Result = result
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Teamgram-Business-Provider-Signature", s.sign(encoded))
	_, _ = w.Write(encoded)
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var store businessStore
	if s != nil {
		store = s.store
	}
	if err := providerReadiness(ctx, store); err != nil {
		http.Error(w, "provider database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) dispatch(ctx context.Context, request requestEnvelope) (json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return nil, errors.New("payload must be an object")
	}
	switch request.Operation {
	case "stories.startLive":
		randomID := rawInt64(payload, "random_id")
		if randomID <= 0 {
			return nil, errors.New("random_id must be positive")
		}
		updates, err := s.store.StartLive(ctx, request.UserID, randomID, request.Payload)
		return updates, err
	case "fragment.getCollectibleInfo":
		key, err := collectibleKey(payload["collectible"])
		if err != nil {
			return nil, err
		}
		result, err := s.store.Collectible(ctx, key)
		return result, err
	case "ephemeral.getCallbackAnswer":
		peerKey, err := peerKey(payload["peer"])
		if err != nil {
			return nil, err
		}
		messageID := rawInt64(payload, "id")
		if messageID <= 0 {
			return nil, errors.New("message id must be positive")
		}
		var data []byte
		if value := payload["data"]; len(value) > 0 && string(value) != "null" {
			if err = json.Unmarshal(value, &data); err != nil {
				return nil, err
			}
		}
		return s.store.Callback(ctx, request.UserID, peerKey, messageID, data)
	case "account.getUniqueGiftChatThemes":
		return s.store.GiftChatThemes(ctx, request.UserID, request.Payload)
	case "bots.answerWebhookJSONQuery", "bots.checkDownloadFileParams",
		"bots.getBotRecommendations", "bots.getPopularAppBots",
		"bots.invokeWebViewCustomMethod", "bots.sendCustomRequest",
		"bots.setCustomVerification", "bots.setJoinChatResults",
		"bots.updateStarRefProgram", "messages.editInlineBotMessage",
		"messages.getBotApp", "messages.requestAppWebView",
		"messages.getPaidReactionPrivacy",
		"messages.sendWebViewResultMessage", "messages.setBotGuestChatResult",
		"messages.requestChatJoinWebView", "messages.togglePaidReactionPrivacy",
		"messages.setBotShippingResults", "messages.setBotPrecheckoutResults":
		return s.store.BotOperation(ctx, request.UserID, request.Operation, request.Payload)
	case "messages.sendWebViewData":
		if strings.EqualFold(strings.TrimSpace(s.webViewDataMode), "echo") {
			return json.RawMessage(canonicalUpdates(time.Now().Unix())), nil
		}
		return s.store.BotOperation(ctx, request.UserID, request.Operation, request.Payload)
	case "payments.launchPrepaidGiveaway", "payments.getStarGiftUpgradePreview",
		"payments.upgradeStarGift", "payments.transferStarGift", "payments.getStarGiftWithdrawalUrl",
		"payments.getResaleStarGifts", "payments.updateStarGiftPrice",
		"payments.checkCanSendGift", "payments.getStarGiftAuctionState",
		"payments.getStarGiftAuctionAcquiredGifts", "payments.getStarGiftActiveAuctions",
		"payments.resolveStarGiftOffer", "payments.sendStarGiftOffer",
		"payments.getStarGiftUpgradeAttributes", "payments.getCraftStarGifts",
		"payments.craftStarGift":
		return s.store.PaymentOperation(ctx, request.UserID, request.Operation, request.Payload)
	default:
		return nil, fmt.Errorf("operation %q is not provisioned", request.Operation)
	}
}

func rawInt64(fields map[string]json.RawMessage, name string) int64 {
	var value int64
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func collectibleKey(raw json.RawMessage) (string, error) {
	var value struct {
		Username string `json:"username"`
		Phone    string `json:"phone"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	username := strings.ToLower(strings.TrimSpace(value.Username))
	phone := strings.TrimSpace(value.Phone)
	if (username == "") == (phone == "") {
		return "", errors.New("collectible requires exactly one username or phone")
	}
	if username != "" {
		return "username:" + username, nil
	}
	return "phone:" + phone, nil
}

// peerKey is intentionally a stable, opaque lookup key.  The BFF validates
// ownership and access hashes before reaching this service; the provider only
// needs a canonical key to avoid cross-peer callback leakage.
func peerKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "self", nil
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	if predicate := rawString(value, "predicate_name"); predicate != "" {
		return predicate + ":" + string(raw), nil
	}
	return string(raw), nil
}

func rawString(fields map[string]json.RawMessage, name string) string {
	var value string
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func canonicalUpdates(date int64) []byte {
	result, _ := json.Marshal(map[string]any{
		"predicate_name": "updates",
		"updates":        []any{},
		"users":          []any{},
		"chats":          []any{},
		"date":           date,
		"seq":            0,
	})
	return result
}

func openPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("TEAMGRAM_POSTGRES_DSN is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&version); err != nil || version/10000 != 18 {
		pool.Close()
		if err == nil {
			err = fmt.Errorf("PostgreSQL 18 is required (server_version_num=%d)", version)
		}
		return nil, err
	}
	if err = validateProviderSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func validateProviderSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if err := requireMigrationCatalog(ctx, pool, "070_apifull_gift_chat_themes", "078_apifull_payment_provider_control_plane", "079_provider_control_plane", "082_business_bot_provider_operations", "086_business_message_provider_operations", "088_business_payment_provider_operations", "110_business_mini_app_provider_operations"); err != nil {
		return fmt.Errorf("provider migration catalog is unavailable: %w", err)
	}
	for _, query := range []string{
		`SELECT user_id, random_id, request_fingerprint, updates FROM teamgram_business_live_session LIMIT 0`,
		`SELECT lookup_key, purchase_date, url FROM teamgram_business_collectible LIMIT 0`,
		`SELECT user_id, peer_key, message_id, data_digest, answer, expires_at FROM teamgram_business_callback_answer LIMIT 0`,
		`SELECT operation, user_id, request_fingerprint, result, expires_at FROM teamgram_business_bot_operation LIMIT 0`,
		`SELECT operation, user_id, request_fingerprint, result, expires_at FROM teamgram_business_payment_operation LIMIT 0`,
		`SELECT gift_id FROM apifull_gift_chat_theme LIMIT 0`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("provider schema is unavailable: %w", err)
		}
	}
	return nil
}

func requireMigrationCatalog(ctx context.Context, pool *pgxpool.Pool, versions ...string) error {
	if pool == nil || len(versions) == 0 {
		return errors.New("provider migration catalog is unavailable")
	}
	for _, version := range versions {
		var checksum string
		var appliedAt time.Time
		err := pool.QueryRow(ctx, `
			SELECT checksum, applied_at
			FROM public.schema_migrations
			WHERE version = $1`, version).Scan(&checksum, &appliedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("migration %q is not applied", version)
		}
		if err != nil {
			return err
		}
		if !validMigrationChecksum(checksum) || appliedAt.IsZero() {
			return fmt.Errorf("migration %q has an invalid catalog record", version)
		}
	}
	return nil
}

func validMigrationChecksum(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func main() {
	key := strings.TrimSpace(os.Getenv("TEAMGRAM_BUSINESS_PROVIDER_SIGNING_KEY"))
	if len(key) < 32 {
		log.Fatal("TEAMGRAM_BUSINESS_PROVIDER_SIGNING_KEY must contain at least 32 bytes")
	}
	authKey := strings.TrimSpace(os.Getenv("TEAMGRAM_BUSINESS_PROVIDER_KEY"))
	if authKey == "" {
		log.Fatal("TEAMGRAM_BUSINESS_PROVIDER_KEY must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := openPostgres(ctx, os.Getenv("TEAMGRAM_POSTGRES_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	timeout := 5 * time.Second
	if value := strings.TrimSpace(os.Getenv("TEAMGRAM_BUSINESS_PROVIDER_TIMEOUT_SECONDS")); value != "" {
		seconds, parseErr := strconv.Atoi(value)
		if parseErr != nil || seconds <= 0 || seconds > 300 {
			log.Fatal("TEAMGRAM_BUSINESS_PROVIDER_TIMEOUT_SECONDS must be from 1 to 300")
		}
		timeout = time.Duration(seconds) * time.Second
	}
	s := &server{
		key:             []byte(key),
		authKey:         authKey,
		store:           &postgresStore{db: pool},
		maxAge:          timeout,
		clock:           time.Now,
		webViewDataMode: os.Getenv("WEBVIEW_DATA_MODE"),
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	portNumber, parseErr := strconv.Atoi(port)
	if parseErr != nil || portNumber < 1 || portNumber > 65535 {
		log.Fatal("PORT must be an integer from 1 to 65535")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/", s.handle)
	server := &http.Server{Addr: ":" + port, Handler: mux}
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer healthCancel()
		if err := providerReadiness(healthCtx, s.store); err != nil {
			log.Fatalf("provider readiness check failed: %v", err)
		}
		return
	}
	if len(os.Args) > 1 {
		log.Fatal("unsupported argument")
	}
	log.Printf("business provider fixture listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}
