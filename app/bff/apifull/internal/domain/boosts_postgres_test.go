package domain

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func requireBoostsDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Fatalf("open boost database: %v", err)
	}
	t.Cleanup(func() { _ = Close() })
}

func TestBoostSlotsTransactionalReassignment(t *testing.T) {
	requireBoostsDB(t)
	userID := time.Now().UnixNano()
	target1 := userID + 1
	target2 := userID + 2
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_boost_slot WHERE user_id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM apifull_boost_target WHERE peer_id IN ($1,$2)`, target1, target2)
	})
	if _, err := ApplyBoost(userID, BoostScopePremium, 4, target1, []int32{1, 2}); err != nil {
		t.Fatal(err)
	}
	first, count, err := ListBoosts(BoostScopePremium, 4, target1, nil, false, 0, 100)
	if err != nil || count != 2 || len(first) != 2 {
		t.Fatalf("initial boosts=%d rows=%d err=%v", count, len(first), err)
	}
	if _, err = ApplyBoost(userID, BoostScopePremium, 4, target2, []int32{1}); err != nil {
		t.Fatal(err)
	}
	_, count, err = ListBoosts(BoostScopePremium, 4, target1, nil, false, 0, 100)
	if err != nil || count != 1 {
		t.Fatalf("reassigned old target count=%d err=%v", count, err)
	}
	_, count, err = ListBoosts(BoostScopePremium, 4, target2, nil, false, 0, 100)
	if err != nil || count != 1 {
		t.Fatalf("reassigned new target count=%d err=%v", count, err)
	}
}

func TestBoostConcurrentAssignmentsKeepOneSlot(t *testing.T) {
	requireBoostsDB(t)
	userID := time.Now().UnixNano()
	target := userID + 3
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_boost_slot WHERE user_id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM apifull_boost_target WHERE peer_id=$1`, target)
	})
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ApplyBoost(userID, BoostScopeStories, 2, target, []int32{1})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, count, err := ListBoosts(BoostScopeStories, 2, target, nil, false, 0, 100)
	if err != nil || count != 1 || len(rows) != 1 {
		t.Fatalf("concurrent rows=%d count=%d err=%v", len(rows), count, err)
	}
}

func TestBoostRestrictionRequiresChannelOwner(t *testing.T) {
	requireBoostsDB(t)
	owner := time.Now().UnixNano()
	channelID := owner + 10
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "restricted"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DeleteChannel(owner, channelID) })
	if err := SetBoostRestrictions(owner+1, 4, channelID, 2); !errors.Is(err, ErrBoostForbidden) {
		t.Fatalf("unauthorized restriction error=%v", err)
	}
	if err := SetBoostRestrictions(owner, 4, channelID, 2); err != nil {
		t.Fatal(err)
	}
	target, found, err := LoadBoostTarget(BoostScopePremium, 4, channelID)
	if err != nil || !found || target.BlockedBoosts != 2 || target.OwnerUserID != owner {
		t.Fatalf("target=%+v found=%v err=%v", target, found, err)
	}
}
