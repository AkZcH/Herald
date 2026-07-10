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
	id            string
	db            *sql.DB
	electionConn  *sql.Conn
	heartbeatConn *sql.Conn
	role          Role
	term          int64
	mu            sync.RWMutex
	stopCh        chan struct{}
	leaderID      string
	peers         []*Peer
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
	electionConn, err := n.db.Conn(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get election connection")
	}
	n.electionConn = electionConn

	heartbeatConn, err := n.db.Conn(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get heartbeat connection")
	}
	n.heartbeatConn = heartbeatConn

	go n.electionLoop(ctx)
	go n.startHeartbeat(ctx)
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
	err := n.electionConn.QueryRowContext(ctx,
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
			log.Info().Int64("term", n.term).Msg("leader elected")
		}
	} else {
		if n.role == RoleLeader {
			n.role = RoleFollower
			log.Info().Msg("lost leadership")
		}
	}
}

func (n *Node) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopCh:
			return
		case <-ticker.C:
			if n.IsLeader() {
				n.refreshLock(ctx)
			}
		}
	}
}

func (n *Node) refreshLock(ctx context.Context) {
	var held bool
	err := n.heartbeatConn.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM pg_locks
			WHERE locktype='advisory'
			AND objid=42
			AND granted=true
		)`,
	).Scan(&held)
	if err != nil {
		log.Error().Err(err).Msg("heartbeat check failed")
		return
	}

	if !held {
		n.mu.Lock()
		n.role = RoleFollower
		n.leaderID = ""
		n.mu.Unlock()
		log.Warn().Msg("lost advisory lock, stepping down")
	}
}