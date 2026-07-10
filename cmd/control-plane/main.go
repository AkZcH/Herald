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
	"github.com/AkZcH/herald/internal/raft"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	raftNode := raft.NewNode(db.DB)
	raftNode.Start(ctx)
	log.Info().Msg("raft node started")

	grpcPort := os.Getenv("HERALD_GRPC_PORT")
if grpcPort == "" {
    grpcPort = "7800"
}

go func() {
    if err := raftNode.StartGRPCServer(grpcPort); err != nil {
        log.Fatal().Err(err).Msg("gRPC server failed")
    }
}()

log.Info().Str("grpc_port", grpcPort).Msg("raft gRPC server started")

	srv := api.NewServer(nodeID, port, db.DB, raftNode)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-quit
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatal().Err(err).Msg("shutdown failed")
	}
}