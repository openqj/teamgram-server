package dao

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

const (
	deliveryCompletionBatchSize   = 64
	deliveryCompletionBatchWindow = 2 * time.Millisecond
	deliveryCompletionTimeout     = 30 * time.Second
)

var errDeliveryCompletionBatchClosed = errors.New("sync: delivery completion batch is closed")

type deliveryCompletionRequest struct {
	userID  int64
	receipt DeliveryReceipt
	result  chan error
}

type deliveryCompletionBatcher struct {
	pool  *pgxpool.Pool
	queue chan deliveryCompletionRequest
	done  chan struct{}

	mu     sync.RWMutex
	closed bool
}

func newDeliveryCompletionBatcher(pool *pgxpool.Pool) *deliveryCompletionBatcher {
	batcher := &deliveryCompletionBatcher{
		pool:  pool,
		queue: make(chan deliveryCompletionRequest, deliveryCompletionBatchSize*4),
		done:  make(chan struct{}),
	}
	go batcher.run()
	return batcher
}

func (b *deliveryCompletionBatcher) Submit(ctx context.Context, userID int64, receipt DeliveryReceipt) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := deliveryCompletionRequest{
		userID:  userID,
		receipt: receipt,
		result:  make(chan error, 1),
	}

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return errDeliveryCompletionBatchClosed
	}
	select {
	case b.queue <- request:
		b.mu.RUnlock()
	case <-ctx.Done():
		b.mu.RUnlock()
		return ctx.Err()
	}

	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *deliveryCompletionBatcher) run() {
	defer close(b.done)
	for first := range b.queue {
		batch := make([]deliveryCompletionRequest, 0, deliveryCompletionBatchSize)
		batch = append(batch, first)
		timer := time.NewTimer(deliveryCompletionBatchWindow)
	collect:
		for len(batch) < deliveryCompletionBatchSize {
			select {
			case request, ok := <-b.queue:
				if !ok {
					stopTimer(timer)
					b.flush(batch)
					return
				}
				batch = append(batch, request)
			case <-timer.C:
				break collect
			}
		}
		stopTimer(timer)
		b.flush(batch)
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (b *deliveryCompletionBatcher) flush(batch []deliveryCompletionRequest) {
	if len(batch) == 0 {
		return
	}
	// Stable ordering prevents overlapping batches from taking receipt row
	// locks in different orders when one user has multiple updates.
	sort.SliceStable(batch, func(i, j int) bool {
		left, right := batch[i], batch[j]
		if left.receipt.ConsumerGroup != right.receipt.ConsumerGroup {
			return left.receipt.ConsumerGroup < right.receipt.ConsumerGroup
		}
		if left.receipt.Topic != right.receipt.Topic {
			return left.receipt.Topic < right.receipt.Topic
		}
		if left.receipt.Partition != right.receipt.Partition {
			return left.receipt.Partition < right.receipt.Partition
		}
		if left.receipt.Offset != right.receipt.Offset {
			return left.receipt.Offset < right.receipt.Offset
		}
		return left.userID < right.userID
	})

	ctx, cancel := context.WithTimeout(context.Background(), deliveryCompletionTimeout)
	err := postgres.WithTx(ctx, b.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var statements pgx.Batch
		for _, request := range batch {
			statements.Queue(`UPDATE sync_delivery_receipts SET delivered=TRUE,delivered_at=clock_timestamp()
 WHERE consumer_group=$1 AND topic=$2 AND partition_id=$3 AND message_offset=$4
 AND user_id=$5 AND request_hash=$6 AND delivery_data IS NOT NULL`,
				request.receipt.ConsumerGroup, request.receipt.Topic, request.receipt.Partition,
				request.receipt.Offset, request.userID, request.receipt.RequestHash)
		}
		results := tx.SendBatch(ctx, &statements)
		for range batch {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		if err := results.Close(); err != nil {
			return err
		}
		return nil
	})
	cancel()
	for _, request := range batch {
		request.result <- err
	}
}

func (b *deliveryCompletionBatcher) Close() {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.queue)
	}
	b.mu.Unlock()
	<-b.done
}
