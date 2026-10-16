package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type agentInfo struct {
	AgentID    string            `json:"agent_id"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Status     string            `json:"status"`
	LastSeen   time.Time         `json:"last_seen"`
	VersionID  string            `json:"version_id"`
	ConfigHash string            `json:"config_hash"`
}

const listAgentsQuery = `
	SELECT a.agent_id,
	       a.namespace,
	       a.labels,
	       a.status,
	       a.last_seen,
	       COALESCE(a.version_id::text, ''),
	       COALESCE(a.config_hash, md5(cv.value::text), '')
	FROM agent_registry a
	LEFT JOIN config_versions cv ON cv.id = a.version_id
	WHERE ($1 = '' OR a.namespace = $1)
	ORDER BY a.namespace, a.agent_id`

// handleAgentsList serves GET /agents?namespace=
func (s *Server) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")

	rows, err := s.db.QueryContext(r.Context(), listAgentsQuery, namespace)
	if err != nil {
		log.Error().Err(err).Msg("list agents failed")
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	defer rows.Close()

	agents := make([]agentInfo, 0)
	for rows.Next() {
		var a agentInfo
		var labelsRaw []byte
		if err := rows.Scan(
			&a.AgentID, &a.Namespace, &labelsRaw, &a.Status,
			&a.LastSeen, &a.VersionID, &a.ConfigHash,
		); err != nil {
			log.Error().Err(err).Msg("scan agent row failed")
			writeError(w, http.StatusInternalServerError, "failed to read agents")
			return
		}
		a.Labels = map[string]string{}
		_ = json.Unmarshal(labelsRaw, &a.Labels)
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read agents")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}