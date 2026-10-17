package api

import (
	"context"
	"math"
	"time"

	"github.com/AkZcH/herald/internal/metrics"
	"github.com/rs/zerolog/log"
)

const (
	// highest raft log index whose config version has been committed
	commitIndexQuery = `
		SELECT COALESCE(MAX(l.log_index), 0)
		FROM raft_log l
		JOIN config_versions c ON c.id = l.config_version_id
		WHERE c.status = 'COMMITTED'`

	staleAgentsQuery = `SELECT COUNT(*) FROM agent_registry WHERE status = 'STALE'`
)

func (s *Server) newMetrics() *metrics.Metrics {
	return metrics.New(metrics.Sources{
		RaftTerm:    func() float64 { return float64(s.raftNode.Term()) },
		CommitIndex: s.dbGauge(commitIndexQuery),
		StaleAgents: s.dbGauge(staleAgentsQuery),
	})
}

// dbGauge returns a scrape-time reader for a single-number query. On error it
// reports NaN, so Prometheus shows a gap instead of a misleading 0.
func (s *Server) dbGauge(query string) func() float64 {
	return func() float64 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		var v float64
		if err := s.db.QueryRowContext(ctx, query).Scan(&v); err != nil {
			log.Error().Err(err).Msg("metrics query failed")
			return math.NaN()
		}
		return v
	}
}