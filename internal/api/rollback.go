package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/AkZcH/herald/internal/audit"
	"github.com/AkZcH/herald/internal/fsm"
	"github.com/AkZcH/herald/internal/raft"
	"github.com/AkZcH/herald/internal/store"
	"github.com/AkZcH/herald/internal/targeting"
	"github.com/rs/zerolog/log"
)

type rollbackRequest struct {
	VersionID string `json:"version_id"`
	Namespace string `json:"namespace"`
	Actor     string `json:"actor"`
}

type rollbackResponse struct {
	Status    string `json:"status"`
	VersionID string `json:"version_id"`
	Key       string `json:"key"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.VersionID == "" {
		writeError(w, http.StatusBadRequest, "version_id is required")
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Actor == "" {
		req.Actor = "anonymous"
	}

	db := store.NewDBFromSQL(s.db)

	target, err := db.GetConfigVersionByID(r.Context(), req.VersionID)
	if err != nil || target == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("version %s not found", req.VersionID))
		return
	}

	f := fsm.New()

	if err := f.Transition(fsm.StateRollbackPending); err != nil {
		writeError(w, http.StatusConflict, "rollback already in progress")
		return
	}

	log.Info().
		Str("version_id", req.VersionID).
		Str("key", target.Key).
		Str("fsm", string(f.State())).
		Msg("rollback initiated")

	if err := f.Transition(fsm.StateCommitting); err != nil {
		f.Transition(fsm.StateActive)
		writeError(w, http.StatusInternalServerError, "fsm transition failed")
		return
	}

	var value any
	json.Unmarshal(target.Value, &value)

	cv := store.ConfigVersion{
		Key:         target.Key,
		Namespace:   req.Namespace,
		Value:       target.Value,
		VectorClock: target.VectorClock,
		Status:      "PENDING",
		PushedBy:    fmt.Sprintf("rollback:%s", req.Actor),
	}
	if target.PreviousID != nil {
		cv.PreviousID = target.PreviousID
	}

	newVersionID, err := db.InsertConfigVersion(r.Context(), cv)
	if err != nil {
		f.Transition(fsm.StateActive)
		writeError(w, http.StatusInternalServerError, "failed to create rollback version")
		return
	}

	logIndex, err := s.raftNode.AppendToLog(r.Context(), newVersionID)
	if err != nil {
		f.Transition(fsm.StateActive)
		writeError(w, http.StatusInternalServerError, "failed to append to raft log")
		return
	}

	entry := raft.LogEntry{
		Term:            s.raftNode.Term(),
		Index:           logIndex,
		ConfigVersionID: newVersionID,
		Command:         "ROLLBACK",
	}

	quorumReached, err := s.raftNode.BroadcastAppendEntries(r.Context(), entry)
	if err != nil || !quorumReached {
		f.Transition(fsm.StateActive)
		writeError(w, http.StatusInternalServerError, "failed to reach quorum for rollback")
		return
	}

	if err := s.raftNode.CommitVersion(r.Context(), newVersionID); err != nil {
		f.Transition(fsm.StateActive)
		writeError(w, http.StatusInternalServerError, "failed to commit rollback version")
		return
	}

	pushPayload, _ := json.Marshal(map[string]any{
		"type":       "config_push",
		"version_id": newVersionID,
		"delta":      map[string]any{target.Key: value},
		"namespace":  req.Namespace,
	})
	s.hub.broadcastTargeted(req.Namespace, string(pushPayload), targeting.Selector{})
	s.hub.notifyWatchers(target.Key, req.Namespace, string(pushPayload))

	s.auditWriter.Write(r.Context(), audit.Entry{
		EventType:   audit.EventRollback,
		Key:         target.Key,
		Namespace:   req.Namespace,
		OldValue:    nil,
		NewValue:    target.Value,
		VersionID:   newVersionID,
		VectorClock: target.VectorClock,
		Actor:       req.Actor,
	})

	if err := f.Transition(fsm.StateRolledBack); err != nil {
		log.Warn().Err(err).Msg("fsm final transition failed")
	}

	log.Info().
		Str("version_id", newVersionID).
		Str("key", target.Key).
		Str("fsm", string(f.State())).
		Msg("rollback complete")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollbackResponse{
		Status:    "ROLLED_BACK",
		VersionID: newVersionID,
		Key:       target.Key,
		Namespace: req.Namespace,
	})
}