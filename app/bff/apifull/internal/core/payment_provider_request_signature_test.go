package core

import "testing"

func TestPaymentProviderRequestSignatureBindsExactBody(t *testing.T) {
	const key = "local-payment-provider-signing-key-20261012"
	body := []byte(`{"operation":"get_form","user_id":5}`)
	signature := paymentProviderRequestSignature(key, body)
	if !verifyPaymentProviderSignature(key, signature, body) {
		t.Fatal("request signature did not verify against the exact body")
	}
	if verifyPaymentProviderSignature(key, signature, []byte(`{"operation":"get_form","user_id":6}`)) {
		t.Fatal("request signature verified for a changed body")
	}
}
