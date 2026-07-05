package raft

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Role int

const (
	RoleFollower Role = iota
	RoleLeader
)

func (r Role) String() string {
	switch r {
	case RoleLeader:
		return "leader"
	default:
		return "follower"
	}
}

type Node struct {
	id       string
	db       *sql.DB
	role     Role
	term     int64
	mu       sync.RWMutex
	stopCh   chan struct{}
	leaderID string
}

func NewNode(db *sql.DB) *Node {
	return &Node{
		id:     os.Getenv("HERALD_NODE_ID"),
		db:     db,
		role:   RoleFollower,
		stopCh: make(chan struct{}),
	}
}

func (n *Node) ID() string {
	return n.id
}

func (n *Node) Role() Role {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.role
}

func (n *Node) Term() int64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.term
}

func (n *Node) LeaderID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.leaderID
}

func (n *Node) IsLeader() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.role == RoleLeader
}

func (n *Node) Start(ctx context.Context) {
	go n.electionLoop(ctx)
}

func (n *Node) Stop() {
	close(n.stopCh)
}

func (n *Node) electionLoop(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.tryAcquireLeadership(ctx)
		}
	}
}

func (n *Node) tryAcquireLeadership(ctx context.Context) {
	var acquired bool
	err := n.db.QueryRowContext(ctx,
		"SELECT pg_try_advisory_lock(42)",
	).Scan(&acquired)
	if err != nil {
		log.Error().Err(err).Msg("advisory lock query failed")
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if acquired {
		if n.role != RoleLeader {
			n.term++
			n.role = RoleLeader
			n.leaderID = n.id
			log.Info().
				Str("node_id", n.id).
				Int64("term", n.term).
				Msg("leader elected")
		}
	} else {
		if n.role == RoleLeader {
			n.role = RoleFollower
			log.Info().
				Str("node_id", n.id).
				Msg("lost leadership")
		}
	}
}