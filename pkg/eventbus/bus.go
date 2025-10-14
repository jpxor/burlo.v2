// Copyright (C) 2025 Josh Simonot
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package eventbus

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
)

type Topic string
type Event = any

// Bus implements an in-memory pub/sub where the most recent event
// is the only one kept per subscriber.
type Bus struct {
	mu        sync.RWMutex
	subs      map[Topic]map[uint64]chan Event
	last      map[Topic]Event
	idCounter uint64
	closed    atomic.Bool

	eventCount       atomic.Int64
	sendCount        atomic.Int64
	sendDropCount    atomic.Int64
	sendReplaceCount atomic.Int64
}

func (b *Bus) PrintStats() {
	log.Println("event count:", b.eventCount.Load())
	log.Println("send count:", b.sendCount.Load())
	log.Println("send overwrit:", b.sendReplaceCount.Load())
	log.Println("send dropped count:", b.sendDropCount.Load())
}

// New returns an initialized Bus.
func New() *Bus {
	return &Bus{
		subs: make(map[Topic]map[uint64]chan Event),
		last: make(map[Topic]Event),
	}
}

// Publish publishes ev to topic. It stores ev as the "last" event for the topic.
// For each subscriber, the channel is size 1; publishing will replace any older value in the channel
// so that subscribers always see the most recent event.
func (b *Bus) Publish(topic Topic, ev Event) {
	if b.closed.Load() {
		return
	}

	b.eventCount.Add(1)

	// Save last event
	b.mu.Lock()
	b.last[topic] = ev

	// Create a copy of existing channels to avoid holding lock while sending
	var chans []chan Event
	if m, ok := b.subs[topic]; ok {
		chans = make([]chan Event, 0, len(m))
		for _, ch := range m {
			chans = append(chans, ch)
		}
	}
	b.mu.Unlock()

	// Send to each subscriber with "replace oldest" semantics
	for _, ch := range chans {
		b.publishReplace(ch, ev)
	}
}

// publishReplace tries to deliver ev to ch. If ch is full, it removes the existing item (if any)
// and then attempts to send ev. All operations are non-blocking to avoid global stalls.
func (b *Bus) publishReplace(ch chan Event, ev Event) {
	// Fast path: try sending
	select {
	case ch <- ev:
		return
	default:
	}

	// Channel full: drop the old value (non-blocking receive) then attempt to send the new one.
	select {
	case <-ch:
		b.sendReplaceCount.Add(1)
	default:
	}
	select {
	case ch <- ev:
	default:
		log.Printf("[error] dropped event: %+v", ev)
		b.sendDropCount.Add(1)
		return
	}
	b.sendCount.Add(1)
}

// Subscribe subscribes to a topic and returns a receive-only channel and an unsubscribe func.
// If withLast is true and there is a stored "last" event, that event will be delivered immediately
// (subject to replace semantics).
// The subscription is automatically removed and the channel closed when ctx is canceled.
// You may also call the returned unsubscribe() to remove the subscription earlier.
func (b *Bus) Subscribe(ctx context.Context, topic Topic, withLast bool) (<-chan Event, func()) {

	if b.closed.Load() {
		// If someone tries to subscribe after the bus is closed
		// they get a closed channel
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan Event, 1)
	id := atomic.AddUint64(&b.idCounter, 1)

	b.mu.Lock()
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[uint64]chan Event)
	}
	b.subs[topic][id] = ch

	// capture last if requested
	var last Event
	var hasLast bool
	if withLast {
		last, hasLast = b.last[topic]
	}
	b.mu.Unlock()

	// deliver last if requested
	if withLast && hasLast {
		b.publishReplace(ch, last)
	}

	// Unsubscribe helper
	done := make(chan struct{})
	unsub := func() {
		// signal goroutine to remove & close channel
		select {
		case <-done:
			// already unsubscribed
		default:
			close(done)
		}
	}

	// start a goroutine to cleanup on ctx cancel or explicit unsubscribe
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}

		// remove subscription and close channel
		b.mu.Lock()
		if m, ok := b.subs[topic]; ok {
			delete(m, id)
			if len(m) == 0 {
				delete(b.subs, topic)
			}
		}
		b.mu.Unlock()
		close(ch)
	}()

	return ch, unsub
}

// GetLast returns the last published event for a topic (if any).
func (b *Bus) GetLast(topic Topic) (Event, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.last[topic]
	return v, ok
}

// Close closes the bus and all subscriber channels. After Close, Publish is a no-op and Subscribe
// returns a closed channel.
func (b *Bus) Close() {
	if b.closed.Swap(true) {
		return // already closed
	}
	b.mu.Lock()
	for _, m := range b.subs {
		for _, ch := range m {
			close(ch)
		}
	}
	b.subs = nil
	b.last = nil
	b.mu.Unlock()
}
