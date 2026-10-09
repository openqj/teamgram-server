package main

import "testing"

func TestValidatePostgres18Version(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion int
		wantError     bool
	}{
		{name: "PostgreSQL 18", serverVersion: 180006},
		{name: "PostgreSQL 17", serverVersion: 170006, wantError: true},
		{name: "PostgreSQL 19", serverVersion: 190000, wantError: true},
		{name: "unknown version", serverVersion: 0, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePostgres18Version(tt.serverVersion)
			if (err != nil) != tt.wantError {
				t.Fatalf("validatePostgres18Version(%d) error = %v, wantError %t", tt.serverVersion, err, tt.wantError)
			}
		})
	}
}
