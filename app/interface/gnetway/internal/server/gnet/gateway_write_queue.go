// Copyright 2026 The Teamgram Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package gnet

import (
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
)

const (
	// Keep the queue bounded so a slow client cannot consume unbounded memory.
	gatewayWriteQueueLimit = 128
	// Keep the callback batch below the queue limit so one busy connection
	// yields regularly to other event-loop work during a broadcast.
	gatewayWriteBatchSize = 32
)

type gatewayWriteItem struct {
	msg      *mtproto.MTPRawMessage
	queuedAt time.Time
}

type gatewayWriteQueue struct {
	mu        sync.Mutex
	items     []gatewayWriteItem
	scheduled bool
}

func (q *gatewayWriteQueue) enqueue(item gatewayWriteItem) (schedule bool, accepted bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) >= gatewayWriteQueueLimit {
		return false, false
	}
	q.items = append(q.items, item)
	if !q.scheduled {
		q.scheduled = true
		return true, true
	}
	return false, true
}

func (q *gatewayWriteQueue) take(limit int) []gatewayWriteItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if limit <= 0 || len(q.items) == 0 {
		return nil
	}
	if limit > len(q.items) {
		limit = len(q.items)
	}
	items := append([]gatewayWriteItem(nil), q.items[:limit]...)
	copy(q.items, q.items[limit:])
	q.items = q.items[:len(q.items)-limit]
	return items
}

// reschedule returns true when another event-loop callback is needed. The
// scheduled bit is cleared while holding the same lock used by enqueue, so a
// producer racing with the final drain either observes the bit or schedules
// the next callback itself.
func (q *gatewayWriteQueue) reschedule() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) > 0 {
		return true
	}
	q.scheduled = false
	return false
}

func (q *gatewayWriteQueue) depth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
