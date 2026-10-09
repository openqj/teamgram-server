package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPostgresChannelDeliveryRetriesAndConcurrentCompletion(t *testing.T) {
	requireMigrationDB(t)
	ctx := context.Background()
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID, Creator: ownerID, Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := DeleteChannel(ownerID, channelID); err != nil {
			t.Errorf("clean delivery fixture: %v", err)
		}
	})
	for i := int64(2); i <= 6; i++ {
		if err := JoinChannel(channelID, channelID+i); err != nil {
			t.Fatal(err)
		}
	}
	row, err := InsertChannelMessageWithContentAndRandomIDForDelivery(channelID, ownerID, time.Now().Unix(), "delivery", 0, 0, channelID, ChannelMessageContent{}, "delivery-fixture", 77)
	if err != nil {
		t.Fatal(err)
	}
	key := ChannelDeliveryKey{ChannelID: channelID, PTSFrom: row.Pts, PTSTo: row.Pts}
	recipients, err := ListChannelDeliveryRecipients(ctx, key, false)
	if err != nil || len(recipients) != 6 {
		t.Fatalf("recipient snapshot: count=%d err=%v", len(recipients), err)
	}
	if err = RetryChannelDeliveryRecipient(ctx, key, ownerID, errors.New("fixture delivery failed")); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var due int64
	var detail string
	if err = db.QueryRow(`SELECT r.attempts, r.next_attempt_at, r.last_error
		FROM apifull_channel_delivery_recipient r JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id
		WHERE o.channel_id=$1 AND r.user_id=$2`, channelID, ownerID).Scan(&attempts, &due, &detail); err != nil || attempts != 1 || due <= time.Now().Unix() || detail != "fixture delivery failed" {
		t.Fatalf("retry state: attempts=%d due=%d detail=%q err=%v", attempts, due, detail, err)
	}
	ready := make(chan struct{})
	results := make(chan error, len(recipients))
	for _, recipient := range recipients {
		go func(userID int64) {
			<-ready
			results <- CompleteChannelDeliveryRecipient(ctx, key, userID, "delivered")
		}(recipient.UserID)
	}
	close(ready)
	for range recipients {
		if err = <-results; err != nil {
			t.Fatal(err)
		}
	}
	payload, found, err := LoadChannelDelivery(ctx, key)
	if err != nil || !found || !payload.Complete || payload.ExcludeAuthKeyID != 0 {
		t.Fatalf("completed delivery: payload=%+v found=%v err=%v", payload, found, err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id WHERE o.channel_id=$1`, channelID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("completion left recipient snapshot: count=%d err=%v", count, err)
	}
	if err = RetryChannelDeliveryRecipient(ctx, key, ownerID, fmt.Errorf("late retry")); err != nil {
		t.Fatal(err)
	}
	if err = CompleteChannelDeliveryRecipient(ctx, key, ownerID, "delivered"); err != nil {
		t.Fatal(err)
	}

	replayed, err := InsertChannelMessageWithContentAndRandomIDForDelivery(channelID, ownerID, row.Date, "delivery", 0, 0, channelID, ChannelMessageContent{}, "delivery-fixture", 77)
	if err != nil || replayed.MessageID != row.MessageID || replayed.Pts != row.Pts {
		t.Fatalf("idempotent message replay: row=%+v err=%v", replayed, err)
	}
	recipients, err = ListChannelDeliveryRecipients(ctx, key, false)
	if err != nil || len(recipients) != 0 {
		t.Fatalf("completed replay recreated recipients: count=%d err=%v", len(recipients), err)
	}
}
