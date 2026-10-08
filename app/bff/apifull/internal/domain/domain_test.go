package domain

import (
	"errors"
	"math"
	"os"
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
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured; PostgreSQL runtime tests cover production storage")
	}
	if err := Open(dsn); err != nil {
		t.Fatal(err)
	}
	const user int64 = 424242
	bal, err := ApplyStars(user, 100, "seed-424242")
	if err != nil {
		t.Fatal(err)
	}
	if bal != 100 && bal != 90 {
		// 90 if a previous debit from a rerun already happened after a seed that was idempotent.
		if bal != 100 {
			t.Fatalf("balance %d", bal)
		}
	}
	again, err := ApplyStars(user, 100, "seed-424242")
	if err != nil || again != bal {
		t.Fatalf("idempotent: %d %v", again, err)
	}
	debited, err := ApplyStars(user, -10, "debit-424242")
	if err != nil {
		t.Fatal(err)
	}
	if debited != bal-10 && debited != 90 {
		t.Fatalf("debit %d from %d", debited, bal)
	}
	if _, err = ApplyStars(user, -100000, "too-much-424242"); err == nil {
		t.Fatal("expected insufficient balance")
	}
	ch := Channel{ID: 424242, AccessHash: 7, Creator: user, Title: "prod", Broadcast: true}
	if err = SaveChannel(ch); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadChannel(424242)
	if err != nil || !ok || got.Title != "prod" || !got.Broadcast {
		t.Fatalf("channel %+v ok %v err %v", got, ok, err)
	}
}

func TestListInactiveChannelsUsesCanonicalMessageActivity(t *testing.T) {
	dsn := os.Getenv("APIFULL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("APIFULL_MYSQL_DSN is not configured; PostgreSQL runtime tests cover production storage")
	}
	if err := Open(dsn); err != nil {
		t.Fatal(err)
	}
	const owner int64 = 424243
	cutoff := time.Now().Add(-30 * 24 * time.Hour).Unix()
	oldID := time.Now().UnixNano()
	newID := oldID + 1
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
