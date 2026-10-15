package channelview

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	channelDeliveryStageDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "teamgram_apifull_channel_delivery_stage_seconds",
		Help:    "Duration of APIFull channel delivery stages.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	}, []string{"stage"})
	channelDeliveryStageResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_apifull_channel_delivery_stage_total",
		Help: "APIFull channel delivery stage results.",
	}, []string{"stage", "result"})
)

func observeChannelDeliveryStage(stage string, started time.Time, err error) {
	if stage == "" || started.IsZero() {
		return
	}
	result := "ok"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result = "timeout"
	case errors.Is(err, context.Canceled):
		result = "canceled"
	case err != nil:
		result = "error"
	}
	channelDeliveryStageDuration.WithLabelValues(stage).Observe(max(time.Since(started).Seconds(), 0))
	channelDeliveryStageResults.WithLabelValues(stage, result).Inc()
}
