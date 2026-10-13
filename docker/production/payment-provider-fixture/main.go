// A deterministic signed payment provider used by the local Layer 229
// acceptance probes. It is intentionally small and keeps no customer data.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
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

type fixture struct {
	key      []byte
	authKey  string
	mu       sync.Mutex
	settled  map[string]settledCacheEntry
	requests map[string]int
	store    *controlPlane
}

type settledCacheEntry struct {
	payloadHash string
	response    []byte
}

func (f *fixture) sign(body []byte) string {
	mac := hmac.New(sha256.New, f.key)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func verifyRequestSignature(key []byte, value string, body []byte) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "sha256=")
	provided, err := hex.DecodeString(value)
	if err != nil || len(provided) != sha256.Size || len(key) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func rawString(fields map[string]json.RawMessage, name string) string {
	var value string
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return ""
	}
	return value
}

func rawInt64(fields map[string]json.RawMessage, name string) int64 {
	var value int64
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return 0
	}
	return value
}

func rawObject(fields map[string]json.RawMessage, name string) map[string]json.RawMessage {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return nil
	}
	return value
}

func objectInt64(value map[string]json.RawMessage, name string) int64 {
	var result int64
	if err := json.Unmarshal(value[name], &result); err != nil {
		return 0
	}
	return result
}

func objectString(value map[string]json.RawMessage, name string) string {
	var result string
	if err := json.Unmarshal(value[name], &result); err != nil {
		return ""
	}
	return result
}

type starsOffer struct {
	ID           int64
	Stars        int64
	StoreProduct string
	Currency     string
	Amount       int64
}

func parseStarsOffer(fields map[string]json.RawMessage) (starsOffer, error) {
	offerFields := rawObject(fields, "offer")
	if offerFields == nil {
		return starsOffer{}, fmt.Errorf("missing Stars offer")
	}
	offer := starsOffer{
		ID:           objectInt64(offerFields, "ID"),
		Stars:        objectInt64(offerFields, "Stars"),
		StoreProduct: objectString(offerFields, "StoreProduct"),
		Currency:     objectString(offerFields, "Currency"),
		Amount:       objectInt64(offerFields, "Amount"),
	}
	if offer.ID <= 0 || offer.Stars <= 0 || strings.TrimSpace(offer.Currency) == "" || offer.Amount < 0 {
		return starsOffer{}, fmt.Errorf("invalid Stars offer")
	}
	return offer, nil
}

func starsFormID(offerID int64) int64 {
	// Keep the fixture form ID stable across retries while avoiding catalog IDs
	// that could collide with the Premium form used by the same fixture.
	if offerID > 0 && offerID < 1<<50 {
		return 990002 + offerID
	}
	return 990002
}

func starsForm(offer starsOffer, formID int64) map[string]any {
	return map[string]any{
		"predicate_name": "payments_paymentFormStars",
		"form_id":        formID,
		"bot_id":         0,
		"title":          "Stars top-up",
		"description":    "Local Stars acceptance purchase",
		"invoice": map[string]any{
			"predicate_name": "invoice",
			"test":           true,
			"currency":       offer.Currency,
			"prices": []any{
				map[string]any{"predicate_name": "labeledPrice", "label": "Stars top-up", "amount": offer.Amount},
			},
		},
		"users": []any{},
	}
}

func settlementCacheKey(operation string, userID int64, requestKey string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "premium_subscription"
	}
	return fmt.Sprintf("%s\x00%d\x00%s", operation, userID, requestKey)
}

func settlementPayloadHash(fields map[string]json.RawMessage) (string, error) {
	body, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func userID(fields map[string]json.RawMessage) int64 {
	if value := rawInt64(fields, "user_id"); value > 0 {
		return value
	}
	if payload := rawObject(fields, "payload"); payload != nil {
		return rawInt64(payload, "user_id")
	}
	return 0
}

func providerUserID(fields map[string]json.RawMessage, operation string) int64 {
	if operation == "get_paid_messages_revenue" || operation == "toggle_no_paid_messages_exception" {
		if callerID := rawInt64(fields, "caller_user_id"); callerID > 0 {
			return callerID
		}
	}
	return userID(fields)
}

func premiumPromo() map[string]any {
	return map[string]any{
		"predicate_name": "help_premiumPromo",
		"currency":       "USD",
		"monthly_amount": 499,
		"period_options": []any{
			map[string]any{"predicate_name": "premiumSubscriptionOption", "months": 1, "currency": "USD", "amount": 499},
			map[string]any{"predicate_name": "premiumSubscriptionOption", "months": 12, "currency": "USD", "amount": 4990},
		},
	}
}

func emptyUpdates() map[string]any {
	return map[string]any{
		"predicate_name": "updates",
		"updates":        []any{},
		"users":          []any{},
		"chats":          []any{},
		"date":           time.Now().Unix(),
		"seq":            0,
	}
}

func premiumEnvelope(fields map[string]json.RawMessage, result any) ([]byte, string, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, "", err
	}
	envelope := map[string]any{
		"request_id": rawString(fields, "request_id"),
		"operation":  rawString(fields, "operation"),
		"verified":   true,
		"user_id":    userID(fields),
		"result":     json.RawMessage(resultJSON),
	}
	body, err := json.Marshal(envelope)
	return body, "X-Teamgram-Payment-Provider-Signature", err
}

