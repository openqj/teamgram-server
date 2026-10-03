package codec

import "testing"

func TestDCIDOnlyObfuscatedTransportCarriesIdentity(t *testing.T) {
	obfuscated := &ObfuscatedCodec{dc: 2}
	got, ok := DCID(obfuscated)
	if !ok || got != 2 {
		t.Fatalf("DCID(obfuscated) = (%d, %v), want (2, true)", got, ok)
	}

	plain := newMTProtoFullCodec()
	if got, ok := DCID(plain); ok || got != 0 {
		t.Fatalf("DCID(plain) = (%d, %v), want (0, false)", got, ok)
	}
}
