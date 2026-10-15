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

var (
	ErrWebhookConflict = domain.ErrPaymentWebhookConflict
	ErrInvalidWebhook  = domain.ErrPaymentRecoveryInvalid
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
