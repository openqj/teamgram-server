package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/providercontract"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type paymentProviderStatusResponse struct {
	Verified      bool   `json:"verified"`
	UserID        int64  `json:"user_id"`
	Provider      string `json:"provider"`
	RequestKey    string `json:"request_key"`
	TransactionID string `json:"transaction_id"`
	Product       string `json:"product"`
	Status        string `json:"status"`
	Months        int32  `json:"months"`
	Stars         int64  `json:"stars"`
	EventID       string `json:"event_id"`
	OccurredAt    int64  `json:"occurred_at"`
	Reason        string `json:"reason"`
}

func RunPaymentRecoveryReconciler(ctx context.Context, svcCtx *svc.ServiceContext) {
	interval := 30 * time.Second
	batchSize := 50
	if svcCtx != nil {
		if svcCtx.Config.PaymentReconciliationIntervalSeconds > 0 {
			interval = time.Duration(svcCtx.Config.PaymentReconciliationIntervalSeconds) * time.Second
		}
		if svcCtx.Config.PaymentReconciliationBatchSize > 0 {
			batchSize = svcCtx.Config.PaymentReconciliationBatchSize
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var unavailable bool
	for {
		err := reconcilePaymentRecovery(ctx, svcCtx, batchSize)
		if err != nil && !unavailable {
			logx.WithContext(ctx).Errorf("payment recovery reconciler unavailable: %v", err)
			unavailable = true
		} else if err == nil && unavailable {
			logx.WithContext(ctx).Info("payment recovery reconciler available")
			unavailable = false
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reconcilePaymentRecovery(ctx context.Context, svcCtx *svc.ServiceContext, batchSize int) error {
	if svcCtx == nil {
		return nil
	}
	if err := domain.CheckPaymentRecoverySchema(ctx); err != nil {
		return err
	}
	release, acquired, err := domain.LockPaymentRecoveryReconciler(ctx)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer release()
	if err = domain.EnsurePaymentReconciliations(ctx, batchSize); err != nil {
		return err
	}
	if err = processPaymentWebhooks(ctx, svcCtx, batchSize); err != nil {
		return err
	}
	return processPaymentReconciliations(ctx, svcCtx, batchSize)
}

func processPaymentWebhooks(ctx context.Context, svcCtx *svc.ServiceContext, batchSize int) error {
	events, err := domain.ClaimPaymentWebhooks(ctx, batchSize)
	if err != nil {
		return err
	}
	for _, event := range events {
		result, applyErr := domain.ApplyPaymentReversal(ctx, event)
		if applyErr == nil && result.NeedPremium {
			if svcCtx.Dao == nil || svcCtx.Dao.UserClient == nil {
				applyErr = errors.New("user service is unavailable for Premium revoke")
			} else {
				callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				var response *mtproto.Bool
				response, applyErr = svcCtx.Dao.UserClient.UserUpdatePremium(callCtx, &userpb.TLUserUpdatePremium{
					UserId:        result.UserID,
					Premium:       mtproto.ToBool(false),
					Provider:      result.Provider,
					TransactionId: result.TransactionID,
					Months:        wrapperspb.Int32(result.Months),
				})
				cancel()
				if applyErr == nil && (response == nil || !mtproto.FromBool(response)) {
					applyErr = errors.New("user service rejected Premium revoke")
				}
			}
		}
		if applyErr != nil {
			retryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if retryErr := domain.RetryPaymentWebhook(retryCtx, event.EventID, event.ClaimToken, applyErr); retryErr != nil {
				logx.WithContext(ctx).Errorf("payment webhook retry scheduling failed for %s: %v", event, retryErr)
			}
			cancel()
			logx.WithContext(ctx).Errorf("payment webhook %s failed: %v", event, applyErr)
			continue
		}
		completeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if completeErr := domain.CompletePaymentWebhook(completeCtx, event.EventID, event.ClaimToken); completeErr != nil {
			logx.WithContext(ctx).Errorf("payment webhook %s completion failed: %v", event, completeErr)
		}
		cancel()
	}
	return nil
}

func processPaymentReconciliations(ctx context.Context, svcCtx *svc.ServiceContext, batchSize int) error {
	items, err := domain.ClaimPaymentReconciliations(ctx, batchSize)
	if err != nil {
		return err
	}
	for _, item := range items {
		status, statusErr := queryPaymentProviderStatus(ctx, svcCtx, item)
		if statusErr != nil {
			retryPaymentReconciliation(ctx, item, "", statusErr)
			continue
		}
		switch status.Status {
		case "refunded", "revoked":
			if status.Product == "" {
				status.Product = item.Product
			}
			if status.Months == 0 {
				status.Months = item.Months
			}
			if status.Stars == 0 {
				status.Stars = item.Stars
			}
			eventID := strings.TrimSpace(status.EventID)
			if eventID == "" {
				eventID = domain.PaymentRecoveryEventID(item.Provider, item.TransactionID, status.Status)
			}
			event := domain.PaymentWebhook{
				EventID:   eventID,
				EventType: "payment." + status.Status, Provider: item.Provider, UserID: item.UserID,
				RequestKey: item.RequestKey, TransactionID: item.TransactionID, Product: status.Product,
				Months: status.Months, Stars: status.Stars, Reason: status.Reason, OccurredAt: status.OccurredAt,
			}
			// The provider's status endpoint may return a fresh observation
			// timestamp on every read. Keep the internally generated event
			// payload stable because its event ID is deterministic.
			event.OccurredAt = 0
			event.Payload, _ = json.Marshal(event)
			if _, enqueueErr := domain.EnqueuePaymentWebhook(ctx, event, event.Payload); enqueueErr != nil {
				retryPaymentReconciliation(ctx, item, status.Status, enqueueErr)
				continue
			}
			completePaymentReconciliation(ctx, item, status.Status)
		case "settled":
			completePaymentReconciliation(ctx, item, status.Status)
		case "pending", "not_found":
			// A missing provider row is never interpreted as a refund. The
			// exponential retry keeps a grace window for delayed provider writes.
			retryPaymentReconciliation(ctx, item, status.Status, nil)
		default:
			retryPaymentReconciliation(ctx, item, status.Status, fmt.Errorf("unsupported provider status %q", status.Status))
		}
	}
	return nil
}

func retryPaymentReconciliation(ctx context.Context, item domain.PaymentReconciliation, status string, cause error) {
	retryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := domain.RetryPaymentReconciliation(retryCtx, item, status, cause); err != nil {
		logx.WithContext(ctx).Errorf("payment reconciliation retry scheduling failed for %s/%s: %v", item.Provider, item.TransactionID, err)
	}
	cancel()
}

func completePaymentReconciliation(ctx context.Context, item domain.PaymentReconciliation, status string) {
	completeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := domain.CompletePaymentReconciliation(completeCtx, item, status); err != nil {
		logx.WithContext(ctx).Errorf("payment reconciliation completion failed for %s/%s: %v", item.Provider, item.TransactionID, err)
	}
	cancel()
}

func queryPaymentProviderStatus(ctx context.Context, svcCtx *svc.ServiceContext, item domain.PaymentReconciliation) (paymentProviderStatusResponse, error) {
	if svcCtx == nil {
		return paymentProviderStatusResponse{}, errors.New("payment provider is unavailable")
	}
	endpoint := strings.TrimSpace(svcCtx.Config.PaymentProviderEndpoint)
	signingKey := strings.TrimSpace(svcCtx.Config.PaymentProviderSigningKey)
	if endpoint == "" || signingKey == "" {
		return paymentProviderStatusResponse{}, errors.New("payment provider reconciliation is not configured")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !securePaymentProviderURL(parsed, svcCtx.Config.AllowInsecureProviderHTTP) {
		return paymentProviderStatusResponse{}, errors.New("payment provider endpoint is invalid")
	}
	timeout := 10 * time.Second
	if svcCtx.Config.PaymentProviderTimeoutSeconds < 0 || svcCtx.Config.PaymentProviderTimeoutSeconds > 300 {
		return paymentProviderStatusResponse{}, errors.New("payment provider timeout is invalid")
	}
	if svcCtx.Config.PaymentProviderTimeoutSeconds > 0 {
		timeout = time.Duration(svcCtx.Config.PaymentProviderTimeoutSeconds) * time.Second
	}
	requestBody := map[string]any{
		"operation": "get_payment_status", "user_id": item.UserID, "provider": item.Provider,
		"request_key": item.RequestKey, "transaction_id": item.TransactionID, "product": item.Product,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return paymentProviderStatusResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return paymentProviderStatusResponse{}, err
	}
	providercontract.SetRequestHeaders(request, body, svcCtx.Config.PaymentProviderKey,
		providercontract.IdempotencyKey(item.UserID, "get_payment_status", body),
		"X-Teamgram-Payment-Provider-Request-Signature", signingKey)
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return paymentProviderStatusResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return paymentProviderStatusResponse{}, fmt.Errorf("payment provider status returned HTTP %d", response.StatusCode)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return paymentProviderStatusResponse{}, err
	}
	if !providercontract.VerifyResponse(response, responseBody, "X-Teamgram-Payment-Signature", signingKey) {
		return paymentProviderStatusResponse{}, errors.New("payment provider status response signature is invalid")
	}
	var status paymentProviderStatusResponse
	if err = json.Unmarshal(responseBody, &status); err != nil {
		return paymentProviderStatusResponse{}, err
	}
	status.Status = strings.ToLower(strings.TrimSpace(status.Status))
	status.Provider = strings.TrimSpace(status.Provider)
	status.Product = strings.TrimSpace(status.Product)
	if !status.Verified || status.UserID != item.UserID || (status.Provider != "" && status.Provider != item.Provider) ||
		(status.RequestKey != "" && status.RequestKey != item.RequestKey) ||
		status.TransactionID != item.TransactionID || (status.Product != "" && status.Product != item.Product) ||
		(status.Months != 0 && status.Months != item.Months) || (status.Stars != 0 && status.Stars != item.Stars) {
		return paymentProviderStatusResponse{}, errors.New("payment provider status identity mismatch")
	}
	if status.Status == "" {
		return paymentProviderStatusResponse{}, errors.New("payment provider status is empty")
	}
	status.Provider = item.Provider
	if status.Product == "" {
		status.Product = item.Product
	}
	return status, nil
}
