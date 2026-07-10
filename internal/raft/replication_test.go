package raft_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/AkZcH/herald/internal/raft"
	"github.com/AkZcH/herald/internal/store"
)

func getFreePort() (int, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func TestWriteCommittedAfterQuorum(t *testing.T) {
	dbURL := os.Getenv("HERALD_DB_URL")
	if dbURL == "" {
		t.Skip("HERALD_DB_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// spin up 3 nodes each with their own DB connection
	nodes := make([]*raft.Node, 3)
	ports := make([]int, 3)

	for i := range nodes {
		db, err := store.NewDB(dbURL)
		if err != nil {
			t.Fatalf("connect node %d: %v", i, err)
		}
		defer db.Close()

		port, err := getFreePort()
		if err != nil {
			t.Fatalf("get free port: %v", err)
		}
		ports[i] = port

		t.Setenv("HERALD_NODE_ID", fmt.Sprintf("node-%d", i+1))
		nodes[i] = raft.NewNode(db.DB)
	}

	// start gRPC servers for all nodes
	for i, node := range nodes {
		go func(n *raft.Node, port int) {
			if err := n.StartGRPCServer(fmt.Sprintf("%d", port)); err != nil {
				t.Logf("gRPC server error: %v", err)
			}
		}(node, ports[i])
	}

	time.Sleep(100 * time.Millisecond)

	// wire peers: each node knows about the other two
	for i, node := range nodes {
		for j, port := range ports {
			if i != j {
				node.AddPeer(fmt.Sprintf("node-%d", j+1), fmt.Sprintf("localhost:%d", port))
			}
		}
	}

	// start election on all nodes
	for _, node := range nodes {
		node.Start(ctx)
	}

	time.Sleep(600 * time.Millisecond)

	// find the leader
	var leader *raft.Node
	for _, node := range nodes {
		if node.IsLeader() {
			leader = node
			break
		}
	}

	if leader == nil {
		t.Fatal("no leader elected")
	}

	// append a log entry from the leader
	db, err := store.NewDB(dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	logIndex, err := leader.AppendToLog(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("append to log: %v", err)
	}

	entry := raft.LogEntry{
		Term:            leader.Term(),
		Index:           logIndex,
		ConfigVersionID: "00000000-0000-0000-0000-000000000000",
		Command:         "CONFIG_WRITE",
	}

	quorumReached, err := leader.BroadcastAppendEntries(ctx, entry)
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}

	if !quorumReached {
		t.Error("expected quorum to be reached with 3 nodes")
	}

	for _, node := range nodes {
		node.Stop()
	}
}