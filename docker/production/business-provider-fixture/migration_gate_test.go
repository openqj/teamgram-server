package main

import "testing"

func TestValidMigrationChecksum(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, value := range []string{valid, "", "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef", valid[:63]} {
		want := value == valid
		if got := validMigrationChecksum(value); got != want {
			t.Fatalf("validMigrationChecksum(%q) = %v, want %v", value, got, want)
		}
	}
}
