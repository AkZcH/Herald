package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type agentRegisterRequest struct {
	AgentID   string            `json:"agent_id"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
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