package dao

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	syncDeliveryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "teamgram_sync_delivery_seconds",
		Help:    "Duration of durable sync delivery stages, including consumer handling and gateway push.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"stage"})
	syncDeliveryResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_sync_delivery_total",
		Help: "Completed sync delivery stages by outcome.",
	}, []string{"stage", "outcome"})
)

// ObserveDeliveryStage records a bounded-label stage from the Sync Kafka
// consumer through gateway acknowledgement. It intentionally excludes user,
// session, gateway and message identifiers to keep cardinality stable.
func ObserveDeliveryStage(stage string, started time.Time, err error) {
	if stage == "" || started.IsZero() {
		return
	}
	outcome := "ok"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case err != nil:
		outcome = "error"
	}
	syncDeliveryDuration.WithLabelValues(stage).Observe(max(time.Since(started).Seconds(), 0))
	syncDeliveryResults.WithLabelValues(stage, outcome).Inc()
}
