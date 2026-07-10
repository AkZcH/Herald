package raft

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/AkZcH/herald/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Peer struct {
	ID      string
	Address string
	client  pb.HeraldRaftClient
}

func (n *Node) LoadPeers() {
	peersEnv := os.Getenv("HERALD_PEERS")
	if peersEnv == "" {
		return
	}

	for _, p := range strings.Split(peersEnv, ",") {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 {
			continue
		}
		n.peers = append(n.peers, &Peer{
			ID:      strings.TrimSpace(parts[0]),
			Address: strings.TrimSpace(parts[1]),
		})
		log.Info().Str("peer", parts[0]).Str("addr", parts[1]).Msg("peer registered")
	}
}

func (n *Node) connectPeer(peer *Peer) error {
	conn, err := grpc.NewClient(peer.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect peer %s: %w", peer.ID, err)
	}
	peer.client = pb.NewHeraldRaftClient(conn)
	return nil
}

func (n *Node) BroadcastAppendEntries(ctx context.Context, entry LogEntry) (bool, error) {
	if len(n.peers) == 0 {
		return true, nil
	}

	req := &pb.AppendEntriesRequest{
		Term:     n.term,
		LeaderId: n.id,
		Entries: []*pb.LogEntry{
			{
				Term:            entry.Term,
				Index:           entry.Index,
				ConfigVersionId: entry.ConfigVersionID,
				Command:         entry.Command,
			},
		},
	}

	successCount := 1
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range n.peers {
		wg.Add(1)
		go func(p *Peer) {
			defer wg.Done()

			if p.client == nil {
				if err := n.connectPeer(p); err != nil {
					log.Error().Str("peer", p.ID).Err(err).Msg("failed to connect peer")
					return
				}
			}

			peerCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()

			resp, err := p.client.AppendEntries(peerCtx, req)
			if err != nil {
				log.Warn().Str("peer", p.ID).Err(err).Msg("AppendEntries failed")
				return
			}

			if resp.Success {
				mu.Lock()
				successCount++
				mu.Unlock()
				log.Info().Str("peer", p.ID).Msg("AppendEntries acked")
			}
		}(peer)
	}

	wg.Wait()

	quorum := (len(n.peers)+1)/2 + 1
	return successCount >= quorum, nil
}