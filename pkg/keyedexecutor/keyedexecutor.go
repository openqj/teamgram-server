// Copyright 2024 Teamgram Authors
//  All rights reserved.

// Package keyedexecutor runs bounded concurrent work while preserving FIFO
// order for jobs that share a key.
package keyedexecutor

import (
	"context"
	"sync"
)

const inFlightMultiplier = 4

type job[T any] struct {
	key   string
	value T
}

type keyQueue[T any] struct {
	jobs    []job[T]
	running bool
}

// Executor keeps one active handler per key and bounds the number of queued
// and running jobs. The handler is called in input order for each key; jobs
// belonging to different keys may run concurrently.
type Executor[T any] struct {
	ctx     context.Context
	handler func(context.Context, T)

	stop     chan struct{}
	stopOnce sync.Once

	acceptMu  sync.RWMutex
	accepting bool

	slots chan struct{}
	ready chan job[T]

	mu      sync.Mutex
	queues  map[string]*keyQueue[T]
	tasks   sync.WaitGroup
	workers sync.WaitGroup
}

// New starts a bounded executor. A nil context is treated as Background.
func New[T any](ctx context.Context, concurrency int, handler func(context.Context, T)) *Executor[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	if concurrency < 1 {
		concurrency = 1
	}
	inFlight := concurrency * inFlightMultiplier
	if inFlight < concurrency {
		inFlight = concurrency
	}
	e := &Executor[T]{
		ctx:       ctx,
		handler:   handler,
		stop:      make(chan struct{}),
		accepting: true,
		slots:     make(chan struct{}, inFlight),
		ready:     make(chan job[T], inFlight),
		queues:    make(map[string]*keyQueue[T]),
	}
	for i := 0; i < inFlight; i++ {
		e.slots <- struct{}{}
	}
	for i := 0; i < concurrency; i++ {
		e.workers.Add(1)
		go e.worker()
	}
	return e
}

func (e *Executor[T]) worker() {
	defer e.workers.Done()
	for {
		select {
		case <-e.stop:
			return
		case current := <-e.ready:
			e.handler(e.ctx, current.value)
			e.tasks.Done()
			e.slots <- struct{}{}
			e.scheduleNext(current.key)
		}
	}
}

// Submit blocks when the bounded in-flight budget is exhausted. It returns
// false when the parent context is cancelled or after Wait has begun.
func (e *Executor[T]) Submit(key string, value T) bool {
	e.acceptMu.RLock()
	if !e.accepting {
		e.acceptMu.RUnlock()
		return false
	}
	select {
	case <-e.ctx.Done():
		e.acceptMu.RUnlock()
		return false
	case <-e.slots:
	}
	if e.ctx.Err() != nil {
		e.slots <- struct{}{}
		e.acceptMu.RUnlock()
		return false
	}

	e.mu.Lock()
	e.tasks.Add(1)
	queue := e.queues[key]
	if queue == nil {
		queue = &keyQueue[T]{}
		e.queues[key] = queue
	}
	queue.jobs = append(queue.jobs, job[T]{key: key, value: value})
	var first job[T]
	ready := false
	if !queue.running {
		queue.running = true
		first = queue.jobs[0]
		queue.jobs = queue.jobs[1:]
		ready = true
	}
	e.mu.Unlock()
	e.acceptMu.RUnlock()

	if ready {
		// The ready buffer has the same capacity as the in-flight budget. A
		// slot was consumed above, so this send cannot grow without bound.
		e.ready <- first
	}
	return true
}

func (e *Executor[T]) scheduleNext(key string) {
	e.mu.Lock()
	queue := e.queues[key]
	if queue == nil || len(queue.jobs) == 0 {
		if queue != nil {
			delete(e.queues, key)
		}
		e.mu.Unlock()
		return
	}
	next := queue.jobs[0]
	queue.jobs = queue.jobs[1:]
	e.mu.Unlock()
	e.ready <- next
}

// Wait stops accepting work, drains all accepted jobs, and then stops the
// worker goroutines. It is safe to call more than once.
func (e *Executor[T]) Wait() {
	e.stopOnce.Do(func() {
		e.acceptMu.Lock()
		e.accepting = false
		e.acceptMu.Unlock()
		e.tasks.Wait()
		close(e.stop)
	})
	e.workers.Wait()
}
