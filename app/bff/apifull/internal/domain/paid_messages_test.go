package domain

import (
	"errors"
	"testing"
	"time"
)

func TestUpdatePaidMessagesPricePostgresRoundTrip(t *testing.T) {
	requireMigrationDB(t)
	channelID := time.Now().UnixNano()
	creatorID := channelID + 1
	accessHash := channelID + 2
	if err := SaveChannel(Channel{
		ID: channelID, AccessHash: accessHash, Creator: creatorID,
		Title: "paid-price-test", Broadcast: true,
	}); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM apifull_channel WHERE id=$1`, channelID); err != nil {
			t.Errorf("cleanup channel: %v", err)
		}
	})

	updated, err := UpdatePaidMessagesPrice(creatorID, channelID, accessHash, 17, true)
	if err != nil {
		t.Fatalf("update paid-message price: %v", err)
	}
	if updated.PaidMessagesStars == nil || *updated.PaidMessagesStars != 17 || !updated.BroadcastMessagesAllowed {
		t.Fatalf("updated channel policy: %+v", updated)
	}
	loaded, ok, err := LoadChannel(channelID)
	if err != nil || !ok || loaded.PaidMessagesStars == nil || *loaded.PaidMessagesStars != 17 || !loaded.BroadcastMessagesAllowed {
		t.Fatalf("readback policy: channel=%+v ok=%v err=%v", loaded, ok, err)
	}
	if _, err = UpdatePaidMessagesPrice(creatorID+1, channelID, accessHash, 18, false); !errors.Is(err, ErrNotCreator) {
		t.Fatalf("non-creator update error=%v, want ErrNotCreator", err)
	}
	if _, err = UpdatePaidMessagesPrice(creatorID, channelID, accessHash+1, 18, false); !errors.Is(err, ErrInvalidChannelAccessHash) {
		t.Fatalf("stale access hash error=%v, want ErrInvalidChannelAccessHash", err)
	}
}
