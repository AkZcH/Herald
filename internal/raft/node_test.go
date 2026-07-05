package raft_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AkZcH/herald/internal/raft"
	"github.com/AkZcH/herald/internal/store"
)

func TestExactlyOneLeader(t *testing.T) {
	dbURL := os.Getenv("HERALD_DB_URL")
	if dbURL == "" {
		t.Skip("HERALD_DB_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodes := make([]*raft.Node, 3)
	for i := range nodes {
		db, err := store.NewDB(dbURL)
		if err != nil {
			t.Fatalf("connect node %d: %v", i, err)
		}
		defer db.Close()

		t.Setenv("HERALD_NODE_ID", fmt.Sprintf("node-%d", i+1))
		nodes[i] = raft.NewNode(db.DB)
		nodes[i].Start(ctx)
	}

	time.Sleep(600 * time.Millisecond)

	leaderCount := 0
	for _, n := range nodes {
		if n.IsLeader() {
			leaderCount++
		}
	}

	if leaderCount != 1 {
		t.Errorf("expected exactly 1 leader, got %d", leaderCount)
	}

	for _, n := range nodes {
		n.Stop()
	}
}