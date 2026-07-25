package api

import (
	"context"
	"encoding/json"
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

type ackRequest struct {
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
	labelsRaw := r.URL.Query().Get("labels")

	if namespace == "" {
		namespace = "default"
	}

	var labels map[string]string
	if labelsRaw != "" {
		json.Unmarshal([]byte(labelsRaw), &labels)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	client := s.hub.register(agentID, namespace, labels)
	defer s.hub.unregister(agentID)

	log.Info().Str("agent_id", agentID).Str("namespace", namespace).Msg("agent SSE connected")

	connected, _ := json.Marshal(map[string]string{
		"type":     "connected",
		"agent_id": agentID,
	})
	client.send <- string(connected)

	s.hub.serveSSE(client, w, r)

	log.Info().Str("agent_id", agentID).Msg("agent SSE disconnected")
}

func (s *Server) handleAck(w http.ResponseWriter, r *http.Request) {
	var req ackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err := s.db.ExecContext(r.Context(), `
		INSERT INTO ack_states (version_id, agent_id, acked_at, status)
		VALUES ($1, $2, $3, 'ACKED')
		ON CONFLICT (version_id, agent_id) DO NOTHING
	`, req.VersionID, req.AgentID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record ack")
		return
	}

	log.Info().
		Str("agent_id", req.AgentID).
		Str("version_id", req.VersionID).
		Msg("ack received")

	w.WriteHeader(http.StatusOK)
}