package model

import "testing"

func TestQRCodeTransactionExcludesUser(t *testing.T) {
	qrCode := &QRCodeTransaction{ExceptIDs: []int64{7, 42}}
	if !qrCode.ExcludesUser(42) {
		t.Fatal("ExcludesUser(42) = false, want true")
	}
	if qrCode.ExcludesUser(43) {
		t.Fatal("ExcludesUser(43) = true, want false")
	}
}
