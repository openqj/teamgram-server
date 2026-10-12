package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RunBotPaymentReconciler delivers the two bot-facing payment queries from
// the PostgreSQL outbox. The query and its delivery intent are committed in
// one transaction, so a process restart can only delay delivery, never lose
// updateBotShippingQuery or updateBotPrecheckoutQuery.
func RunBotPaymentReconciler(ctx context.Context, svcCtx *svc.ServiceContext) {
	if svcCtx == nil || svcCtx.Dao == nil || svcCtx.Dao.SyncClient == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var unavailable bool
	for {
		items, err := persist.ClaimBotPaymentOutbox(ctx, 50)
		if err != nil {
			if !unavailable {
				logx.WithContext(ctx).Errorf("bot payment reconciler unavailable: %v", err)
				unavailable = true
			}
		} else {
			if unavailable {
				logx.WithContext(ctx).Info("bot payment reconciler available")
				unavailable = false
			}
			for _, item := range items {
				if err = deliverBotPaymentOutbox(ctx, svcCtx, item); err != nil {
					logx.WithContext(ctx).Errorf("bot payment delivery %d failed: %v", item.ID, err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type botPaymentOutboxPayload struct {
	QueryID      int64           `json:"query_id"`
	BotUserID    int64           `json:"bot_user_id"`
	BuyerUserID  int64           `json:"buyer_user_id"`
	QueryPayload json.RawMessage `json:"query_payload"`
	Currency     string          `json:"currency"`
	Amount       int64           `json:"amount"`
}

func botPaymentOutboxContext(item persist.BotPaymentOutbox) (botPaymentContext, error) {
	var payload botPaymentOutboxPayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return botPaymentContext{}, err
	}
	if payload.QueryID != 0 && payload.QueryID != item.QueryID ||
		payload.BotUserID != 0 && payload.BotUserID != item.BotUserID ||
		payload.BuyerUserID != 0 && payload.BuyerUserID != item.BuyerUserID {
		return botPaymentContext{}, errors.New("bot payment outbox identity mismatch")
	}
	if len(payload.QueryPayload) == 0 {
		query, found, err := persist.LoadBotPaymentQuery(item.QueryID)
		if err != nil {
			return botPaymentContext{}, err
		}
		if !found {
			return botPaymentContext{}, persist.ErrBotPaymentQueryNotFound
		}
		payload.QueryPayload = query.Payload
	}
	value, err := decodeBotPaymentContext(payload.QueryPayload)
	if err != nil {
		return botPaymentContext{}, err
	}
	if value.QueryID != item.QueryID || value.BotUserID != item.BotUserID || value.BuyerUserID != item.BuyerUserID {
		return botPaymentContext{}, errors.New("bot payment context identity mismatch")
	}
	if strings.TrimSpace(payload.Currency) != "" && value.Currency != strings.TrimSpace(payload.Currency) {
		return botPaymentContext{}, errors.New("bot payment context currency mismatch")
	}
	if payload.Amount > 0 && value.Amount != payload.Amount {
		return botPaymentContext{}, errors.New("bot payment context amount mismatch")
	}
	return value, nil
}

func botPaymentUpdate(item persist.BotPaymentOutbox) (*mtproto.Updates, error) {
	value, err := botPaymentOutboxContext(item)
	if err != nil {
		return nil, err
	}
	switch item.EventType {
	case persist.BotPaymentOutboxEventShippingQuery:
		info := value.Info
		if info == nil || info.GetShippingAddress() == nil {
			return nil, mtproto.ErrInvoicePayloadInvalid
		}
		address := info.GetShippingAddress()
		if address.GetPredicateName() == "" {
			mtproto.MakeTLPostAddress(address).To_PostAddress()
		}
		update := mtproto.MakeTLUpdateBotShippingQuery(&mtproto.Update{
			QueryId_INT64:   item.QueryID,
			UserId:          item.BuyerUserID,
			Payload_BYTES:   append([]byte(nil), value.InvoicePayload...),
			ShippingAddress: address,
		}).To_Update()
		return mtproto.MakeUpdatesByUpdates(update), nil
	case persist.BotPaymentOutboxEventPrecheckoutQuery:
		updateData := &mtproto.Update{
			QueryId_INT64: item.QueryID,
			UserId:        item.BuyerUserID,
			Payload_BYTES: append([]byte(nil), value.InvoicePayload...),
			Currency:      value.Currency,
			TotalAmount:   value.Amount,
		}
		if value.Info == nil {
			value.Info = &mtproto.PaymentRequestedInfo{}
		}
		if value.Info.GetPredicateName() == "" {
			mtproto.MakeTLPaymentRequestedInfo(value.Info).To_PaymentRequestedInfo()
		}
		updateData.Info_FLAGPAYMENTREQUESTEDINFO = value.Info
		if shippingOptionID := strings.TrimSpace(value.ShippingOptionID); shippingOptionID != "" {
			updateData.ShippingOptionId = wrapperspb.String(shippingOptionID)
		}
		update := mtproto.MakeTLUpdateBotPrecheckoutQuery(updateData).To_Update()
		return mtproto.MakeUpdatesByUpdates(update), nil
	default:
		return nil, nil
	}
}

func deliverBotPaymentOutbox(ctx context.Context, svcCtx *svc.ServiceContext, item persist.BotPaymentOutbox) error {
	// Callback answer intents are durable evidence for the payment waiter but
	// do not produce a second bot update. Mark them complete when claimed.
	if item.EventType != persist.BotPaymentOutboxEventShippingQuery && item.EventType != persist.BotPaymentOutboxEventPrecheckoutQuery {
		return persist.CompleteBotPaymentOutboxWithToken(ctx, item.ID, item.ClaimToken)
	}
	updates, err := botPaymentUpdate(item)
	if err != nil {
		_ = persist.RetryBotPaymentOutboxWithToken(ctx, item.ID, item.ClaimToken, err)
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	result, pushErr := svcCtx.Dao.SyncClient.SyncPushBotUpdates(callCtx, &syncpb.TLSyncPushBotUpdates{
		UserId: item.BotUserID, Updates: updates,
	})
	cancel()
	if pushErr == nil && result == nil {
		pushErr = errors.New("sync bot update returned no result")
	}
	if pushErr != nil {
		if retryErr := persist.RetryBotPaymentOutboxWithToken(ctx, item.ID, item.ClaimToken, pushErr); retryErr != nil {
			return errors.Join(pushErr, retryErr)
		}
		return pushErr
	}
	return persist.CompleteBotPaymentOutboxWithToken(ctx, item.ID, item.ClaimToken)
}
