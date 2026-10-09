package domain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

func paymentPostgresFixture(t *testing.T) int64 {
	t.Helper()
	requirePaymentLedgerDB(t)
	userID := time.Now().UnixNano()
	t.Cleanup(func() {
		for _, table := range []string{"apifull_payment_saved_info", "apifull_payment_entitlement_outbox", "apifull_payment_receipt", "apifull_payment_ledger", "apifull_payment_request", "apifull_star_tx", "apifull_stars"} {
			if _, err := db.Exec(`DELETE FROM `+table+` WHERE user_id=$1`, userID); err != nil {
				t.Errorf("clean %s fixture: %v", table, err)
			}
		}
	})
	return userID
}

func TestPostgresDomainRequiresExplicitDSN(t *testing.T) {
	for _, dsn := range []string{"", " \t\n"} {
		if err := OpenPostgresReadOnly(dsn); err == nil {
			t.Fatalf("OpenPostgresReadOnly(%q) accepted a missing DSN", dsn)
		}
	}
}

func TestPostgresStarsAndGiftReadsUsePostgresParameters(t *testing.T) {
	userID := paymentPostgresFixture(t)
	product := fmt.Sprintf("pg-stars-%d", userID)
	slug := fmt.Sprintf("pg-gift-%d", userID)
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM apifull_stars_offer WHERE store_product=$1`, product); err != nil {
			t.Errorf("clean stars offer fixture: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM apifull_gift WHERE slug=$1`, slug); err != nil {
			t.Errorf("clean gift fixture: %v", err)
		}
	})

	if err := UpsertStarsOffer("topup", 17, product, "USD", 170, true); err != nil {
		t.Fatalf("upsert stars offer: %v", err)
	}
	offers, err := ListStarsOffers("topup")
	if err != nil {
		t.Fatalf("list stars offers: %v", err)
	}
	var foundOffer StarsOffer
	for _, offer := range offers {
		if offer.StoreProduct == product {
			foundOffer = offer
			break
		}
	}
	if foundOffer.ID == 0 || foundOffer.Stars != 17 || !foundOffer.Extended {
		t.Fatalf("stars offer readback: %+v", foundOffer)
	}
	matched, found, err := FindActiveStarsTopupOffer(17, product, "USD", 170, ptrBool(true))
	if err != nil || !found || matched.ID != foundOffer.ID {
		t.Fatalf("find stars offer: %+v found=%v err=%v", matched, found, err)
	}

	if _, err := ApplyStars(userID, 17, "pg-stars-seed"); err != nil {
		t.Fatalf("apply stars: %v", err)
	}
	transactions, err := ListStarsTransactions(userID, true, false, true, 0, 10)
	if err != nil || len(transactions) != 1 || transactions[0].Amount != 17 {
		t.Fatalf("list stars transactions: %+v err=%v", transactions, err)
	}
	transaction, found, err := GetStarsTransaction(userID, transactions[0].Idem)
	if err != nil || !found || transaction.ID != transactions[0].ID {
		t.Fatalf("get stars transaction: %+v found=%v err=%v", transaction, found, err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 17 {
		t.Fatalf("stars balance: %d err=%v", balance, err)
	}

	if err := SaveGift(userID+1, userID, slug, 17); err != nil {
		t.Fatalf("save gift: %v", err)
	}
	gift, found, err := FindGiftBySlug(slug)
	if err != nil || !found || gift.To != userID || gift.Slug != slug {
		t.Fatalf("find gift: %+v found=%v err=%v", gift, found, err)
	}
	gifts, err := ListGifts(userID, false)
	if err != nil || len(gifts) != 1 || gifts[0].ID != gift.ID {
		t.Fatalf("list gifts: %+v err=%v", gifts, err)
	}
}

func ptrBool(value bool) *bool {
	return &value
}

func TestPostgresPremiumSettlementReplayAndTransactionConflict(t *testing.T) {
	userID := paymentPostgresFixture(t)
	key := fmt.Sprintf("premium-%d", userID)
	transactionID := key + ":transaction"
	request, err := BeginPaymentRequest(userID, key, "fixture", "premium-fingerprint", "USD", 499, userID, 7)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		settled, receipt, err := SettlePremiumPaymentRequest(userID, key, "premium-fingerprint", transactionID, "USD", "Premium", 499, userID, 7, []byte("signed-premium-receipt"), 3)
		if err != nil || settled.State != PaymentStateSettled || receipt.RequestID != request.ID {
			t.Fatalf("settlement attempt %d: state=%s receipt=%+v err=%v", attempt, settled.State, receipt, err)
		}
	}
	if err = EnsurePremiumGrant(request.ID, userID, "fixture", transactionID, 3); err != nil {
		t.Fatalf("entitlement replay: %v", err)
	}
	if err = EnsurePremiumGrant(request.ID, userID, "fixture", transactionID, 6); !errors.Is(err, ErrPaymentTransactionConflict) {
		t.Fatalf("conflicting entitlement: %v", err)
	}
	if err = RetryPremiumGrant(context.Background(), request.ID, errors.New("temporary provider failure")); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	var months, attempts int32
	var state string
	var updatedAt, nextAttemptAt int64
	if err = db.QueryRow(`SELECT months, state, attempts, updated_at, next_attempt_at FROM apifull_payment_entitlement_outbox WHERE request_id=$1`, request.ID).
		Scan(&months, &state, &attempts, &updatedAt, &nextAttemptAt); err != nil || months != 3 || state != "pending" || attempts != 1 || nextAttemptAt != updatedAt+2 {
		t.Fatalf("entitlement retry: months=%d state=%s attempts=%d updated=%d next=%d err=%v", months, state, attempts, updatedAt, nextAttemptAt, err)
	}
	if err = CompletePremiumGrant(context.Background(), request.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = SettlePremiumPaymentRequest(userID, key, "premium-fingerprint", transactionID, "USD", "Premium", 499, userID, 7, []byte("signed-premium-receipt"), 3); err != nil {
		t.Fatalf("settled replay after entitlement delivered: %v", err)
	}
	if err = db.QueryRow(`SELECT state FROM apifull_payment_entitlement_outbox WHERE request_id=$1`, request.ID).Scan(&state); err != nil || state != "complete" {
		t.Fatalf("replay reopened completed entitlement: state=%s err=%v", state, err)
	}

	otherKey := key + ":other"
	other, err := BeginPaymentRequest(userID, otherKey, "fixture", "other-fingerprint", "USD", 499, userID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = SettlePremiumPaymentRequest(userID, otherKey, "other-fingerprint", transactionID, "USD", "Premium", 499, userID, 8, []byte("signed-premium-receipt"), 3); !errors.Is(err, ErrPaymentTransactionConflict) {
		t.Fatalf("transaction reused by another payment: %v", err)
	}
	assertPaymentPendingWithoutSettlement(t, other)
	var ledgerEntries int
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_payment_ledger WHERE request_id=$1`, request.ID).Scan(&ledgerEntries); err != nil || ledgerEntries != 2 {
		t.Fatalf("replay duplicated ledger: count=%d err=%v", ledgerEntries, err)
	}
}

func TestPostgresPaymentSettlementRollsBackWhenGrantFails(t *testing.T) {
	userID := paymentPostgresFixture(t)
	key := fmt.Sprintf("rollback-%d", userID)
	request, err := BeginPaymentRequest(userID, key, "fixture", "fingerprint", "USD", 499, userID, 9)
	if err != nil {
		t.Fatal(err)
	}
	transactionID := key + ":transaction"
	if err = EnsurePremiumGrant(request.ID, userID, "fixture", transactionID, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err = SettlePremiumPaymentRequest(userID, key, "fingerprint", transactionID, "USD", "Premium", 499, userID, 9, []byte("signed-premium-receipt"), 3); !errors.Is(err, ErrPaymentTransactionConflict) {
		t.Fatalf("conflicting grant: %v", err)
	}
	assertPaymentPendingWithoutSettlement(t, request)
	var months int32
	if err = db.QueryRow(`SELECT months FROM apifull_payment_entitlement_outbox WHERE request_id=$1`, request.ID).Scan(&months); err != nil || months != 1 {
		t.Fatalf("rollback changed prior grant: months=%d err=%v", months, err)
	}

	starsKey := key + ":stars"
	starsRequest, err := BeginPaymentRequest(userID, starsKey, "fixture", "stars-fingerprint", "USD", 100, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ApplyStars(userID, math.MaxInt64, key+":seed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = SettleStarsPaymentRequest(userID, starsKey, "stars-fingerprint", key+":stars-tx", "USD", "Stars", 100, 1, []byte("signed-stars-receipt")); !errors.Is(err, ErrStarsBalanceOverflow) {
		t.Fatalf("overflowing Stars grant: %v", err)
	}
	assertPaymentPendingWithoutSettlement(t, starsRequest)
	if balance, err := StarsBalance(userID); err != nil || balance != math.MaxInt64 {
		t.Fatalf("failed settlement changed Stars balance: %d %v", balance, err)
	}
}

func assertPaymentPendingWithoutSettlement(t *testing.T, request PaymentRequest) {
	t.Helper()
	got, found, err := LoadPaymentRequest(request.UserID, request.RequestKey)
	if err != nil || !found || got.State != PaymentStatePending || got.TransactionID != "" {
		t.Fatalf("request after rollback: %+v found=%v err=%v", got, found, err)
	}
	var receipts, entries int
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_payment_receipt WHERE request_id=$1`, request.ID).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("rollback left receipt: count=%d err=%v", receipts, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_payment_ledger WHERE request_id=$1`, request.ID).Scan(&entries); err != nil || entries != 1 {
		t.Fatalf("rollback left settlement ledger: count=%d err=%v", entries, err)
	}
}

func TestPostgresStarsConcurrentDebitRetryIsIdempotent(t *testing.T) {
	userID := paymentPostgresFixture(t)
	if _, err := ApplyStars(userID, 100, "seed"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT balance FROM apifull_stars WHERE user_id=$1 FOR UPDATE`, userID); err != nil {
		t.Fatal(err)
	}
	const workers = 6
	type result struct {
		balance int64
		err     error
	}
	results := make(chan result, workers)
	for i := 0; i < workers; i++ {
		go func() {
			balance, err := ApplyStars(userID, -100, "debit")
			results <- result{balance: balance, err: err}
		}()
	}
	// Hold the balance until every writer is queued so the pre-migration
	// absent-ledger race is exercised deterministically.
	deadline := time.Now().Add(5 * time.Second)
	queued := 0
	for time.Now().Before(deadline) {
		err = db.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE datname=current_database()
			AND wait_event_type='Lock' AND query LIKE '%apifull_stars%'`).Scan(&queued)
		if err != nil || queued >= workers {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = tx.Rollback()
	for i := 0; i < workers; i++ {
		select {
		case got := <-results:
			if got.err != nil || got.balance != 0 {
				t.Errorf("retry result: balance=%d err=%v", got.balance, got.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent debit did not finish")
		}
	}
	if err != nil || queued < workers {
		t.Fatalf("writers did not queue under the fixture lock: count=%d err=%v", queued, err)
	}
	if _, err = ApplyStars(userID, -1, "debit"); !errors.Is(err, ErrStarsIdempotencyConflict) {
		t.Fatalf("conflicting retry: %v", err)
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 0 {
		t.Fatalf("ledger balance: %d %v", balance, err)
	}
	var entries int
	if err = db.QueryRow(`SELECT COUNT(*) FROM apifull_star_tx WHERE user_id=$1`, userID).Scan(&entries); err != nil || entries != 2 {
		t.Fatalf("concurrent retry duplicated ledger: count=%d err=%v", entries, err)
	}
}

func TestPostgresPaymentLocksWaitAndRelease(t *testing.T) {
	userID := paymentPostgresFixture(t)
	key := fmt.Sprintf("payment-lock-%d", userID)
	release, err := LockPaymentRequest(context.Background(), userID, key, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	started := time.Now()
	if unlock, err := LockPaymentRequest(context.Background(), userID, key, 80*time.Millisecond); !errors.Is(err, ErrPaymentRequestBusy) || unlock != nil || time.Since(started) < 60*time.Millisecond {
		t.Fatalf("bounded lock wait: unlock=%v elapsed=%s err=%v", unlock != nil, time.Since(started), err)
	}
	release()
	release()
	reacquired, err := LockPaymentRequest(context.Background(), userID, key, time.Second)
	if err != nil {
		t.Fatalf("lock leaked after release: %v", err)
	}
	reacquired()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if unlock, err := LockPaymentRequest(ctx, userID, key, time.Second); !errors.Is(err, context.Canceled) || unlock != nil {
		t.Fatalf("canceled lock request: unlock=%v err=%v", unlock != nil, err)
	}
}

func TestPostgresGiftConversionCreditsOnceAndRollsBack(t *testing.T) {
	userID := paymentPostgresFixture(t)
	slug := fmt.Sprintf("gift-%d", userID)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_gift WHERE to_user=$1`, userID)
	})
	if err := SaveGift(userID+1, userID, slug, 25); err != nil {
		t.Fatal(err)
	}
	const workers = 4
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() { errs <- ConvertGift(userID, nil, slug) }()
	}
	for i := 0; i < workers; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("conversion retry: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("conversion retry did not finish")
		}
	}
	if balance, err := StarsBalance(userID); err != nil || balance != 25 {
		t.Fatalf("gift credit: balance=%d err=%v", balance, err)
	}
	if _, err := ApplyStars(userID, math.MaxInt64-25, "max-balance"); err != nil {
		t.Fatal(err)
	}
	overflowSlug := slug + ":overflow"
	if err := SaveGift(userID+1, userID, overflowSlug, 1); err != nil {
		t.Fatal(err)
	}
	if err := ConvertGift(userID, nil, overflowSlug); !errors.Is(err, ErrStarsBalanceOverflow) {
		t.Fatalf("overflow conversion: %v", err)
	}
	var saved, entries int
	if err := db.QueryRow(`SELECT saved FROM apifull_gift WHERE to_user=$1 AND slug=$2`, userID, overflowSlug).Scan(&saved); err != nil || saved != 1 {
		t.Fatalf("failed conversion consumed gift: saved=%d err=%v", saved, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM apifull_star_tx WHERE user_id=$1`, userID).Scan(&entries); err != nil || entries != 2 {
		t.Fatalf("gift conversion ledger: entries=%d err=%v", entries, err)
	}
}

func TestPostgresGiftSaveRechecksOwnerAfterConcurrentTransfer(t *testing.T) {
	userID := paymentPostgresFixture(t)
	nextOwner := userID + 2
	slug := fmt.Sprintf("gift-owner-%d", userID)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_gift WHERE slug=$1 AND to_user IN ($2,$3)`, slug, userID, nextOwner)
	})
	if err := SaveGift(userID+1, userID, slug, 25); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE apifull_gift SET to_user=$1 WHERE to_user=$2 AND slug=$3`, nextOwner, userID, slug); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- SetGiftSaved(userID, nil, slug, false) }()
	queued := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err = db.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE datname=current_database()
			AND wait_event_type='Lock' AND query LIKE '%apifull_gift%'`).Scan(&queued)
		if err != nil || queued > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case saveErr := <-finished:
		if !errors.Is(saveErr, ErrGiftNotFound) {
			t.Fatalf("former owner save: %v", saveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("former owner save did not finish")
	}
	if queued == 0 {
		t.Fatal("gift save did not wait for the concurrent transfer")
	}
	var saved int
	if err = db.QueryRow(`SELECT saved FROM apifull_gift WHERE to_user=$1 AND slug=$2`, nextOwner, slug).Scan(&saved); err != nil || saved != 1 {
		t.Fatalf("former owner altered transferred gift: saved=%d err=%v", saved, err)
	}
}
