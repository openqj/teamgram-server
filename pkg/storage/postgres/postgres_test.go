package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateServerVersion(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion int
		wantError     bool
	}{
		{name: "PostgreSQL 18.0", serverVersion: 180000},
		{name: "PostgreSQL 18.6", serverVersion: 180006},
		{name: "PostgreSQL 17", serverVersion: 170011, wantError: true},
		{name: "PostgreSQL 19", serverVersion: 190000, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServerVersion(tt.serverVersion)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateServerVersion(%d) error = %v, wantError %v", tt.serverVersion, err, tt.wantError)
			}
		})
	}
}

func TestNewPoolRejectsNonPostgres18(t *testing.T) {
	dsn := os.Getenv("TEAMGRAM_POSTGRES_NON18_DSN")
	if dsn == "" {
		t.Skip("TEAMGRAM_POSTGRES_NON18_DSN must point to a reachable non-18 PostgreSQL instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, Config{DSN: dsn})
	if pool != nil {
		pool.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL 18 is required") {
		t.Fatalf("NewPool() = (%v, %v), want PostgreSQL 18 rejection", pool, err)
	}
}
