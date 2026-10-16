package raft

import (
	"context"
	"fmt"
	"net"

	pb "github.com/AkZcH/herald/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type grpcServer struct {
	pb.UnimplementedHeraldRaftServer
	node *Node
}

func (s *grpcServer) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	s.node.mu.Lock()
	defer s.node.mu.Unlock()

	if req.Term < s.node.term {
		return &pb.AppendEntriesResponse{
			Term:    s.node.term,
			Success: false,
		}, nil
	}

	if req.Term > s.node.term {
		s.node.term = req.Term
		s.node.role = RoleFollower
		s.node.leaderID = req.LeaderId
	}

	for _, entry := range req.Entries {
		_, err := s.node.db.ExecContext(ctx, `
			INSERT INTO raft_log (term, log_index, node_id, config_version_id, command)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (log_index) DO NOTHING
		`, entry.Term, entry.Index, req.LeaderId, entry.ConfigVersionId, entry.Command)
		if err != nil {
			log.Error().Err(err).Msg("failed to append log entry")
			return &pb.AppendEntriesResponse{
				Term:    s.node.term,
				Success: false,
			}, nil
		}
	}

	return &pb.AppendEntriesResponse{
		Term:    s.node.term,
		Success: true,
	}, nil
}

func (s *grpcServer) RequestVote(ctx context.Context, req *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	s.node.mu.Lock()
	defer s.node.mu.Unlock()

	if req.Term < s.node.term {
		return &pb.RequestVoteResponse{
			Term:        s.node.term,
			VoteGranted: false,
		}, nil
	}

	if req.Term > s.node.term {
		s.node.term = req.Term
		s.node.role = RoleFollower
	}

	return &pb.RequestVoteResponse{
		Term:        s.node.term,
		VoteGranted: true,
	}, nil
}

func (n *Node) StartGRPCServer(port string) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	srv := grpc.NewServer()
	pb.RegisterHeraldRaftServer(srv, &grpcServer{node: n})

	log.Info().Str("port", port).Msg("raft gRPC server starting")
	return srv.Serve(lis)
}