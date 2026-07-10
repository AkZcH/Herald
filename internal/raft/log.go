package raft

import (
	"context"
	"database/sql"
	"fmt"
)

type LogEntry struct {
	Term            int64
	Index           int64
	NodeID          string
	ConfigVersionID string
	Command         string
}

func (n *Node) AppendToLog(ctx context.Context, configVersionID string) (int64, error) {
	n.mu.RLock()
	term := n.term
	n.mu.RUnlock()

	var maxIndex int64
	err := n.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(log_index), 0) FROM raft_log",
	).Scan(&maxIndex)
	if err != nil {
		return 0, fmt.Errorf("get max index: %w", err)
	}

	nextIndex := maxIndex + 1

	_, err = n.db.ExecContext(ctx, `
		INSERT INTO raft_log (term, log_index, node_id, config_version_id, command)
		VALUES ($1, $2, $3, $4, $5)
	`, term, nextIndex, n.id, configVersionID, "CONFIG_WRITE")
	if err != nil {
		return 0, fmt.Errorf("append log entry: %w", err)
	}

	return nextIndex, nil
}

func (n *Node) GetLogEntry(ctx context.Context, index int64) (*LogEntry, error) {
	var entry LogEntry
	var configVersionID sql.NullString

	err := n.db.QueryRowContext(ctx, `
		SELECT term, log_index, node_id, config_version_id, command
		FROM raft_log WHERE log_index = $1
	`, index).Scan(
		&entry.Term,
		&entry.Index,
		&entry.NodeID,
		&configVersionID,
		&entry.Command,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get log entry: %w", err)
	}

	if configVersionID.Valid {
		entry.ConfigVersionID = configVersionID.String
	}

	return &entry, nil
}

func (n *Node) CommitVersion(ctx context.Context, configVersionID string) error {
	_, err := n.db.ExecContext(ctx, `
		UPDATE config_versions SET status = 'COMMITTED'
		WHERE id = $1
	`, configVersionID)
	if err != nil {
		return fmt.Errorf("commit version: %w", err)
	}
	return nil
}