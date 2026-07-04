package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AkZcH/herald/internal/api"
	"github.com/AkZcH/herald/internal/store"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	nodeID := os.Getenv("HERALD_NODE_ID")
	if nodeID == "" {
		nodeID = "node-1"
	}

	port := os.Getenv("HERALD_PORT")
	if port == "" {
		port = "7700"
	}

	dbURL := os.Getenv("HERALD_DB_URL")
	if dbURL == "" {
		log.Fatal().Msg("HERALD_DB_URL is required")
	}

	log.Logger = log.With().Str("node_id", nodeID).Logger()

	db, err := store.NewDB(dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	log.Info().Msg("database connected")

	srv := api.NewServer(nodeID, port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("shutdown failed")
	}
}