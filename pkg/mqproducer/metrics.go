package mqproducer

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	producerCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_kafka_producer_calls_total", Help: "Kafka publication calls by caller result.",
	}, []string{"topic", "result"})
	producerCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "teamgram_kafka_producer_call_seconds", Help: "Kafka publication wait, including outstanding capacity and acknowledgement.",
		Buckets: []float64{0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
	}, []string{"topic", "result"})
	producerOutstanding = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_kafka_producer_outstanding", Help: "Reserved or accepted Kafka records awaiting a final broker result.",
	}, []string{"topic"})
	producerDeliveryResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_kafka_producer_delivery_results_total", Help: "Final Kafka delivery results, including acknowledgements after caller timeout.",
	}, []string{"topic", "result"})
	producerShutdownTimeouts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_kafka_producer_shutdown_timeouts_total", Help: "Kafka producer shutdown waits ended before result draining completed.",
	}, []string{"topic"})
)

func callResult(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, ErrClosed):
		return "closed"
	default:
		return "error"
	}
}
