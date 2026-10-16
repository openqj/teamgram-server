package domain

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var channelPersistenceDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "teamgram_apifull_channel_persistence_seconds",
	Help:    "Duration of APIFull channel persistence stages.",
	Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
}, []string{"stage"})

func observeChannelPersistence(stage string, started time.Time) {
	if stage == "" || started.IsZero() {
		return
	}
	channelPersistenceDuration.WithLabelValues(stage).Observe(time.Since(started).Seconds())
}
