// Package providercontract contains the wire-level contract shared by
// provider clients and their local acceptance fixtures.
package providercontract

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const SignaturePrefix = "sha256="

// RequestEnvelope is the common request shape used by provider boundaries.
// Payload remains opaque so each provider can preserve its own method data.
type RequestEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	UserID    int64           `json:"user_id"`
	Payload   json.RawMessage `json:"payload"`
}

// ResponseEnvelope is the common authenticated response shape. Result is
// intentionally raw because its constructor is owned by the provider method.
type ResponseEnvelope struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	Verified  bool            `json:"verified"`
	UserID    int64           `json:"user_id"`
	Error     string          `json:"error,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

// Signature returns the HMAC-SHA256 value used on provider requests and
// responses. The prefix is part of the wire contract.
func Signature(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(key)))
	_, _ = mac.Write(body)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks an HMAC-SHA256 signature in constant time.
func VerifySignature(key, value string, body []byte) bool {
	key = strings.TrimSpace(key)
	value = strings.TrimPrefix(strings.TrimSpace(value), SignaturePrefix)
	provided, err := hex.DecodeString(value)
	if err != nil || len(provided) != sha256.Size || key == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

// IdempotencyKey derives a stable key from the authenticated caller, method,
// and exact JSON payload. Callers that intentionally need a per-attempt
// request ID should keep that request ID separate from this key.
func IdempotencyKey(userID int64, operation string, payload []byte) string {
	input := []byte(strconv.FormatInt(userID, 10) + "\x00" + strings.TrimSpace(operation) + "\x00")
	input = append(input, payload...)
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

// SetRequestHeaders applies the common authentication headers. Provider
// specific signature header names remain explicit at the call site because
// they are part of each deployed boundary's compatibility contract.
func SetRequestHeaders(req *http.Request, body []byte, bearerToken, idempotencyKey, signatureHeader, signingKey string) {
	if req == nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearerToken))
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(idempotencyKey))
	}
	if strings.TrimSpace(signatureHeader) != "" && strings.TrimSpace(signingKey) != "" {
		req.Header.Set(signatureHeader, Signature(signingKey, body))
	}
}

// VerifyResponse authenticates an HTTP response body before its envelope is
// decoded. It deliberately does not interpret provider errors.
func VerifyResponse(resp *http.Response, body []byte, signatureHeader, signingKey string) bool {
	return resp != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices &&
		VerifySignature(signingKey, resp.Header.Get(signatureHeader), body)
}

// ValidateResponse checks the identity fields shared by all envelope based
// providers and returns the opaque result for method-specific decoding.
func ValidateResponse(body []byte, request RequestEnvelope) (ResponseEnvelope, error) {
	var response ResponseEnvelope
	if err := json.Unmarshal(body, &response); err != nil {
		return ResponseEnvelope{}, err
	}
	if !response.Verified || response.RequestID != request.RequestID ||
		response.Operation != request.Operation || response.UserID != request.UserID {
		return ResponseEnvelope{}, errors.New("provider response identity mismatch")
	}
	if response.Error != "" || len(response.Result) == 0 || string(response.Result) == "null" {
		return ResponseEnvelope{}, errors.New("provider response contains an error or empty result")
	}
	return response, nil
}

type DeliveryState string

const (
	DeliveryPending  DeliveryState = "pending"
	DeliveryInflight DeliveryState = "inflight"
	DeliveryComplete DeliveryState = "complete"
)

func ValidDeliveryState(value string) bool {
	switch DeliveryState(value) {
	case DeliveryPending, DeliveryInflight, DeliveryComplete:
		return true
	default:
		return false
	}
}

// CanTransition describes the durable delivery-intent state machine. A
// completed intent is idempotently complete; it can never return to pending.
func CanTransition(from, to string) bool {
	if from == string(DeliveryComplete) && to == string(DeliveryComplete) {
		return true
	}
	switch from {
	case string(DeliveryPending):
		return to == string(DeliveryInflight)
	case string(DeliveryInflight):
		return to == string(DeliveryPending) || to == string(DeliveryComplete)
	default:
		return false
	}
}
