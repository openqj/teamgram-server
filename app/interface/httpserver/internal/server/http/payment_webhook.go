package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/app/bff/apifull/paymentrecovery"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/providercontract"
)

const (
	paymentWebhookSignatureHeader         = "X-Teamgram-Payment-Provider-Webhook-Signature"
	paymentWebhookFallbackSignatureHeader = "X-Teamgram-Payment-Provider-Signature"
)

type paymentProviderWebhookPayload struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	Provider      string `json:"provider"`
	UserID        int64  `json:"user_id"`
	RequestKey    string `json:"request_key"`
	TransactionID string `json:"transaction_id"`
	Product       string `json:"product"`
	Months        int32  `json:"months"`
	Stars         int64  `json:"stars"`
	OccurredAt    int64  `json:"occurred_at"`
	Reason        string `json:"reason"`
}

func paymentProviderWebhookHandler(serverCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if serverCtx == nil {
			http.Error(w, "payment recovery is unavailable", http.StatusServiceUnavailable)
			return
		}
		key := strings.TrimSpace(serverCtx.Config.PaymentProviderWebhookSigningKey)
		if key == "" {
			http.Error(w, "payment webhook is not configured", http.StatusServiceUnavailable)
			return
		}
		maxBodyBytes := serverCtx.Config.PaymentProviderWebhookMaxBodyBytes
		if maxBodyBytes <= 0 || maxBodyBytes > 8<<20 {
			maxBodyBytes = 1 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid webhook body", http.StatusRequestEntityTooLarge)
			return
		}
		signature := r.Header.Get(paymentWebhookSignatureHeader)
		if signature == "" {
			signature = r.Header.Get(paymentWebhookFallbackSignatureHeader)
		}
		if !providercontract.VerifySignature(key, signature, body) {
			http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
			return
		}
		var payload paymentProviderWebhookPayload
		if err = json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid webhook JSON", http.StatusBadRequest)
			return
		}
		canonicalProvider := strings.TrimSpace(serverCtx.Config.PaymentProviderName)
		if canonicalProvider == "" {
			canonicalProvider = "external"
		}
		if len(canonicalProvider) > 32 {
			http.Error(w, "payment provider configuration is invalid", http.StatusServiceUnavailable)
			return
		}
		if (payload.Product == "premium_subscription" || payload.Product == "stars_topup") &&
			strings.TrimSpace(payload.Provider) != canonicalProvider {
			http.Error(w, "webhook provider does not match the configured payment provider", http.StatusBadRequest)
			return
		}
		if payload.EventID == "" {
			digest := sha256.Sum256(body)
			payload.EventID = "payload:" + hex.EncodeToString(digest[:])
		}
		if payload.OccurredAt <= 0 {
			payload.OccurredAt = time.Now().Unix()
		}
		event := paymentrecovery.WebhookEvent{
			EventID: payload.EventID, EventType: payload.EventType, Provider: payload.Provider,
			UserID: payload.UserID, RequestKey: payload.RequestKey, TransactionID: payload.TransactionID,
			Product: payload.Product, Months: payload.Months, Stars: payload.Stars,
			OccurredAt: payload.OccurredAt, Reason: payload.Reason, Payload: body,
		}
		_, err = paymentrecovery.EnqueueWebhook(r.Context(), event, body)
		if err != nil {
			if errors.Is(err, paymentrecovery.ErrWebhookConflict) {
				http.Error(w, "webhook event conflict", http.StatusConflict)
				return
			}
			if errors.Is(err, paymentrecovery.ErrInvalidWebhook) {
				http.Error(w, "invalid webhook event", http.StatusBadRequest)
				return
			}
			http.Error(w, "payment webhook unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		response, _ := json.Marshal(map[string]any{"accepted": true, "event_id": event.EventID})
		_, _ = w.Write(response)
	}
}
