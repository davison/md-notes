package watch

import (
	"reflect"
	"testing"
	"time"
)

func TestHubFanOutAndUnsubscribe(t *testing.T) {
	h := NewHub()
	a, cancelA := h.Subscribe()
	b, cancelB := h.Subscribe()
	defer cancelB()
	h.Publish(Batch{Paths: []string{"x.md"}})
	for _, ch := range []<-chan Batch{a, b} {
		select {
		case got := <-ch:
			if !reflect.DeepEqual(got.Paths, []string{"x.md"}) {
				t.Fatalf("got %v", got.Paths)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive")
		}
	}
	cancelA()
	cancelA()
	if _, ok := <-a; ok {
		t.Fatal("cancelled channel still open")
	}
	if h.Len() != 1 {
		t.Fatalf("Len = %d, want 1", h.Len())
	}
	h.Publish(Batch{})
}

func TestHubSlowSubscriberDropsNotBlocks(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe()
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			h.Publish(Batch{Paths: []string{"x"}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
}
