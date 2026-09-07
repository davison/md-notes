package watch

import "sync"

// Hub fans one root's batches out to any number of subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Batch]struct{}
}

// NewHub returns an empty Hub.
func NewHub() *Hub { return &Hub{subs: map[chan Batch]struct{}{}} }

// Subscribe returns a channel of batches and a function that unsubscribes
// and closes it. A subscriber that falls behind drops batches rather than
// blocking the others.
func (h *Hub) Subscribe() (<-chan Batch, func()) {
	ch := make(chan Batch, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// Publish delivers b to every subscriber.
func (h *Hub) Publish(b Batch) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
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
