package api

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

type sseClient struct {
	agentID   string
	namespace string
	send      chan string
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]*sseClient
}

func newHub() *Hub {
	return &Hub{
		clients: make(map[string]*sseClient),
	}
}

func (h *Hub) register(agentID, namespace string) *sseClient {
	client := &sseClient{
		agentID:   agentID,
		namespace: namespace,
		send:      make(chan string, 16),
	}
	h.mu.Lock()
	h.clients[agentID] = client
	h.mu.Unlock()
	return client
}

func (h *Hub) unregister(agentID string) {
	h.mu.Lock()
	delete(h.clients, agentID)
	h.mu.Unlock()
}

func (h *Hub) broadcast(namespace string, payload string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if client.namespace != namespace {
			continue
		}
		select {
		case client.send <- payload:
		default:
			log.Warn().Str("agent_id", client.agentID).Msg("SSE send buffer full, dropping message")
		}
	}
}

func (h *Hub) serveSSE(client *sseClient, w http.ResponseWriter, r *http.Request) {
	flusher := w.(http.Flusher)

	for {
		select {
		case msg := <-client.send:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}