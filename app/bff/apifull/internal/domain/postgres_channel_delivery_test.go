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

func TestPostgresChannelDeliveryBatchResultsAndAuthKeyExclusion(t *testing.T) {
	requireMigrationDB(t)
	ctx := context.Background()
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	cleanupMigrationChannel(t, channelID)
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID, Creator: ownerID, Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{channelID + 2, channelID + 3} {
		if err := JoinChannel(channelID, userID); err != nil {
			t.Fatal(err)
		}
	}
	row, err := InsertChannelMessageWithReplyAndRandomIDForDelivery(channelID, ownerID, time.Now().Unix(), "batch delivery", 0, 0, channelID, 987654)
	if err != nil {
		t.Fatal(err)
	}
	key := ChannelDeliveryKey{ChannelID: channelID, PTSFrom: row.Pts, PTSTo: row.Pts}
	recipients, err := ListChannelDeliveryRecipients(ctx, key, false)
	if err != nil || len(recipients) != 3 {
		t.Fatalf("recipient snapshot: count=%d err=%v", len(recipients), err)
	}
	payload, found, err := LoadChannelDelivery(ctx, key)
	if err != nil || !found || payload.ExcludeAuthKeyID != 987654 {
		t.Fatalf("delivery auth-key exclusion: payload=%+v found=%v err=%v", payload, found, err)
	}
	results := []ChannelDeliveryRecipientResult{
		{UserID: recipients[0].UserID, State: ChannelDeliveryRecipientDelivered},
		{UserID: recipients[1].UserID, State: ChannelDeliveryRecipientSkipped},
		{UserID: recipients[2].UserID, State: ChannelDeliveryRecipientRetry, Err: errors.New("fixture temporary failure")},
	}
	if err = RecordChannelDeliveryRecipientResults(ctx, key, results); err != nil {
		t.Fatal(err)
	}
	var delivered, skipped string
	if err = db.QueryRow(`SELECT state FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id WHERE o.channel_id=$1 AND r.user_id=$2`,
		channelID, recipients[0].UserID).Scan(&delivered); err != nil || delivered != ChannelDeliveryRecipientDelivered {
		t.Fatalf("delivered recipient state=%q err=%v", delivered, err)
	}
	if err = db.QueryRow(`SELECT state FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id WHERE o.channel_id=$1 AND r.user_id=$2`,
		channelID, recipients[1].UserID).Scan(&skipped); err != nil || skipped != ChannelDeliveryRecipientSkipped {
		t.Fatalf("skipped recipient state=%q err=%v", skipped, err)
	}
	var attempts int
	var nextAttempt int64
	var detail string
	if err = db.QueryRow(`SELECT attempts,next_attempt_at,last_error FROM apifull_channel_delivery_recipient r
		JOIN apifull_channel_delivery_outbox o ON o.id=r.delivery_id WHERE o.channel_id=$1 AND r.user_id=$2`,
		channelID, recipients[2].UserID).Scan(&attempts, &nextAttempt, &detail); err != nil || attempts != 1 || nextAttempt <= time.Now().Unix() || detail != "fixture temporary failure" {
		t.Fatalf("retry recipient state: attempts=%d next=%d detail=%q err=%v", attempts, nextAttempt, detail, err)
	}
	pending, err := ListChannelDeliveryRecipients(ctx, key, false)
	if err != nil || len(pending) != 1 || pending[0].UserID != recipients[2].UserID {
		t.Fatalf("pending recipient state: recipients=%+v err=%v", pending, err)
	}
	if err = RecordChannelDeliveryRecipientResults(ctx, key, []ChannelDeliveryRecipientResult{{
		UserID: recipients[2].UserID, State: ChannelDeliveryRecipientDelivered,
	}}); err != nil {
		t.Fatal(err)
	}
	payload, found, err = LoadChannelDelivery(ctx, key)
	if err != nil || !found || !payload.Complete || payload.ExcludeAuthKeyID != 0 {
		t.Fatalf("completed batch delivery: payload=%+v found=%v err=%v", payload, found, err)
	}
}

func TestPostgresChannelDeliveryRecipientBatchVisibility(t *testing.T) {
	requireMigrationDB(t)
	ctx := context.Background()
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	memberID := channelID + 2
	bannedID := channelID + 3
	expiredBanID := channelID + 4
	hiddenID := channelID + 5
	outsiderID := channelID + 6
	cleanupMigrationChannel(t, channelID)
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID, Creator: ownerID, Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{memberID, bannedID, expiredBanID, hiddenID} {
		if err := JoinChannel(channelID, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE apifull_channel_member SET banned_rights=? WHERE channel_id=? AND user_id=?`, `{"view_messages":true}`, channelID, bannedID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE apifull_channel_member SET banned_rights=? WHERE channel_id=? AND user_id=?`, `{"view_messages":true,"until_date":1}`, channelID, expiredBanID); err != nil {
		t.Fatal(err)
	}
	row, err := InsertChannelMessageWithReplyAndRandomIDForDelivery(channelID, ownerID, time.Now().Unix(), "visibility", 0, 0, channelID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_message_hidden (user_id,channel_id,message_id) VALUES (?,?,?)`, hiddenID, channelID, row.MessageID); err != nil {
		t.Fatal(err)
	}
	recipients := []ChannelDeliveryRecipient{
		{UserID: ownerID}, {UserID: memberID}, {UserID: bannedID},
		{UserID: expiredBanID}, {UserID: hiddenID}, {UserID: outsiderID},
	}
	got, err := ChannelDeliveryRecipientsCanReceive(ctx, channelID, ownerID, recipients, []int32{row.MessageID})
	if err != nil {
		t.Fatal(err)
	}
	for userID, want := range map[int64]bool{
		ownerID: true, memberID: true, bannedID: false,
		expiredBanID: true, hiddenID: false, outsiderID: false,
	} {
		if got[userID] != want {
			t.Errorf("recipient %d can receive=%v, want %v", userID, got[userID], want)
		}
	}
}
