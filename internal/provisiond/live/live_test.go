package live

import (
	"testing"

	"github.com/bumaruf/discord-go-vpn/internal/provisiond/metrics"
)

func TestHubCloseEndsStreams(t *testing.T) {
	h := NewHub()
	open, unsubscribe := h.subscribe()

	h.Close()
	if _, ok := <-open.ch; ok {
		t.Error("a subscriber's channel stayed open after Close")
	}
	unsubscribe() // must not close the channel twice

	late, _ := h.subscribe()
	if _, ok := <-late.ch; ok {
		t.Error("a subscriber after Close got an open channel")
	}
	h.Publish(metrics.Snapshot{})
}
