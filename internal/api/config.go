package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/AkZcH/herald/internal/audit"
	"github.com/AkZcH/herald/internal/store"
	"github.com/AkZcH/herald/internal/vectorclock"
	"github.com/go-chi/chi/v5"
)

type configWriteRequest struct {
	Key             string            `json:"key"`
	Value           any               `json:"value"`
	Namespace       string            `json:"namespace"`
	TTL             string            `json:"ttl,omitempty"`
	TargetingLabels map[string]string `json:"targeting_labels,omitempty"`
	PushedBy        string            `json:"pushed_by"`
}

type configWriteResponse struct {
	VersionID   string `json:"version_id"`
	Key         string `json:"key"`
	Namespace   string `json:"namespace"`
	Status      string `json:"status"`
	VectorClock any    `json:"vector_clock"`
}

type configGetResponse struct {
	Key         string         `json:"key"`
	Namespace   string         `json:"namespace"`
	Value       any            `json:"value"`
	VersionID   string         `json:"version_id"`
	Status      string         `json:"status"`
	VectorClock map[string]int `json:"vector_clock"`
	PushedBy    string         `json:"pushed_by"`
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

	valueJSON, err := json.Marshal(req.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid value")
		return
	}

	var targetingJSON []byte
	if req.TargetingLabels != nil {
		targetingJSON, _ = json.Marshal(req.TargetingLabels)
	}

	db := store.NewDBFromSQL(s.db)

	prev, err := db.GetLatestConfigVersion(r.Context(), req.Key, req.Namespace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch previous version")
		return
	}

	vc := vectorclock.New()
	if prev != nil {
		vc, err = vectorclock.FromJSON(prev.VectorClock)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse vector clock")
			return
		}
	}
	vc = vc.Increment(s.nodeID)

	clockJSON, err := vc.ToJSON()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to serialize vector clock")
		return
	}

	cv := store.ConfigVersion{
		Key:             req.Key,
		Namespace:       req.Namespace,
		Value:           valueJSON,
		VectorClock:     clockJSON,
		Status:          "PENDING",
		TargetingLabels: targetingJSON,
		PushedBy:        req.PushedBy,
	}

	if prev != nil {
		cv.PreviousID = &prev.ID
	}

	versionID, err := db.InsertConfigVersion(r.Context(), cv)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist config version")
		return
	}

var oldValue []byte
if prev != nil {
    oldValue = prev.Value
}

if err := s.auditWriter.Write(r.Context(), audit.Entry{
    EventType:   audit.EventConfigWrite,
    Key:         req.Key,
    Namespace:   req.Namespace,
    OldValue:    oldValue,
    NewValue:    valueJSON,
    VersionID:   versionID,
    VectorClock: clockJSON,
    Actor:       req.PushedBy,
    Targeting:   targetingJSON,
}); err != nil {
    writeError(w, http.StatusInternalServerError, "failed to write audit log")
    return
}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(configWriteResponse{
		VersionID:   versionID,
		Key:         req.Key,
		Namespace:   req.Namespace,
		Status:      "PENDING",
		VectorClock: vc,
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	db := store.NewDBFromSQL(s.db)
	cv, err := db.GetLatestConfigVersion(r.Context(), key, namespace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch config")
		return
	}
	if cv == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("key %q not found in namespace %q", key, namespace))
		return
	}

	var value any
	if err := json.Unmarshal(cv.Value, &value); err != nil {
		value = string(cv.Value)
	}

	var vc map[string]int
	json.Unmarshal(cv.VectorClock, &vc)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configGetResponse{
		Key:         cv.Key,
		Namespace:   cv.Namespace,
		Value:       value,
		VersionID:   cv.ID,
		Status:      cv.Status,
		VectorClock: vc,
		PushedBy:    cv.PushedBy,
	})
}

func (s *Server) handleConfigDiff(w http.ResponseWriter, r *http.Request) {
	v1ID := r.URL.Query().Get("v1")
	v2ID := r.URL.Query().Get("v2")

	if v1ID == "" || v2ID == "" {
		writeError(w, http.StatusBadRequest, "v1 and v2 query params are required")
		return
	}

	db := store.NewDBFromSQL(s.db)

	v1, err := db.GetConfigVersionByID(r.Context(), v1ID)
	if err != nil || v1 == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("version %s not found", v1ID))
		return
	}

	v2, err := db.GetConfigVersionByID(r.Context(), v2ID)
	if err != nil || v2 == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("version %s not found", v2ID))
		return
	}

	var oldValue, newValue any
	json.Unmarshal(v1.Value, &oldValue)
	json.Unmarshal(v2.Value, &newValue)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"key":       v1.Key,
		"namespace": v1.Namespace,
		"v1": map[string]any{
			"id":    v1.ID,
			"value": oldValue,
		},
		"v2": map[string]any{
			"id":    v2.ID,
			"value": newValue,
		},
		"pushed_by": v2.PushedBy,
	})
}