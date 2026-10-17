package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultAuditLimit = 20
	maxAuditLimit     = 500
)

type auditEntry struct {
	ID          string          `json:"id"`
	EventType   string          `json:"event_type"`
	Key         string          `json:"key"`
	Namespace   string          `json:"namespace"`
	OldValue    json.RawMessage `json:"old_value"`
	NewValue    json.RawMessage `json:"new_value"`
	VersionID   string          `json:"version_id"`
	VectorClock json.RawMessage `json:"vector_clock"`
	Actor       string          `json:"actor"`
	Targeting   json.RawMessage `json:"targeting"`
	CreatedAt   time.Time       `json:"created_at"`
}

const listAuditQuery = `
	SELECT id::text, event_type, key, namespace,
	       old_value, new_value,
	       COALESCE(version_id::text, ''),
	       vector_clock, actor, targeting, created_at
	FROM audit_log
	WHERE ($1 = '' OR namespace = $1)
	ORDER BY created_at DESC
	LIMIT $2`

// handleAuditList serves GET /audit?namespace=&last=N (newest first).
func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")

	limit := defaultAuditLimit
	if raw := r.URL.Query().Get("last"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "last must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxAuditLimit {
		limit = maxAuditLimit
	}

	rows, err := s.db.QueryContext(r.Context(), listAuditQuery, namespace, limit)
	if err != nil {
		log.Error().Err(err).Msg("list audit failed")
		writeError(w, http.StatusInternalServerError, "failed to read audit log")
		return
	}
	defer rows.Close()

	entries := make([]auditEntry, 0, limit)
	for rows.Next() {
		var e auditEntry
		var oldV, newV, vc, tgt []byte
		if err := rows.Scan(
			&e.ID, &e.EventType, &e.Key, &e.Namespace,
			&oldV, &newV, &e.VersionID,
			&vc, &e.Actor, &tgt, &e.CreatedAt,
		); err != nil {
			log.Error().Err(err).Msg("scan audit row failed")
			writeError(w, http.StatusInternalServerError, "failed to read audit log")
			return
		}
		e.OldValue, e.NewValue, e.VectorClock, e.Targeting = oldV, newV, vc, tgt
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read audit log")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}