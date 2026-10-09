package domain

import "testing"

func TestPostgresSavedPaymentInfoRoundTripAndIndependentClear(t *testing.T) {
	requirePaymentLedgerDB(t)
	userID := paymentPostgresFixture(t)
	if err := SaveSavedPaymentInfo(userID, "Alice", "+8613800000000", "alice@example.com"); err != nil {
		t.Fatalf("save payment info: %v", err)
	}
	if err := SetSavedPaymentCredentials(userID, true); err != nil {
		t.Fatalf("set credentials marker: %v", err)
	}
	info, found, err := LoadSavedPaymentInfo(userID)
	if err != nil || !found || info.UserID != userID || info.Name != "Alice" || info.Phone != "+8613800000000" || info.Email != "alice@example.com" || !info.HasSavedCredentials {
		t.Fatalf("saved payment info = %+v found=%v err=%v", info, found, err)
	}
	if err = ClearSavedPaymentInfo(userID, true, false); err != nil {
		t.Fatalf("clear contact info: %v", err)
	}
	info, found, err = LoadSavedPaymentInfo(userID)
	if err != nil || !found || info.Name != "" || info.Phone != "" || info.Email != "" || !info.HasSavedCredentials {
		t.Fatalf("contact clear changed credentials: %+v found=%v err=%v", info, found, err)
	}
	if err = ClearSavedPaymentInfo(userID, false, true); err != nil {
		t.Fatalf("clear credentials marker: %v", err)
	}
	if info, found, err = LoadSavedPaymentInfo(userID); err != nil || found || info.HasSavedCredentials {
		t.Fatalf("full clear = %+v found=%v err=%v", info, found, err)
	}
}
