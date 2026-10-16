package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPushSessionTargetsBoundsConcurrencyAndReturnsFailure(t *testing.T) {
	const (
		targetCount = 24
		concurrency = 3
	)
	wantErr := errors.New("session unavailable")
	targets := make([]sessionPushTarget, targetCount)
	for i := range targets {
		targets[i].authKeyID = int64(i + 1)
	}

	var mu sync.Mutex
	active, maxActive, completed := 0, 0, 0
	err := pushSessionTargets(context.Background(), targets, concurrency, func(_ context.Context, target sessionPushTarget) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(time.Millisecond)

		mu.Lock()
		active--
		completed++
		mu.Unlock()
		if target.authKeyID == 7 {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("pushSessionTargets error = %v, want %v", err, wantErr)
	}
	if maxActive > concurrency {
		t.Fatalf("maximum concurrent pushes = %d, want <= %d", maxActive, concurrency)
	}
	if completed != targetCount {
		t.Fatalf("completed pushes = %d, want %d", completed, targetCount)
	}
}
