package dao

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	syncPostgresPoolConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_connections",
		Help: "Current Sync PostgreSQL pool connections by state.",
	}, []string{"state"})
	syncPostgresPoolMaxConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_max_connections",
		Help: "Configured maximum connections for the Sync PostgreSQL pool.",
	})
	syncPostgresPoolAcquireCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_acquire_count",
		Help: "Cumulative successful PostgreSQL pool acquires reported by pgxpool.",
	})
	syncPostgresPoolAcquireDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_acquire_duration_seconds",
		Help: "Cumulative duration of successful PostgreSQL pool acquires reported by pgxpool.",
	})
	syncPostgresPoolEmptyAcquireCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_empty_acquire_count",
		Help: "Cumulative successful acquires that waited because the Sync PostgreSQL pool had no available connection.",
	})
	syncPostgresPoolEmptyAcquireWait = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_empty_acquire_wait_seconds",
		Help: "Cumulative time successful acquires waited for an available Sync PostgreSQL connection.",
	})
	syncPostgresPoolCanceledAcquireCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "teamgram_sync_postgres_pool_canceled_acquire_count",
		Help: "Cumulative PostgreSQL pool acquires canceled by context before obtaining a connection.",
	})
)

const syncPostgresPoolMetricsInterval = 5 * time.Second

func observeSyncPostgresPool(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	stats := pool.Stat()
	syncPostgresPoolConnections.WithLabelValues("acquired").Set(float64(stats.AcquiredConns()))
	syncPostgresPoolConnections.WithLabelValues("idle").Set(float64(stats.IdleConns()))
	syncPostgresPoolConnections.WithLabelValues("total").Set(float64(stats.TotalConns()))
	syncPostgresPoolConnections.WithLabelValues("constructing").Set(float64(stats.ConstructingConns()))
	syncPostgresPoolMaxConnections.Set(float64(stats.MaxConns()))
	syncPostgresPoolAcquireCount.Set(float64(stats.AcquireCount()))
	syncPostgresPoolAcquireDuration.Set(stats.AcquireDuration().Seconds())
	syncPostgresPoolEmptyAcquireCount.Set(float64(stats.EmptyAcquireCount()))
	syncPostgresPoolEmptyAcquireWait.Set(stats.EmptyAcquireWaitTime().Seconds())
	syncPostgresPoolCanceledAcquireCount.Set(float64(stats.CanceledAcquireCount()))
}

// startPoolMetrics samples pgxpool's cumulative counters and current
// connection state independently of message delivery. A stalled database
// query must remain visible even when Kafka handling is backlogged.
func (d *Postgres) startPoolMetrics(parent context.Context) func() {
	if d == nil || d.Pool == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		observeSyncPostgresPool(d.Pool)
		ticker := time.NewTicker(syncPostgresPoolMetricsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				observeSyncPostgresPool(d.Pool)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
