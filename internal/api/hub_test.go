package api

import (
	"testing"

	"github.com/AkZcH/herald/internal/targeting"
)

func TestBroadcastCountsDroppedDeliveries(t *testing.T) {
	h := newHub()
	drops := 0
	h.onDrop = func() { drops++ }

	c := h.register("agent-1", "prod", nil)
	for i := 0; i < cap(c.send); i++ {
		c.send <- "queued"
	}

	// buffer is full, so this push must be dropped and counted
	h.broadcastTargeted("prod", "payload", targeting.Selector{})

	if drops != 1 {
		t.Fatalf("drops = %d, want 1", drops)
	}
}

func TestBroadcastDoesNotCountSuccessfulDeliveries(t *testing.T) {
	h := newHub()
	drops := 0
	h.onDrop = func() { drops++ }

	c := h.register("agent-1", "prod", nil)
	h.broadcastTargeted("prod", "payload", targeting.Selector{})

	if drops != 0 {
		t.Fatalf("drops = %d, want 0", drops)
	}
	if got := <-c.send; got != "payload" {
		t.Fatalf("got %q, want payload", got)
	}
}