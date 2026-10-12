package core

import (
	"net/url"
	"testing"
)

func TestSecurePremiumProviderURLRejectsInsecureRemoteByDefault(t *testing.T) {
	parsed, err := url.Parse("http://payment-provider-fixture:18080")
	if err != nil {
		t.Fatal(err)
	}
	if securePremiumProviderURL(parsed, false) {
		t.Fatal("remote HTTP provider unexpectedly accepted without fixture mode")
	}
}

func TestSecurePremiumProviderURLAllowsInsecureFixtureEndpoint(t *testing.T) {
	parsed, err := url.Parse("http://payment-provider-fixture:18080")
	if err != nil {
		t.Fatal(err)
	}
	if !securePremiumProviderURL(parsed, true) {
		t.Fatal("fixture HTTP provider rejected with fixture mode")
	}
}
