// Package paymentrecovery exposes the small webhook ingress surface shared by
// the HTTP interface and the APIFull recovery worker. The implementation stays
// in APIFull's internal PostgreSQL domain so public MTProto packages do not
// acquire a second payment state machine.
package paymentrecovery

import (
	"context"

	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

type WebhookEvent = domain.PaymentWebhook
type Sub2APIStarsUsageEvent = domain.Sub2APIStarsUsageEvent

var (
	ErrWebhookConflict      = domain.ErrPaymentWebhookConflict
	ErrInvalidWebhook       = domain.ErrPaymentRecoveryInvalid
	ErrStarsUsageConflict   = domain.ErrStarsUsageConflict
	ErrStarsBalanceExceeded = domain.ErrStarsBalanceExceeded
)

func OpenPostgresReadOnly(dsn string) error {
	return domain.OpenPostgresReadOnly(dsn)
}

func ClosePostgres() error {
	return domain.Close()
}

func EnqueueWebhook(ctx context.Context, event WebhookEvent, payload []byte) (bool, error) {
	return domain.EnqueuePaymentWebhook(ctx, event, payload)
}

func ApplySub2APIStarsUsage(ctx context.Context, event Sub2APIStarsUsageEvent) (int64, error) {
	return domain.ApplySub2APIStarsUsage(ctx, event)
}
