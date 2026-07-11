package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type agentRegisterRequest struct {
	AgentID   string            `json:"agent_id"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type heartbeatRequest struct {
	AgentID   string `json:"agent_id"`
	VersionID string `json:"version_id"`
}

func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	var req agentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	labelsJSON, _ := json.Marshal(req.Labels)

	_, err := s.db.ExecContext(r.Context(), `
		INSERT INTO agent_registry (agent_id, namespace, labels, status, last_seen)
		VALUES ($1, $2, $3, 'HEALTHY', $4)
		ON CONFLICT (agent_id) DO UPDATE
		SET namespace = $2, labels = $3, status = 'HEALTHY', last_seen = $4
	`, req.AgentID, req.Namespace, labelsJSON, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register agent")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "registered",
		"agent_id": req.AgentID,
	})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	_, err := s.db.ExecContext(r.Context(), `
		UPDATE agent_registry
		SET last_seen = $1, status = 'HEALTHY'
		WHERE agent_id = $2
	`, time.Now().UTC(), req.AgentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) StartStaleDetector(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := s.db.ExecContext(ctx, `
				UPDATE agent_registry
				SET status = 'STALE'
				WHERE status = 'HEALTHY'
				AND last_seen < $1
			`, time.Now().UTC().Add(-1500*time.Millisecond))
			if err != nil {
				log.Error().Err(err).Msg("stale detection failed")
			}
		}
	}
}

func (s *Server) handleAgentSSE(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	log.Info().Str("agent_id", agentID).Str("namespace", namespace).Msg("agent SSE connected")

	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"agent_id\":\"%s\"}\n\n", agentID)
	flusher.Flush()

	<-r.Context().Done()
	log.Info().Str("agent_id", agentID).Msg("agent SSE disconnected")
}