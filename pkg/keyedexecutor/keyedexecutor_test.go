package keyedexecutor

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestExecutorPreservesKeyOrderWithoutBlockingOtherKeys(t *testing.T) {
	type task struct {
		id int
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstStarted := make(chan struct{})
	otherStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	var sameKeyOrder []int
	secondStarted := make(chan struct{})

	executor := New(ctx, 2, func(_ context.Context, current task) {
		switch current.id {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(secondStarted)
		}
		if current.id == 3 {
			close(otherStarted)
		}
		if current.id == 1 || current.id == 2 {
			mu.Lock()
			sameKeyOrder = append(sameKeyOrder, current.id)
			mu.Unlock()
		}
	})

	if !executor.Submit("same", task{id: 1}) || !executor.Submit("same", task{id: 2}) || !executor.Submit("other", task{id: 3}) {
		t.Fatal("executor rejected an accepted task")
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}
	select {
	case <-otherStarted:
	case <-time.After(time.Second):
		t.Fatal("different key was blocked by the first key")
	}
	select {
	case <-secondStarted:
		t.Fatal("same key ran concurrently")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	executor.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(sameKeyOrder) != 2 || sameKeyOrder[0] != 1 || sameKeyOrder[1] != 2 {
		t.Fatalf("same-key execution order = %v, want [1 2]", sameKeyOrder)
	}
}
