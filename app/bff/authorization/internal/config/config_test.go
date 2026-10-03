package config

import "testing"

func TestConfigSupportsDcDefaultsToLocalOnly(t *testing.T) {
	c := Config{DcId: 1}
	if !c.SupportsDc(1) {
		t.Fatal("local DC should be supported")
	}
	if c.SupportsDc(2) {
		t.Fatal("unknown DC should be rejected when no peers are configured")
	}
}

func TestConfigSupportsConfiguredPeerDc(t *testing.T) {
	c := Config{DcId: 1, KnownDcIds: []int32{2, 3}}
	for _, dcID := range []int32{1, 2, 3} {
		if !c.SupportsDc(dcID) {
			t.Fatalf("DC %d should be supported", dcID)
		}
	}
	for _, dcID := range []int32{0, -1, 4} {
		if c.SupportsDc(dcID) {
			t.Fatalf("DC %d should be rejected", dcID)
		}
	}
}
