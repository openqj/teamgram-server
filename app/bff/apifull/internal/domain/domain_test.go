package domain

import (
	"errors"
	"math"
	"os"
	"testing"
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
		t.Fatal("APIFULL_MYSQL_DSN must point to an isolated test database")
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
