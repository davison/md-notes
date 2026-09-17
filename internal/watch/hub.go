package watch

import "sync"

// Hub fans one root's batches out to any number of subscribers.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan Batch]*subscriber
	closed bool
}

type subscriber struct {
	lost bool
}

// NewHub returns an empty Hub.
func NewHub() *Hub { return &Hub{subs: map[chan Batch]*subscriber{}} }

// Subscribe returns a channel of batches and a function that unsubscribes
// and closes it. A subscriber that falls behind drops batches rather than
// blocking the others; the next batch it does receive has empty Paths so
// it refreshes everything. On a hub that has been closed the channel comes
// back already closed, so a subscriber that raced the close still ends.
//
// Membership in subs is what decides whether a channel is still to be
// closed, and it is read and written under the mutex: the cancel below and
// Close can both reach the same channel, and only the first of them may
// close it.
func (h *Hub) Subscribe() (<-chan Batch, func()) {
	ch := make(chan Batch, 8)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.subs[ch] = &subscriber{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; !ok {
			return
		}
		delete(h.subs, ch)
		close(ch)
	}
}

// Close ends every subscription and refuses new ones. It is how a stream
// on a root that has just been unregistered finds out: its channel closes,
// the handler returns, and the browser's reconnect meets the 404 an
// unknown slug now gives. Safe to call more than once.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// Publish delivers b to every subscriber.
func (h *Hub) Publish(b Batch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, sub := range h.subs {
		out := b
		if sub.lost {
			out = Batch{Paths: []string{}}
		}
		select {
		case ch <- out:
			sub.lost = false
		default:
			sub.lost = true
		}
	}
}

// Len reports the number of subscribers.
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Pump publishes every batch from w to h until w's events close.
func (h *Hub) Pump(w *Watcher) {
	for b := range w.Events() {
		h.Publish(b)
	}
}