func (f *fixture) response(fields map[string]json.RawMessage) ([]byte, string, error) {
	operation := rawString(fields, "operation")
	if rawString(fields, "request_id") != "" {
		var result any
		switch operation {
		case "help.getPremiumPromo":
			result = map[string]any{"promo": premiumPromo()}
		case "payments.canPurchasePremium", "payments.canPurchaseStore":
			result = map[string]any{"allowed": true}
		case "payments.assignAppStoreTransaction", "payments.assignPlayMarketTransaction":
			result = map[string]any{"accepted": true, "updates": emptyUpdates()}
		default:
			result = map[string]any{"accepted": true}
		}
		return premiumEnvelope(fields, result)
	}

	uid := userID(fields)
	if operation == "get_paid_messages_revenue" {
		callerID := rawInt64(fields, "caller_user_id")
		targetID := rawInt64(fields, "user_id")
		if callerID <= 0 || targetID <= 0 {
			return nil, "", fmt.Errorf("invalid paid-message revenue request")
		}
		body, err := json.Marshal(map[string]any{
			"verified":       true,
			"caller_user_id": callerID,
			"user_id":        targetID,
			"stars_amount":   int64(37),
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "toggle_no_paid_messages_exception" {
		callerID := rawInt64(fields, "caller_user_id")
		targetID := rawInt64(fields, "user_id")
		if callerID <= 0 || targetID <= 0 {
			return nil, "", fmt.Errorf("invalid paid-message exception request")
		}
		body, err := json.Marshal(map[string]any{
			"verified":       true,
			"caller_user_id": callerID,
			"user_id":        targetID,
			"result":         true,
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "get_bot_invoice_form" {
		botID := rawInt64(fields, "bot_user_id")
		title := rawString(fields, "title")
		description := rawString(fields, "description")
		invoice := fields["invoice"]
		if botID <= 0 || strings.TrimSpace(title) == "" || strings.TrimSpace(description) == "" || len(invoice) == 0 {
			return nil, "", fmt.Errorf("invalid bot invoice form request")
		}
		body, err := json.Marshal(map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": botID,
			"bot_user_id":         botID,
			"product_type":        "bot_invoice",
			"form": map[string]any{
				"predicate_name": "payments_paymentForm",
				"form_id":        int64(990003),
				"bot_id":         botID,
				"title":          title,
				"description":    description,
				"invoice":        json.RawMessage(invoice),
				"users":          []any{},
			},
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "validate_bot_invoice_info" {
		botID := rawInt64(fields, "bot_user_id")
		if botID <= 0 {
			return nil, "", fmt.Errorf("invalid bot invoice validation request")
		}
		body, err := json.Marshal(map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": botID,
			"bot_user_id":         botID,
			"product_type":        "bot_invoice",
			"id":                  "fixture-bot-info",
			"shipping_options":    []any{},
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "settle_bot_invoice" {
		botID := rawInt64(fields, "bot_user_id")
		requestKey := rawString(fields, "request_key")
		fingerprint := rawString(fields, "fingerprint")
		formID := rawInt64(fields, "form_id")
		currency := rawString(fields, "currency")
		amount := rawInt64(fields, "amount")
		if botID <= 0 || requestKey == "" || fingerprint == "" || formID <= 0 || strings.TrimSpace(currency) == "" || amount < 0 {
			return nil, "", fmt.Errorf("invalid bot invoice settlement request")
		}
		cacheKey := settlementCacheKey(operation, uid, requestKey)
		payloadHash, err := settlementPayloadHash(fields)
		if err != nil {
			return nil, "", err
		}
		f.mu.Lock()
		if cached, ok := f.settled[cacheKey]; ok {
			if cached.payloadHash != payloadHash {
				f.mu.Unlock()
				return nil, "", errors.New("payment provider idempotency conflict")
			}
			f.mu.Unlock()
			return append([]byte(nil), cached.response...), "X-Teamgram-Payment-Signature", nil
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", uid, requestKey)))
		transactionID := "fixture-bot-" + hex.EncodeToString(digest[:8])
		result := map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": botID,
			"bot_user_id":         botID,
			"request_key":         requestKey,
			"fingerprint":         fingerprint,
			"form_id":             formID,
			"product_type":        "bot_invoice",
			"transaction_id":      transactionID,
			"currency":            currency,
			"amount":              amount,
			"title":               "Bot invoice",
			"receipt":             map[string]any{"provider": "local-fixture", "transaction_id": transactionID},
		}
		body, err := json.Marshal(result)
		if err == nil {
			f.settled[cacheKey] = settledCacheEntry{payloadHash: payloadHash, response: append([]byte(nil), body...)}
		}
		f.mu.Unlock()
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "get_stars_form" {
		if rawString(fields, "required_product") != "stars_topup" {
			return nil, "", fmt.Errorf("unsupported Stars product")
		}
		offer, err := parseStarsOffer(fields)
		if err != nil {
			return nil, "", err
		}
		formID := starsFormID(offer.ID)
		body, err := json.Marshal(map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": rawInt64(fields, "beneficiary_user_id"),
			"product_type":        "stars_topup",
			"offer_id":            offer.ID,
			"stars":               offer.Stars,
			"store_product":       offer.StoreProduct,
			"currency":            offer.Currency,
			"amount":              offer.Amount,
			"form":                starsForm(offer, formID),
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "settle_stars" {
		if rawString(fields, "required_product") != "stars_topup" {
			return nil, "", fmt.Errorf("unsupported Stars product")
		}
		offer, err := parseStarsOffer(fields)
		if err != nil {
			return nil, "", err
		}
		requestKey := rawString(fields, "request_key")
		fingerprint := rawString(fields, "fingerprint")
		formID := rawInt64(fields, "form_id")
		if requestKey == "" || fingerprint == "" || formID <= 0 {
			return nil, "", fmt.Errorf("invalid Stars settlement request")
		}
		cacheKey := settlementCacheKey(operation, uid, requestKey)
		payloadHash, err := settlementPayloadHash(fields)
		if err != nil {
			return nil, "", err
		}
		f.mu.Lock()
		if cached, ok := f.settled[cacheKey]; ok {
			if cached.payloadHash != payloadHash {
				f.mu.Unlock()
				return nil, "", errors.New("payment provider idempotency conflict")
			}
			f.mu.Unlock()
			return append([]byte(nil), cached.response...), "X-Teamgram-Payment-Signature", nil
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", uid, requestKey)))
		transactionID := "fixture-stars-" + hex.EncodeToString(digest[:8])
		result := map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": rawInt64(fields, "beneficiary_user_id"),
			"request_key":         requestKey,
			"fingerprint":         fingerprint,
			"form_id":             formID,
			"product_type":        "stars_topup",
			"offer_id":            offer.ID,
			"stars":               offer.Stars,
			"store_product":       offer.StoreProduct,
			"transaction_id":      transactionID,
			"currency":            offer.Currency,
			"amount":              offer.Amount,
			"title":               "Stars top-up",
			"receipt":             map[string]any{"provider": "local-fixture", "transaction_id": transactionID},
		}
		body, err := json.Marshal(result)
		if err == nil {
			f.settled[cacheKey] = settledCacheEntry{payloadHash: payloadHash, response: append([]byte(nil), body...)}
		}
		f.mu.Unlock()
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "get_form" {
		body, err := json.Marshal(map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": uid,
			"product_type":        "premium_subscription",
			"premium_months":      1,
			"currency":            "USD",
			"amount":              499,
			"form": map[string]any{
				"predicate_name": "payments_paymentForm",
				"form_id":        990001,
				"bot_id":         uid,
				"title":          "Telegram Premium",
				"description":    "Local Premium acceptance purchase",
				"provider_id":    1,
				"invoice": map[string]any{
					"predicate_name": "invoice",
					"test":           true,
					"currency":       "USD",
					"prices": []any{
						map[string]any{"predicate_name": "labeledPrice", "label": "Telegram Premium", "amount": 499},
					},
				},
			},
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	if operation == "validate_requested_info" {
		body, err := json.Marshal(map[string]any{
			"verified":            true,
			"user_id":             uid,
			"beneficiary_user_id": uid,
			"product_type":        "premium_subscription",
			"id":                  "fixture-info",
			"shipping_options":    []any{},
		})
		return body, "X-Teamgram-Payment-Signature", err
	}
	// The HTTP boundary normalizes legacy Premium settlement requests (which
	// omit operation) to this stable control-plane operation name before
	// dispatch. Keep that normalized value valid here so retries can load the
	// same PostgreSQL idempotency row.
	if operation != "" && operation != "premium_subscription" {
		return nil, "", fmt.Errorf("unsupported provider operation %q", operation)
	}
	if rawString(fields, "required_product") != "premium_subscription" {
		return nil, "", fmt.Errorf("unsupported provider operation %q", operation)
	}

	payload := fields
	requestKey := rawString(payload, "request_key")
	if requestKey == "" {
		requestKey = rawString(payload, "idempotency_key")
	}
	fingerprint := rawString(payload, "fingerprint")
	formID := rawInt64(payload, "form_id")
	if requestKey == "" {
		return nil, "", fmt.Errorf("unsupported provider operation %q", operation)
	}
	cacheKey := settlementCacheKey(operation, uid, requestKey)
	payloadHash, err := settlementPayloadHash(fields)
	if err != nil {
		return nil, "", err
	}
	f.mu.Lock()
	if cached, ok := f.settled[cacheKey]; ok {
		if cached.payloadHash != payloadHash {
			f.mu.Unlock()
			return nil, "", errors.New("payment provider idempotency conflict")
		}
		f.mu.Unlock()
		return append([]byte(nil), cached.response...), "X-Teamgram-Payment-Signature", nil
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", uid, requestKey)))
	transactionID := "fixture-premium-" + hex.EncodeToString(digest[:8])
	result := map[string]any{
		"verified":            true,
		"user_id":             uid,
		"beneficiary_user_id": uid,
		"request_key":         requestKey,
		"fingerprint":         fingerprint,
		"form_id":             formID,
		"product_type":        "premium_subscription",
		"premium_months":      1,
		"transaction_id":      transactionID,
		"currency":            "USD",
		"amount":              499,
		"title":               "Telegram Premium",
		"receipt":             map[string]any{"provider": "local-fixture", "transaction_id": transactionID},
	}
	body, err := json.Marshal(result)
	if err == nil {
		f.settled[cacheKey] = settledCacheEntry{payloadHash: payloadHash, response: append([]byte(nil), body...)}
	}
	f.mu.Unlock()
	return body, "X-Teamgram-Payment-Signature", err
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

func (f *fixture) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if f == nil || f.store == nil {
		http.Error(w, "provider database unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := f.store.Ping(ctx); err != nil {
		http.Error(w, "provider database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
		f.healthz(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if f.authKey != "" && !validBearerAuthorization(r.Header.Get("Authorization"), f.authKey) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	if f.authKey != "" && !verifyRequestSignature(f.key, r.Header.Get("X-Teamgram-Payment-Provider-Request-Signature"), body) {
		http.Error(w, "invalid signed request", http.StatusUnauthorized)
		return
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(body, &fields); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	operation := rawString(fields, "operation")
	effective := fields
	if payload := rawObject(fields, "payload"); payload != nil {
		// Premium and business providers wrap the operation payload in a
		// signed envelope. Keep the envelope identity while making the
		// operation-specific fields available to the fixture implementation.
		effective = make(map[string]json.RawMessage, len(fields)+len(payload))
		for key, value := range fields {
			effective[key] = value
		}
		for key, value := range payload {
			effective[key] = value
		}
		if operation != "" {
			effective["operation"] = fields["operation"]
		}
		if _, ok := effective["request_id"]; !ok {
			effective["request_id"] = fields["request_id"]
		}
	}
	if operation == "" {
		operation = rawString(effective, "operation")
	}
	// Premium settlement requests from older clients omit operation and carry
	// only required_product. Normalize before deriving the idempotency key so
	// the control plane sees the same identity on every retry.
	if operation == "" && rawString(effective, "required_product") == "premium_subscription" {
		operation = "premium_subscription"
	}
	if operation != "" {
		encodedOperation, marshalErr := json.Marshal(operation)
		if marshalErr != nil {
			http.Error(w, "invalid operation", http.StatusBadRequest)
			return
		}
		effective["operation"] = encodedOperation
	}
	requestKey := providerRequestKey(effective, operation, body)
	uid := providerUserID(effective, operation)
	business := r.Header.Get("X-Teamgram-Business-Provider-Request-Signature") != ""
	if business {
		http.Error(w, "payment provider fixture does not serve business operations", http.StatusNotImplemented)
		return
	}
	signatureHeader := "X-Teamgram-Payment-Signature"
	if rawString(fields, "request_id") != "" {
		signatureHeader = "X-Teamgram-Payment-Provider-Signature"
	}
	if f.store != nil {
		if cached, found, loadErr := f.store.loadOperation(operation, requestKey, uid, body); loadErr != nil {
			http.Error(w, loadErr.Error(), http.StatusConflict)
			return
		} else if found {
			f.recordRequest(operation)
			w.Header().Set(cached.Header, f.sign(cached.Response))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(cached.Response)
			return
		}
	}
	var response []byte
	var header string
	response, header, err = f.response(effective)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if operation == "" {
		operation = rawString(effective, "required_product")
	}
	count := f.recordRequest(operation)
	log.Printf("operation=%s user_id=%d request_count=%d", operation, uid, count)
	if header == "" {
		header = signatureHeader
	}
	if f.store != nil {
		transactionID := responseTransactionID(response)
		product, months, stars := responseEntitlement(response)
		stored, saveErr := f.store.saveOperation(operation, requestKey, uid, body, response, header, transactionID, product, months, stars)
		if saveErr != nil {
			http.Error(w, saveErr.Error(), http.StatusConflict)
			return
		}
		response = stored.Response
		header = stored.Header
	}
	w.Header().Set(header, f.sign(response))
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(response)
}

func (f *fixture) recordRequest(operation string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests[operation]++
	return f.requests[operation]
}

func responseTransactionID(body []byte) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return ""
	}
	if value := rawString(fields, "transaction_id"); value != "" {
		return value
	}
	if result := rawObject(fields, "result"); result != nil {
		return rawString(result, "transaction_id")
	}
	return ""
}

func responseEntitlement(body []byte) (string, int64, int64) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return "", 0, 0
	}
	if result := rawObject(fields, "result"); result != nil {
		fields = result
	}
	// Form discovery and validation responses carry product metadata but do
	// not settle a transaction. Only settlement responses may create an
	// entitlement in the provider control plane.
	if rawString(fields, "transaction_id") == "" {
		return "", 0, 0
	}
	product := rawString(fields, "product_type")
	if product == "" {
		return "", 0, 0
	}
	// Bot invoices are one-time charges. Their receipt and transaction are
	// durable provider records, but they do not grant an account entitlement.
	// Returning them here would violate the entitlement table invariant that
	// at least one of months or stars is positive and make PostgreSQL-backed
	// checkout fail after the provider already accepted the charge.
	switch product {
	case "premium_subscription":
		months := rawInt64(fields, "premium_months")
		if months <= 0 {
			return "", 0, 0
		}
		return product, months, 0
	case "stars_topup":
		stars := rawInt64(fields, "stars")
		if stars <= 0 {
			return "", 0, 0
		}
		return product, 0, stars
	default:
		return "", 0, 0
	}
}

func main() {
	key := strings.TrimSpace(os.Getenv("PAYMENT_PROVIDER_SIGNING_KEY"))
	if len(key) < 32 {
		log.Fatal("PAYMENT_PROVIDER_SIGNING_KEY must contain at least 32 bytes")
	}
	port := strings.TrimSpace(os.Getenv("PAYMENT_PROVIDER_PORT"))
	if port == "" {
		port = "18080"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		log.Fatalf("invalid PAYMENT_PROVIDER_PORT: %v", err)
	}
	authKey := strings.TrimSpace(os.Getenv("PAYMENT_PROVIDER_KEY"))
	if authKey == "" {
		log.Fatal("PAYMENT_PROVIDER_KEY must be set")
	}
	store, err := openControlPlane(os.Getenv("TEAMGRAM_POSTGRES_DSN"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = store.close() }()
	provider := &fixture{key: []byte(key), authKey: authKey, settled: map[string]settledCacheEntry{}, requests: map[string]int{}, store: store}
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer healthCancel()
		if err := store.Ping(healthCtx); err != nil {
			log.Fatalf("provider readiness check failed: %v", err)
		}
		return
	}
	if len(os.Args) > 1 {
		log.Fatal("unsupported argument")
	}
	server := &http.Server{Addr: ":" + port, Handler: provider}
	log.Printf("payment provider fixture listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
