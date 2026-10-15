package dao

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	deliveryPending = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_delivery_pending", Help: "Durable pending intents, including leased and delayed retries.",
	}, []string{"queue"})
	deliveryOldest = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_delivery_oldest_pending_seconds", Help: "Age of the oldest pending intent; zero when the queue is empty.",
	}, []string{"queue"})
	deliveryExpired = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_delivery_expired_leases", Help: "Pending intents whose worker lease has expired.",
	}, []string{"queue"})
	deliveryAttempts = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_delivery_max_attempts", Help: "Highest attempt count among pending intents.",
	}, []string{"queue"})
	deliveryDeadLetterPending = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_delivery_dead_letter_pending", Help: "Durable dead-lettered intents retained for diagnosis or replay.",
	}, []string{"queue"})
	deliveryDeadLetters = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_delivery_dead_letter_total", Help: "Intents moved to terminal dead-letter state.",
	}, []string{"queue"})
	deliveryRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_delivery_retries_total", Help: "Retry releases for durable delivery intents.",
	}, []string{"queue"})
	deliverySampleTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_delivery_backlog_sample_timestamp_seconds", Help: "Time of the last successful database backlog sample.",
	})
	deliverySampleErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "teamgram_delivery_backlog_sample_errors_total", Help: "Failed database backlog samples.",
	})
	deliveryPublications = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "teamgram_delivery_publications_total", Help: "Publication attempts by queue and outcome; an ACK is not client receipt.",
	}, []string{"queue", "outcome"})
	deliveryPublicationInFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_delivery_publication_inflight", Help: "Currently running independent delivery publications by queue.",
	}, []string{"queue"})
	deliveryClaimWait = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "teamgram_delivery_claim_wait_seconds", Help: "Time from durable enqueue until an outbox intent is claimed.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 15, 60, 300},
	}, []string{"queue"})
	deliveryPublicationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "teamgram_delivery_publication_seconds", Help: "Time spent waiting for the delivery publication result.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 15, 30},
	}, []string{"queue"})
	deliveryCompletion = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "teamgram_delivery_completion_seconds", Help: "Time from durable enqueue to consumer or broker acknowledgement.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"queue", "stage"})
	deliveryRecovery = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "teamgram_delivery_recovery_seconds", Help: "Time from enqueue until a retrying intent reaches an acknowledgement or terminal state.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 15, 30, 60, 120, 300, 600, 1800},
	}, []string{"queue"})
	deliverySampleDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "teamgram_delivery_backlog_sample_seconds", Help: "Duration of a durable delivery backlog sample query.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	})
	deliveryPool = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_postgres_pool_connections", Help: "PostgreSQL pool connections by state.",
	}, []string{"state"})
	deliveryPoolMax = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_postgres_pool_max_connections", Help: "Configured maximum connections for the message service PostgreSQL pool.",
	})
	deliveryPoolAcquireWait = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_postgres_pool_acquire_wait_seconds_total", Help: "Cumulative time spent waiting to acquire PostgreSQL pool connections.",
	})
)

type deliveryBacklog struct {
	queue                               string
	pending, expired, maxAttempts, dead int64
	oldest                              float64
}

func (d *Dao) readDeliveryBacklog(ctx context.Context) ([]deliveryBacklog, error) {
	rows, err := d.Pool.Query(ctx, `SELECT 'inbox', count(*) FILTER (WHERE state=0),
		COALESCE(extract(epoch FROM now()-min(created_at) FILTER (WHERE state=0)),0)::double precision,
		count(*) FILTER (WHERE state=0 AND lease_until<=now()), COALESCE(max(attempts) FILTER (WHERE state=0),0), count(*) FILTER (WHERE state=2)
		FROM msg_inbox_delivery_outbox WHERE state IN (0,2)
		UNION ALL
		SELECT 'state', count(*) FILTER (WHERE state=0),
		COALESCE(extract(epoch FROM now()-min(created_at) FILTER (WHERE state=0)),0)::double precision,
		count(*) FILTER (WHERE state=0 AND lease_until<=now()), COALESCE(max(attempts) FILTER (WHERE state=0),0), count(*) FILTER (WHERE state=2)
		FROM msg_state_delivery_outbox WHERE state IN (0,2)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []deliveryBacklog
	for rows.Next() {
		var sample deliveryBacklog
		if err := rows.Scan(&sample.queue, &sample.pending, &sample.oldest, &sample.expired, &sample.maxAttempts, &sample.dead); err != nil {
			return nil, err
		}
		result = append(result, sample)
	}
	return result, rows.Err()
}

// Sampling runs independently of publication so a stalled broker cannot make
// backlog gauges appear healthy. A failed sample retains its old timestamp.
func (d *Dao) startDeliveryMetrics(ctx context.Context) func() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			sampleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			started := time.Now()
			samples, err := d.readDeliveryBacklog(sampleCtx)
			cancel()
			deliverySampleDuration.Observe(max(time.Since(started).Seconds(), 0))
			if err == nil {
				for _, sample := range samples {
					deliveryPending.WithLabelValues(sample.queue).Set(float64(sample.pending))
					deliveryOldest.WithLabelValues(sample.queue).Set(max(sample.oldest, 0))
					deliveryExpired.WithLabelValues(sample.queue).Set(float64(sample.expired))
					deliveryAttempts.WithLabelValues(sample.queue).Set(float64(sample.maxAttempts))
					deliveryDeadLetterPending.WithLabelValues(sample.queue).Set(float64(sample.dead))
				}
				deliverySampleTime.Set(float64(time.Now().Unix()))
				if d.Postgres != nil && d.Postgres.Pool != nil {
					stats := d.Postgres.Pool.Stat()
					deliveryPool.WithLabelValues("acquired").Set(float64(stats.AcquiredConns()))
					deliveryPool.WithLabelValues("idle").Set(float64(stats.IdleConns()))
					deliveryPool.WithLabelValues("total").Set(float64(stats.TotalConns()))
					deliveryPool.WithLabelValues("constructing").Set(float64(stats.ConstructingConns()))
					deliveryPoolMax.Set(float64(stats.MaxConns()))
					deliveryPoolAcquireWait.Set(stats.AcquireDuration().Seconds())
				}
			} else if ctx.Err() == nil {
				deliverySampleErrors.Inc()
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { <-done }
}

func observeDeliveryPublication(queue string, err error) {
	outcome := "ok"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case err != nil:
		outcome = "error"
	}
	deliveryPublications.WithLabelValues(queue, outcome).Inc()
}

func observeDeliveryClaim(queue string, createdAt time.Time) {
	if createdAt.IsZero() {
		return
	}
	deliveryClaimWait.WithLabelValues(queue).Observe(max(time.Since(createdAt).Seconds(), 0))
}

func observeDeliveryPublicationDuration(queue string, started time.Time) {
	deliveryPublicationDuration.WithLabelValues(queue).Observe(max(time.Since(started).Seconds(), 0))
}
