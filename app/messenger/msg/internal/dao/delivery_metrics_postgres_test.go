package dao

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestInboxDeliveryPostgresBacklogIncludesDelayedAndExpiredWork(t *testing.T) {
	d, _ := newMessagePostgresFixture(t)
	ctx := context.Background()
	for id := int64(1); id <= 3; id++ {
		enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, id, 202))
	}
	_, err := d.Pool.Exec(ctx, `UPDATE msg_inbox_delivery_outbox SET
		created_at=now()-interval '90 seconds', available_at=now()+interval '1 hour',
		attempts=4, lease_until=CASE WHEN dialog_message_id=1 THEN now()-interval '1 second' ELSE NULL END;
		UPDATE msg_inbox_delivery_outbox SET state=1,attempts=100 WHERE dialog_message_id=3;
		INSERT INTO msg_state_delivery_outbox(user_id,method,payload) VALUES (101,'fixture','{}')`)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := d.readDeliveryBacklog(ctx)
	if err != nil || len(samples) != 2 {
		t.Fatalf("backlog samples=%v error=%v", samples, err)
	}
	for _, sample := range samples {
		switch sample.queue {
		case "inbox":
			if sample.pending != 2 || sample.expired != 1 || sample.maxAttempts != 4 || sample.oldest < 89 {
				t.Fatalf("delayed/expired/completed work counted incorrectly: %+v", sample)
			}
		case "state":
			if sample.pending != 1 || sample.expired != 0 || sample.maxAttempts != 0 {
				t.Fatalf("state backlog: %+v", sample)
			}
		default:
			t.Fatalf("unexpected queue: %q", sample.queue)
		}
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE msg_inbox_delivery_outbox SET state=1;
		UPDATE msg_state_delivery_outbox SET state=1`); err != nil {
		t.Fatal(err)
	}
	samples, err = d.readDeliveryBacklog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if sample.pending != 0 || sample.oldest != 0 || sample.expired != 0 || sample.maxAttempts != 0 {
			t.Fatalf("drained backlog retained stale values: %+v", sample)
		}
	}
}

func inboxCompletionCount(t *testing.T) uint64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "teamgram_delivery_completion_seconds" {
			continue
		}
		for _, sample := range family.Metric {
			queue, stage := "", ""
			for _, label := range sample.Label {
				if label.GetName() == "queue" {
					queue = label.GetValue()
				}
				if label.GetName() == "stage" {
					stage = label.GetValue()
				}
			}
			if queue == "inbox" && stage == "consumer_ack" {
				return sample.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func TestInboxDeliveryPostgresCompletionMetricCountsOnlyCommittedAck(t *testing.T) {
	d := newInboxDeliveryPostgresFixture(t)
	ctx := context.Background()
	enqueueInboxDelivery(t, d, inboxDeliveryRequest(101, 1, 202))
	before := inboxCompletionCount(t)
	for i := 0; i < 2; i++ {
		if err := d.CompleteInboxDelivery(ctx, 101, 1, 202); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.CompleteInboxDelivery(ctx, 101, 99, 202); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := d.CompleteInboxDelivery(cancelled, 101, 1, 202); err == nil {
		t.Fatal("canceled acknowledgement succeeded")
	}
	if got := inboxCompletionCount(t) - before; got != 1 {
		t.Fatalf("completion observations=%d, want one committed acknowledgement", got)
	}
}
