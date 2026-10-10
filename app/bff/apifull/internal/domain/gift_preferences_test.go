package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPostgresGiftPreferencesAreAtomic(t *testing.T) {
	requirePaymentLedgerDB(t)
	owner := time.Now().UnixNano()
	first := fmt.Sprintf("gift-pin-%d-a", owner)
	second := fmt.Sprintf("gift-pin-%d-b", owner)
	if err := SaveGift(owner+1, owner, first, 10); err != nil {
		t.Fatalf("save first gift: %v", err)
	}
	if err := SaveGift(owner+1, owner, second, 20); err != nil {
		t.Fatalf("save second gift: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_gift_notification_setting WHERE owner_user_id=$1`, owner)
		_, _ = db.Exec(`DELETE FROM apifull_gift_pin WHERE owner_user_id=$1`, owner)
		_, _ = db.Exec(`DELETE FROM apifull_gift WHERE to_user=$1`, owner)
	})

	if err := SetGiftPinnedOrder(owner, []GiftRef{{Slug: first}, {Slug: second}}); err != nil {
		t.Fatalf("pin gifts: %v", err)
	}
	var firstPosition, secondPosition int
	if err := db.QueryRow(`SELECT position FROM apifull_gift_pin p JOIN apifull_gift g ON g.id=p.gift_id WHERE p.owner_user_id=$1 AND g.slug=$2`, owner, first).Scan(&firstPosition); err != nil {
		t.Fatalf("read first pin: %v", err)
	}
	if err := db.QueryRow(`SELECT position FROM apifull_gift_pin p JOIN apifull_gift g ON g.id=p.gift_id WHERE p.owner_user_id=$1 AND g.slug=$2`, owner, second).Scan(&secondPosition); err != nil {
		t.Fatalf("read second pin: %v", err)
	}
	if firstPosition != 0 || secondPosition != 1 {
		t.Fatalf("pin order = (%d,%d), want (0,1)", firstPosition, secondPosition)
	}
	if err := SetGiftPinnedOrder(owner, []GiftRef{{Slug: "missing-gift"}, {Slug: first}}); !errors.Is(err, ErrGiftPreferenceInvalid) {
		t.Fatalf("invalid replacement error = %v, want ErrGiftPreferenceInvalid", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM apifull_gift_pin WHERE owner_user_id=$1`, owner).Scan(&count); err != nil || count != 2 {
		t.Fatalf("failed replacement changed pins: count=%d err=%v", count, err)
	}
	if err := SetGiftNotificationSetting(owner, 1, owner, true); err != nil {
		t.Fatalf("enable notifications: %v", err)
	}
	if err := SetGiftNotificationSetting(owner, 1, owner, false); err != nil {
		t.Fatalf("disable notifications: %v", err)
	}
	var enabled bool
	if err := db.QueryRow(`SELECT enabled FROM apifull_gift_notification_setting WHERE owner_user_id=$1 AND peer_type=1 AND peer_id=$2`, owner, owner).Scan(&enabled); err != nil || enabled {
		t.Fatalf("notification setting = %v err=%v, want false", enabled, err)
	}
}
