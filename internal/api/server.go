package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

type Server struct {
	router *chi.Mux
	http   *http.Server
	nodeID string
	port   string
	db     *sql.DB
}

func NewServer(nodeID, port string, db *sql.DB) *Server {
	s := &Server{
		nodeID: nodeID,
		port:   port,
		db:     db,
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
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","node_id":"%s"}`, s.nodeID)
}

func (s *Server) Start() error {
	log.Info().
		Str("node_id", s.nodeID).
		Str("port", s.port).
		Msg("control plane starting")
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Str("node_id", s.nodeID).Msg("control plane shutting down")
	return s.http.Shutdown(ctx)
}