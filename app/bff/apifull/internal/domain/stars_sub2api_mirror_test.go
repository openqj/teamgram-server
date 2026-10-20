package domain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestStarsFromSub2APIUSDUsesFixedRateAndRoundsEachSignedEventUp(t *testing.T) {
	for _, test := range []struct {
		amount string
		want   int64
	}{
		{amount: "1", want: 8},
		{amount: "0.1", want: 1},
		{amount: "0.1", want: 1},
		{amount: "-0.1", want: -1},
		{amount: "-0.125", want: -1},
		{amount: "2.5", want: 18},
	} {
		amount, _, err := parseStarsMirrorDecimal(test.amount, 18)
		if err != nil {
			t.Fatal(err)
		}
		got, err := starsFromSub2APIUSD(amount)
		if err != nil || got != test.want {
			t.Errorf("starsFromSub2APIUSD(%q) = (%d, %v), want %d", test.amount, got, err, test.want)
		}
	}
}

func TestStarsFromSub2APIUSDRoundsEventsIndependently(t *testing.T) {
	smallDelta, _, err := parseStarsMirrorDecimal("0.01", 18)
	if err != nil {
		t.Fatal(err)
	}
	combined, _, err := parseStarsMirrorDecimal("0.02", 18)
	if err != nil {
		t.Fatal(err)
	}
	first, err := starsFromSub2APIUSD(smallDelta)
	if err != nil {
		t.Fatal(err)
	}
	second, err := starsFromSub2APIUSD(smallDelta)
	if err != nil {
		t.Fatal(err)
	}
	combinedStars, err := starsFromSub2APIUSD(combined)
	if err != nil {
		t.Fatal(err)
	}
	if first+second != 2 || combinedStars != 1 {
		t.Fatalf("independent deltas = %d Stars, combined delta = %d Stars; want 2 and 1", first+second, combinedStars)
	}
}

func TestUSDFromStarsUsesFixedRateAndEightDecimalSnapshot(t *testing.T) {
	for _, test := range []struct {
		stars int64
		want  string
	}{
		{stars: 1, want: "0.13888889"},
		{stars: 2, want: "0.27777778"},
		{stars: -4, want: "0.55555556"},
	} {
		if got := usdFromStars(test.stars); got != test.want {
			t.Errorf("usdFromStars(%d) = %q, want %q", test.stars, got, test.want)
		}
	}
}

func TestPostgresSub2APIStarsUsageIsIdempotentAndDoesNotRefeed(t *testing.T) {
	requirePaymentLedgerDB(t)
	userID := time.Now().UnixNano()
	accountID := fmt.Sprintf("acct:00000000-0000-4000-8000-%012x", userID&0xffffffffffff)
	eventPrefix := fmt.Sprintf("sub2api-usage-test-%d", userID)
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			statement string
			args      []any
		}{
			{`DELETE FROM apifull_sub2api_stars_usage_event WHERE event_id LIKE $1`, []any{eventPrefix + "%"}},
			{`DELETE FROM apifull_sub2api_stars_account WHERE account_id=$1`, []any{accountID}},
			{`DELETE FROM apifull_stars_sub2api_outbox WHERE user_id=$1`, []any{userID}},
			{`DELETE FROM apifull_star_tx WHERE user_id=$1`, []any{userID}},
			{`DELETE FROM apifull_stars WHERE user_id=$1`, []any{userID}},
		} {
			if _, err := db.Exec(cleanup.statement, cleanup.args...); err != nil {
				t.Errorf("cleanup Stars usage fixture: %v", err)
			}
		}
	})
	if _, err := ApplyStars(userID, 2, "usage-seed"); err != nil {
		t.Fatalf("seed Stars: %v", err)
	}
	apply := func(eventID, amount string) (int64, error) {
		return ApplySub2APIStarsUsage(context.Background(), Sub2APIStarsUsageEvent{
			EventID: eventID, AccountID: accountID, UserID: userID, AmountUSD: amount, Currency: "USD",
		})
	}
	if stars, err := apply(eventPrefix+"-credit", "0.2"); err != nil || stars != 2 {
		t.Fatalf("positive balance event = (%d, %v), want +2 Stars", stars, err)
	}
	if stars, err := apply(eventPrefix+"-1", "-0.1"); err != nil || stars != -1 {
		t.Fatalf("first small event = (%d, %v), want -1 Star", stars, err)
	}
	if stars, err := apply(eventPrefix+"-1", "-0.10"); err != nil || stars != -1 {
		t.Fatalf("same event replay = (%d, %v), want idempotent -1 Star", stars, err)
	}
	if stars, err := apply(eventPrefix+"-2", "-0.1"); err != nil || stars != -1 {
		t.Fatalf("second small event = (%d, %v), want -1 Star", stars, err)
	}
	if _, err := apply(eventPrefix+"-2", "-0.2"); !errors.Is(err, ErrStarsUsageConflict) {
		t.Fatalf("changed replay error = %v, want conflict", err)
	}
	if _, err := ApplySub2APIStarsUsage(context.Background(), Sub2APIStarsUsageEvent{
		EventID: eventPrefix + "-wrong-account-owner", AccountID: accountID, UserID: userID + 1,
		AmountUSD: "0.1", Currency: "USD",
	}); !errors.Is(err, ErrStarsUsageConflict) {
		t.Fatalf("account owner change error = %v, want conflict", err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 2 {
		t.Fatalf("Stars balance after external usage = (%d, %v), want 2", balance, err)
	}
	if _, err := apply(eventPrefix+"-insufficient", "-0.75"); !errors.Is(err, ErrStarsBalanceExceeded) {
		t.Fatalf("insufficient usage error = %v, want balance exceeded", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_sub2api_stars_usage_event WHERE event_id=$1`, eventPrefix+"-insufficient").Scan(&count); err != nil || count != 0 {
		t.Fatalf("insufficient event row count = (%d, %v), want rollback", count, err)
	}
	if _, err := ApplyStars(userID, 4, "usage-retry-funding"); err != nil {
		t.Fatalf("fund retry: %v", err)
	}
	if stars, err := apply(eventPrefix+"-insufficient", "-0.75"); err != nil || stars != -6 {
		t.Fatalf("retried usage = (%d, %v), want -6 Stars", stars, err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 0 {
		t.Fatalf("Stars balance after retried usage = (%d, %v), want 0", balance, err)
	}
	var outboxCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_stars_sub2api_outbox WHERE user_id=$1`, userID).Scan(&outboxCount); err != nil || outboxCount != 2 {
		t.Fatalf("outbound outbox count = (%d, %v), want only two local credits", outboxCount, err)
	}
	events, err := ClaimStarsMirrorOutbox(context.Background(), 1000)
	if err != nil {
		t.Fatalf("claim mirror outbox: %v", err)
	}
	var ours []StarsMirrorOutboxEvent
	for _, event := range events {
		if event.UserID != userID {
			continue
		}
		ours = append(ours, event)
		if event.DeltaStars != 2 && event.DeltaStars != 4 {
			t.Errorf("claimed signed Stars delta = %d", event.DeltaStars)
		}
		if event.AmountUSD != "0.27777778" && event.AmountUSD != "0.55555556" {
			t.Errorf("claimed USD amount = %q", event.AmountUSD)
		}
		if err = CompleteStarsMirrorOutbox(context.Background(), event.EventID, event.ClaimToken); err != nil {
			t.Errorf("complete mirror event %s: %v", event.EventID, err)
		}
	}
	if len(ours) != 2 {
		t.Fatalf("claimed this user's mirror events = %d, want 2", len(ours))
	}
}
