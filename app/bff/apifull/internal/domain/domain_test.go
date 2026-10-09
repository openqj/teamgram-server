package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestValidateStarsDeltaRejectsUnsafeAmounts(t *testing.T) {
	tests := []struct {
		name    string
		balance int64
		delta   int64
		want    error
	}{
		{name: "zero", balance: 10, delta: 0, want: ErrInvalidStarsTransaction},
		{name: "insufficient", balance: 10, delta: -11, want: ErrStarsBalanceExceeded},
		{name: "minimum debit", balance: math.MaxInt64, delta: math.MinInt64, want: ErrStarsBalanceExceeded},
		{name: "credit overflow", balance: math.MaxInt64, delta: 1, want: ErrStarsBalanceOverflow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateStarsDelta(tc.balance, tc.delta); err == nil || !errors.Is(err, tc.want) {
				t.Fatalf("validateStarsDelta(%d, %d) = %v, want %v", tc.balance, tc.delta, err, tc.want)
			}
		})
	}
}

func TestStarsAndChannelRoundTrip(t *testing.T) {
	user := paymentPostgresFixture(t)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_channel WHERE id=$1`, user)
	})
	bal, err := ApplyStars(user, 100, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if bal != 100 {
		t.Fatalf("balance %d", bal)
	}
	again, err := ApplyStars(user, 100, "seed")
	if err != nil || again != bal {
		t.Fatalf("idempotent: %d %v", again, err)
	}
	debited, err := ApplyStars(user, -10, "debit")
	if err != nil {
		t.Fatal(err)
	}
	if debited != bal-10 {
		t.Fatalf("debit %d from %d", debited, bal)
	}
	if _, err = ApplyStars(user, -100000, "too-much"); err == nil {
		t.Fatal("expected insufficient balance")
	}
	ch := Channel{ID: user, AccessHash: 7, Creator: user, Title: "prod", Broadcast: true}
	if err = SaveChannel(ch); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadChannel(user)
	if err != nil || !ok || got.Title != "prod" || !got.Broadcast {
		t.Fatalf("channel %+v ok %v err %v", got, ok, err)
	}
}

func TestListInactiveChannelsUsesCanonicalMessageActivity(t *testing.T) {
	requirePaymentLedgerDB(t)
	cutoff := time.Now().Add(-30 * 24 * time.Hour).Unix()
	oldID := time.Now().UnixNano()
	newID := oldID + 1
	owner := oldID + 2
	if err := SaveChannel(Channel{ID: oldID, AccessHash: oldID, Creator: owner, Title: "inactive", CreatedAt: cutoff - 1}); err != nil {
		t.Fatal(err)
	}
	if err := SaveChannel(Channel{ID: newID, AccessHash: newID, Creator: owner, Title: "active", CreatedAt: cutoff + 1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_channel_message WHERE channel_id IN (?, ?)`, oldID, newID)
		_, _ = db.Exec(`DELETE FROM apifull_channel WHERE id IN (?, ?)`, oldID, newID)
	})
	inactive, err := ListInactiveChannels(owner, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(inactive) != 1 || inactive[0].Channel.ID != oldID || inactive[0].LastActive != cutoff-1 {
		t.Fatalf("inactive channels = %+v, want only %d", inactive, oldID)
	}
	if _, err = db.Exec(`INSERT INTO apifull_channel_message (channel_id, message_id, sender_user_id, date, message) VALUES (?, 1, ?, ?, 'recent')`, oldID, owner, cutoff+1); err != nil {
		t.Fatal(err)
	}
	inactive, err = ListInactiveChannels(owner, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(inactive) != 0 {
		t.Fatalf("recent message did not make channel active: %+v", inactive)
	}
}
