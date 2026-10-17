package api

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/AkZcH/herald/internal/targeting"
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
	watchers []*watcher
	index    *targeting.Index
	onDrop  func()
}

func newHub() *Hub {
	return &Hub{
		clients: make(map[string]*sseClient),
				index:   targeting.NewIndex(),
	}
}

type watcher struct {
	key       string
	namespace string
	send      chan string
}

func (h *Hub) register(agentID, namespace string, labels map[string]string) *sseClient {
	client := &sseClient{
		agentID:   agentID,
		namespace: namespace,
		send:      make(chan string, 16),
	}
	h.mu.Lock()
	h.clients[agentID] = client
	h.mu.Unlock()
	h.index.Upsert(agentID, namespace, labels)
	return client
}

func (h *Hub) unregister(agentID string) {
	h.mu.Lock()
	delete(h.clients, agentID)
	h.mu.Unlock()
	h.index.Remove(agentID)
}

func (h *Hub) broadcastTargeted(namespace string, payload string, sel targeting.Selector) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	matchedAgents := h.index.Match(namespace, sel)
	matched := make(map[string]bool, len(matchedAgents))
	for _, id := range matchedAgents {
		matched[id] = true
	}

	for _, client := range h.clients {
		if client.namespace != namespace {
			continue
		}
		if !sel.IsEmpty() && !matched[client.agentID] {
			continue
		}
		select {
		case client.send <- payload:
		default:
			log.Warn().Str("agent_id", client.agentID).Msg("SSE send buffer full, dropping message")
			if h.onDrop != nil {
				h.onDrop()
			} 
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

func (h *Hub) addWatcher(key, namespace string) *watcher {
	w := &watcher{
		key:       key,
		namespace: namespace,
		send:      make(chan string, 16),
	}
	h.mu.Lock()
	if h.watchers == nil {
		h.watchers = make([]*watcher, 0)
	}
	h.watchers = append(h.watchers, w)
	h.mu.Unlock()
	return w
}

func (h *Hub) removeWatcher(w *watcher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, ww := range h.watchers {
		if ww == w {
			h.watchers = append(h.watchers[:i], h.watchers[i+1:]...)
			return
		}
	}
}

func (h *Hub) notifyWatchers(key, namespace, payload string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, w := range h.watchers {
		if w.key == key && w.namespace == namespace {
			select {
			case w.send <- payload:
			default:
			}
		}
	}
}