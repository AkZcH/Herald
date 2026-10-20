package api

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// An agent stream must outlive the HTTP server's WriteTimeout, which otherwise
// applies to the whole response and kills the stream shortly after it opens.
func TestStreamSurvivesServerWriteTimeout(t *testing.T) {
	h := newHub()
	client := h.register("a1", "prod", nil)
	client.send <- `{"type":"connected"}`

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		clearWriteDeadline(w)
		h.serveSSE(client, w, r)
	}))
	srv.Config.WriteTimeout = 300 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	if line, _ := reader.ReadString('\n'); !strings.Contains(line, "connected") {
		t.Fatalf("expected the connected event first, got %q", line)
	}

	time.Sleep(700 * time.Millisecond) // well past WriteTimeout
	client.send <- `{"type":"config_push"}`

	got := make(chan string, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				got <- "read error: " + err.Error()
				return
			}
			if strings.Contains(line, "config_push") {
				got <- line
				return
			}
		}
	}()
	select {
	case line := <-got:
		if !strings.Contains(line, "config_push") {
			t.Fatalf("stream died after WriteTimeout, push lost: %s", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("push never arrived")
	}
}