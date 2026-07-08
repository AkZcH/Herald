package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type ConfigVersion struct {
	ID              string
	Key             string
	Namespace       string
	Value           json.RawMessage
	PreviousID      *string
	VectorClock     json.RawMessage
	Status          string
	TargetingLabels json.RawMessage
	PushedBy        string
}

func (db *DB) InsertConfigVersion(ctx context.Context, cv ConfigVersion) (string, error) {
	id := uuid.New().String()

	valueJSON, err := json.Marshal(cv.Value)
	if err != nil {
		return "", fmt.Errorf("marshal value: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO config_versions (
			id, key, namespace, value, previous_id,
			vector_clock, status, targeting_labels, pushed_by
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9
		)`,
		id,
		cv.Key,
		cv.Namespace,
		valueJSON,
		cv.PreviousID,
		cv.VectorClock,
		cv.Status,
		cv.TargetingLabels,
		cv.PushedBy,
	)
	if err != nil {
		return "", fmt.Errorf("insert config version: %w", err)
	}

	return id, nil
}

func (db *DB) GetLatestConfigVersion(ctx context.Context, key, namespace string) (*ConfigVersion, error) {
	var cv ConfigVersion
	var previousID sql.NullString

	err := db.QueryRowContext(ctx, `
		SELECT id, key, namespace, value, previous_id,
		       vector_clock, status, pushed_by
		FROM config_versions
		WHERE key = $1 AND namespace = $2
		AND status = 'COMMITTED'
		ORDER BY created_at DESC
		LIMIT 1
	`, key, namespace).Scan(
		&cv.ID,
		&cv.Key,
		&cv.Namespace,
		&cv.Value,
		&previousID,
		&cv.VectorClock,
		&cv.Status,
		&cv.PushedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get config version: %w", err)
	}

	if previousID.Valid {
		cv.PreviousID = &previousID.String
	}

	return &cv, nil
}