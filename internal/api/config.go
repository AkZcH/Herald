package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

type configWriteRequest struct {
	Key             string         `json:"key"`
	Value           any            `json:"value"`
	Namespace       string         `json:"namespace"`
	TTL             string         `json:"ttl,omitempty"`
	TargetingLabels map[string]string `json:"targeting_labels,omitempty"`
	PushedBy        string         `json:"pushed_by"`
}

type configWriteResponse struct {
	VersionID string `json:"version_id"`
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
}

func (s *Server) handleConfigWrite(w http.ResponseWriter, r *http.Request) {
	var req configWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Key = strings.TrimSpace(req.Key)
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.PushedBy == "" {
		req.PushedBy = "anonymous"
	}

	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if req.Value == nil {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(configWriteResponse{
		Key:       req.Key,
		Namespace: req.Namespace,
		Status:    "PENDING",
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}