package config

import "testing"

func TestSupportsDcRequiresExplicitKnownSet(t *testing.T) {
	c := Config{DcId: 2}
	if !c.SupportsDc(2) {
		t.Fatal("local DC should be supported")
	}
	if c.SupportsDc(1) {
		t.Fatal("unconfigured remote DC should not be supported")
	}
	c.KnownDcIds = []int32{1}
	if !c.SupportsDc(1) {
		t.Fatal("configured remote DC should be supported")
	}
	if c.SupportsDc(0) {
		t.Fatal("zero DC should not be supported")
	}
}
