package dao

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"

	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/testutil"
)

func TestDeliveryCompletionBatchCommitsConcurrentRequests(t *testing.T) {
	pool := testutil.PostgresPool(t)
	batcher := newDeliveryCompletionBatcher(pool)
	t.Cleanup(batcher.Close)

	const requests = 96
	for offset := int64(0); offset < requests; offset++ {
		hash := sha256.Sum256([]byte(fmt.Sprint(offset)))
		if _, err := pool.Exec(context.Background(), `INSERT INTO sync_delivery_receipts
 (consumer_group,topic,partition_id,message_offset,user_id,request_hash,delivery_data)
 VALUES ($1,$2,0,$3,$4,$5,'{}'::jsonb)`, "batch-test", "Sync-T", offset, offset+1, hash[:]); err != nil {
			t.Fatal(err)
		}
	}

	errs := make(chan error, requests)
	var wg sync.WaitGroup
	for offset := int64(0); offset < requests; offset++ {
		offset := offset
		hash := sha256.Sum256([]byte(fmt.Sprint(offset)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- batcher.Submit(context.Background(), offset+1, DeliveryReceipt{
				ConsumerGroup: "batch-test", Topic: "Sync-T", Partition: 0,
				Offset: offset, RequestHash: hash[:],
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	var completed int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sync_delivery_receipts
 WHERE consumer_group='batch-test' AND delivered`).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != requests {
		t.Fatalf("completed receipts=%d, want %d", completed, requests)
	}
}
