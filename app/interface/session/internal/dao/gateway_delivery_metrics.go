package dao

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	sessionGatewayDeliveryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "teamgram_session_gateway_delivery_seconds",
		Help:    "Round-trip time from the session service to a gateway delivery acknowledgement.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	}, []string{"transport"})
	sessionGatewayDeliveryResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_session_gateway_delivery_total",
		Help: "Session-to-gateway delivery attempts by transport and outcome.",
	}, []string{"transport", "outcome"})
)

func observeSessionGatewayDelivery(transport string, started time.Time, ok bool, err error) {
	if started.IsZero() || transport == "" {
		return
	}
	outcome := "ok"
	switch {
	case ok && err == nil:
		outcome = "ok"
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case err != nil:
		outcome = "error"
	default:
		outcome = "rejected"
	}
	sessionGatewayDeliveryDuration.WithLabelValues(transport).Observe(max(time.Since(started).Seconds(), 0))
	sessionGatewayDeliveryResults.WithLabelValues(transport, outcome).Inc()
}
