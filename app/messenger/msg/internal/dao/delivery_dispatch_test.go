package dao

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishMessagePublicationSerializesOnlyMatchingKey(t *testing.T) {
	d := &Dao{}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- d.publishMessagePublication(context.Background(), "inbox", "same", func() error {
			close(firstStarted)
			<-releaseFirst
			return nil
		})
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first publication did not start")
	}

	otherDone := make(chan error, 1)
	go func() {
		otherDone <- d.publishMessagePublication(context.Background(), "inbox", "other", func() error { return nil })
	}()
	select {
	case err := <-otherDone:
		if err != nil {
			t.Fatalf("independent publication failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("independent publication was serialized behind another key")
	}

	sameStarted := make(chan struct{})
	sameDone := make(chan error, 1)
	go func() {
		sameDone <- d.publishMessagePublication(context.Background(), "inbox", "same", func() error {
			close(sameStarted)
			return nil
		})
	}()
	select {
	case <-sameStarted:
		t.Fatal("matching publication bypassed the in-flight key")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first publication failed: %v", err)
	}
	select {
	case <-sameStarted:
	case <-time.After(time.Second):
		t.Fatal("matching publication did not resume after the first completed")
	}
	if err := <-sameDone; err != nil {
		t.Fatalf("matching publication failed: %v", err)
	}
}

func TestRunDeliveryWorkersBoundsIndependentConcurrency(t *testing.T) {
	var claimed atomic.Int32
	var active atomic.Int32
	var maximum atomic.Int32
	claim := func(context.Context) (*int, error) {
		value := int(claimed.Add(1))
		if value > 12 {
			return nil, nil
		}
		return &value, nil
	}
	dispatch := func(context.Context, *int) (int, error) {
		current := active.Add(1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return 1, nil
	}
	count, err := runDeliveryWorkers(context.Background(), 12, 3, claim, dispatch)
	if err != nil || count != 12 {
		t.Fatalf("worker result count=%d err=%v", count, err)
	}
	if maximum.Load() > 3 {
		t.Fatalf("worker concurrency exceeded bound: %d", maximum.Load())
	}
}

func TestDeliveryConcurrencyDefaultsAndClamps(t *testing.T) {
	if got := (&Dao{}).deliveryConcurrency(); got != defaultDeliveryPublishConcurrency {
		t.Fatalf("default delivery concurrency = %d", got)
	}
	if got := (&Dao{DeliveryConcurrency: 256}).deliveryConcurrency(); got != 128 {
		t.Fatalf("maximum delivery concurrency = %d", got)
	}
}
