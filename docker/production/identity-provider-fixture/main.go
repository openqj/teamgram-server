// Signed identity provider used by isolated Layer 229 acceptance probes.
//
// The fixture is an explicit test authority. It does not create users or
// credentials and never acts as an application-side fallback. A request is
// accepted only when the corresponding token/bot is provisioned through the
// deployment environment.
package main

import (
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
	"time"
)

const (
	defaultPort = "23192"
	bodyLimit   = 1 << 20
)

type provider struct {
	authKey     string
	signingKey  []byte
	webTokens   map[string]int64
	allowedBots map[int64]struct{}
}

type authRequest struct {
	RequestID    string `json:"request_id"`
	Operation    string `json:"operation"`
	APIID        int32  `json:"api_id"`
	APIHash      string `json:"api_hash"`
	BotAuthToken string `json:"bot_auth_token,omitempty"`
	WebAuthToken string `json:"web_auth_token,omitempty"`
}

type authResponse struct {
	RequestID string `json:"request_id"`
	Verified  bool   `json:"verified"`
	Operation string `json:"operation"`
	APIID     int32  `json:"api_id"`
	UserID    int64  `json:"user_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

type paidAuthRequest struct {
	RequestID     string `json:"request_id"`
	Operation     string `json:"operation"`
	PhoneNumber   string `json:"phone_number"`
	PhoneCodeHash string `json:"phone_code_hash"`
	FormID        int64  `json:"form_id"`
}

type paidAuthResponse struct {
	RequestID           string `json:"request_id"`
	Operation           string `json:"operation"`
	Verified            bool   `json:"verified"`
	PhoneNumber         string `json:"phone_number"`
	PhoneCodeHash       string `json:"phone_code_hash"`
	StoreProduct        string `json:"store_product"`
	SupportEmailAddress string `json:"support_email_address"`
	SupportEmailSubject string `json:"support_email_subject"`
	PremiumDays         int32  `json:"premium_days"`
	Currency            string `json:"currency"`
	Amount              int64  `json:"amount"`
	Error               string `json:"error,omitempty"`
}

type passportRequest struct {
	RequestID   string          `json:"request_id"`
	Operation   string          `json:"operation"`
	UserID      int64           `json:"user_id"`
	BotID       int64           `json:"bot_id"`
	Scope       string          `json:"scope"`
	PublicKey   string          `json:"public_key"`
	ValueHashes json.RawMessage `json:"value_hashes"`
	Credentials json.RawMessage `json:"credentials"`
}

type passportResponse struct {
	RequestID string `json:"request_id"`
	Operation string `json:"operation"`
	Verified  bool   `json:"verified"`
	UserID    int64  `json:"user_id"`
	BotID     int64  `json:"bot_id"`
	Scope     string `json:"scope"`
	PublicKey string `json:"public_key"`
	Result    *bool  `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

type connectionRequest struct {
	RequestID string          `json:"request_id"`
	Operation string          `json:"operation"`
	UserID    int64           `json:"user_id"`
	Payload   json.RawMessage `json:"payload"`
}

type connectionPayload struct {
	BotID int64 `json:"bot_id"`
}

type connectionResponse struct {
	RequestID string `json:"request_id"`
	Operation string `json:"operation"`
	Verified  bool   `json:"verified"`
	UserID    int64  `json:"user_id"`
	Result    any    `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (p *provider) sign(body []byte) string {
	mac := hmac.New(sha256.New, p.signingKey)
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
	if expected == "" || !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(value, "Bearer ")
	return provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (p *provider) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !validBearerAuthorization(r.Header.Get("Authorization"), p.authKey) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		http.Error(w, "idempotency key required", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) == 0 || len(body) > bodyLimit {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	requestSignature := r.Header.Get("X-Teamgram-Auth-Provider-Request-Signature")
	responseSignature := "X-Teamgram-Auth-Provider-Signature"
	if strings.TrimSpace(requestSignature) == "" {
		requestSignature = r.Header.Get("X-Teamgram-Passport-Provider-Request-Signature")
		responseSignature = "X-Teamgram-Passport-Provider-Signature"
	}
	if strings.TrimSpace(requestSignature) == "" {
		requestSignature = r.Header.Get("X-Teamgram-Bot-Connection-Provider-Request-Signature")
		responseSignature = "X-Teamgram-Bot-Connection-Provider-Signature"
	}
	if !verifySignature(p.signingKey, requestSignature, body) {
		http.Error(w, "invalid signed request", http.StatusUnauthorized)
		return
	}

	response, err := p.dispatch(body)
	if err != nil {
		// A signed, verified=false response lets the BFF map the failure to its
		// canonical RPC error while preserving the provider trust boundary.
		response = errResponse(body, err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(responseSignature, p.sign(encoded))
	_, _ = w.Write(encoded)
}

func (p *provider) dispatch(body []byte) (any, error) {
	var operation struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(body, &operation); err != nil || strings.TrimSpace(operation.Operation) == "" {
		return nil, errors.New("operation is required")
	}
	switch operation.Operation {
	case "verify_web_token_authorization", "verify_bot_authorization":
		var request authRequest
		if err := json.Unmarshal(body, &request); err != nil || request.RequestID == "" || request.APIID <= 0 || request.APIHash == "" {
			return nil, errors.New("invalid auth provider request")
		}
		var token string
		if operation.Operation == "verify_web_token_authorization" {
			token = request.WebAuthToken
		} else {
			token = request.BotAuthToken
		}
		userID, ok := p.webTokens[token]
		if !ok || token == "" {
			return authResponse{RequestID: request.RequestID, Verified: false, Operation: request.Operation, APIID: request.APIID, Error: "token is not provisioned"}, nil
		}
		return authResponse{RequestID: request.RequestID, Verified: true, Operation: request.Operation, APIID: request.APIID, UserID: userID}, nil
	case "auth.checkPaidAuth":
		var request paidAuthRequest
		if err := json.Unmarshal(body, &request); err != nil || request.RequestID == "" || strings.TrimSpace(request.PhoneNumber) == "" || strings.TrimSpace(request.PhoneCodeHash) == "" || request.FormID <= 0 {
			return nil, errors.New("invalid paid auth provider request")
		}
		return paidAuthResponse{
			RequestID: request.RequestID, Operation: request.Operation, Verified: true,
			PhoneNumber: request.PhoneNumber, PhoneCodeHash: request.PhoneCodeHash,
			StoreProduct: "telegram-premium-monthly", SupportEmailAddress: "support@example.test",
			SupportEmailSubject: "Paid sign-in", PremiumDays: 30, Currency: "USD", Amount: 499,
		}, nil
	case "account.acceptAuthorization":
		var request passportRequest
		if err := json.Unmarshal(body, &request); err != nil || request.RequestID == "" || request.UserID <= 0 || request.BotID <= 0 || request.Scope == "" || request.PublicKey == "" || len(request.Credentials) == 0 || string(request.Credentials) == "null" {
			return nil, errors.New("invalid passport provider request")
		}
		if _, ok := p.allowedBots[request.BotID]; !ok {
			return passportResponse{RequestID: request.RequestID, Operation: request.Operation, Verified: false, UserID: request.UserID, BotID: request.BotID, Scope: request.Scope, PublicKey: request.PublicKey, Error: "bot is not provisioned"}, nil
		}
		result := true
		return passportResponse{RequestID: request.RequestID, Operation: request.Operation, Verified: true, UserID: request.UserID, BotID: request.BotID, Scope: request.Scope, PublicKey: request.PublicKey, Result: &result}, nil
	case "account.confirmBotConnection":
		var request connectionRequest
		if err := json.Unmarshal(body, &request); err != nil || request.RequestID == "" || request.UserID <= 0 || len(request.Payload) == 0 || string(request.Payload) == "null" {
			return nil, errors.New("invalid bot connection provider request")
		}
		var payload connectionPayload
		if err := json.Unmarshal(request.Payload, &payload); err != nil || payload.BotID <= 0 {
			return nil, errors.New("invalid bot connection payload")
		}
		if _, ok := p.allowedBots[payload.BotID]; !ok {
			return connectionResponse{RequestID: request.RequestID, Operation: request.Operation, Verified: false, UserID: request.UserID, Error: "bot is not provisioned"}, nil
		}
		return connectionResponse{RequestID: request.RequestID, Operation: request.Operation, Verified: true, UserID: request.UserID, Result: map[string]bool{"confirmed": true}}, nil
	default:
		return nil, fmt.Errorf("operation %q is not provisioned", operation.Operation)
	}
}

func errResponse(body []byte, err error) any {
	var operation struct {
		RequestID string `json:"request_id"`
		Operation string `json:"operation"`
		APIID     int32  `json:"api_id"`
		UserID    int64  `json:"user_id"`
		BotID     int64  `json:"bot_id"`
		Scope     string `json:"scope"`
		PublicKey string `json:"public_key"`
	}
	_ = json.Unmarshal(body, &operation)
	switch operation.Operation {
	case "verify_web_token_authorization", "verify_bot_authorization":
		return authResponse{RequestID: operation.RequestID, Verified: false, Operation: operation.Operation, APIID: operation.APIID, Error: err.Error()}
	case "auth.checkPaidAuth":
		return paidAuthResponse{RequestID: operation.RequestID, Operation: operation.Operation, Verified: false, Error: err.Error()}
	case "account.acceptAuthorization":
		return passportResponse{RequestID: operation.RequestID, Operation: operation.Operation, Verified: false, UserID: operation.UserID, BotID: operation.BotID, Scope: operation.Scope, PublicKey: operation.PublicKey, Error: err.Error()}
	default:
		return connectionResponse{RequestID: operation.RequestID, Operation: operation.Operation, Verified: false, UserID: operation.UserID, Error: err.Error()}
	}
}

func (p *provider) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func parseWebTokens(raw string) (map[string]int64, error) {
	result := make(map[string]int64)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, errors.New("IDENTITY_PROVIDER_WEB_TOKENS must use token=user_id pairs")
		}
		userID, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || userID <= 0 {
			return nil, errors.New("IDENTITY_PROVIDER_WEB_TOKENS contains an invalid user id")
		}
		result[strings.TrimSpace(parts[0])] = userID
	}
	if len(result) == 0 {
		return nil, errors.New("IDENTITY_PROVIDER_WEB_TOKENS must contain at least one token")
	}
	return result, nil
}

func parseBotIDs(raw string) (map[int64]struct{}, error) {
	result := make(map[int64]struct{})
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("IDENTITY_PROVIDER_BOT_IDS contains an invalid user id")
		}
		result[id] = struct{}{}
	}
	if len(result) == 0 {
		return nil, errors.New("IDENTITY_PROVIDER_BOT_IDS must contain at least one bot id")
	}
	return result, nil
}

func loadProvider() (*provider, error) {
	authKey := strings.TrimSpace(os.Getenv("TEAMGRAM_IDENTITY_PROVIDER_KEY"))
	signingKey := strings.TrimSpace(os.Getenv("TEAMGRAM_IDENTITY_PROVIDER_SIGNING_KEY"))
	if authKey == "" || len(signingKey) < 32 {
		return nil, errors.New("TEAMGRAM_IDENTITY_PROVIDER_KEY and TEAMGRAM_IDENTITY_PROVIDER_SIGNING_KEY (at least 32 bytes) are required")
	}
	webTokens, err := parseWebTokens(os.Getenv("IDENTITY_PROVIDER_WEB_TOKENS"))
	if err != nil {
		return nil, err
	}
	allowedBots, err := parseBotIDs(os.Getenv("IDENTITY_PROVIDER_BOT_IDS"))
	if err != nil {
		return nil, err
	}
	return &provider{authKey: authKey, signingKey: []byte(signingKey), webTokens: webTokens, allowedBots: allowedBots}, nil
}

func main() {
	p, err := loadProvider()
	if err != nil {
		log.Fatal(err)
	}
	port := strings.TrimSpace(os.Getenv("IDENTITY_PROVIDER_PORT"))
	if port == "" {
		port = defaultPort
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		log.Fatal("IDENTITY_PROVIDER_PORT must be from 1 to 65535")
	}
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		return
	}
	if len(os.Args) > 1 {
		log.Fatal("unsupported argument")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", p.healthz)
	mux.HandleFunc("/", p.handle)
	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("identity provider fixture listening on :%s", port)
	log.Fatal(server.ListenAndServe())
}
