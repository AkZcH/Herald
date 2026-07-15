package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/AkZcH/herald/internal/audit"
	"github.com/AkZcH/herald/internal/raft"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

type Server struct {
	router      *chi.Mux
	http        *http.Server
	nodeID      string
	port        string
	db          *sql.DB
	raftNode    *raft.Node
	auditWriter *audit.Writer
	hub         *Hub
}

func NewServer(nodeID, port string, db *sql.DB, raftNode *raft.Node) *Server {
	s := &Server{
		nodeID:   nodeID,
		port:     port,
		db:       db,
		raftNode: raftNode,
		auditWriter: audit.NewWriter(db),
		hub: newHub(),
	}
	s.router = chi.NewRouter()
	s.setupMiddleware()
	s.setupRoutes()
	s.http = &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Recoverer)
	s.router.Use(zerologMiddleware)
}

func (s *Server) setupRoutes() {
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/cluster/status", s.handleClusterStatus)
	s.router.Post("/config", s.handleConfigWrite)
	s.router.Get("/config/{key}", s.handleConfigGet)
	s.router.Get("/config/diff", s.handleConfigDiff)
	s.router.Post("/agents/register", s.handleAgentRegister)
	s.router.Post("/heartbeat", s.handleHeartbeat)
	s.router.Get("/agents/stream", s.handleAgentSSE)
	s.router.Post("/ack", s.handleAck)
	s.router.Get("/config/{key}/watch", s.handleConfigWatch)
	s.router.Post("/config/rollback", s.handleRollback)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","node_id":"%s"}`, s.nodeID)
}

func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w,
		`{"node_id":"%s","role":"%s","term":%d,"leader_id":"%s"}`,
		s.nodeID,
		s.raftNode.Role().String(),
		s.raftNode.Term(),
		s.raftNode.LeaderID(),
	)
}

func (s *Server) Start() error {
	log.Info().Str("port", s.port).Msg("control plane starting")
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("control plane shutting down")
	return s.http.Shutdown(ctx)
}