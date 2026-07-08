package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const (
	EventConfigWrite  = "CONFIG_WRITE"
	EventRollback     = "ROLLBACK"
	EventSchemaChange = "SCHEMA_CHANGE"
)

type Entry struct {
	EventType   string
	Key         string
	Namespace   string
	OldValue    []byte
	NewValue    []byte
	VersionID   string
	VectorClock []byte
	Actor       string
	Targeting   []byte
}

type Writer struct {
	db *sql.DB
}

func NewWriter(db *sql.DB) *Writer {
	return &Writer{db: db}
}

func (w *Writer) Write(ctx context.Context, e Entry) error {
	var oldValue, newValue, targeting interface{}

	if e.OldValue != nil {
		oldValue = json.RawMessage(e.OldValue)
	}
	if e.NewValue != nil {
		newValue = json.RawMessage(e.NewValue)
	}
	if e.Targeting != nil {
		targeting = json.RawMessage(e.Targeting)
	}

	_, err := w.db.ExecContext(ctx, `
		INSERT INTO audit_log (
			event_type, key, namespace,
			old_value, new_value,
			version_id, vector_clock,
			actor, targeting
		) VALUES (
			$1, $2, $3,
			$4, $5,
			$6, $7,
			$8, $9
		)`,
		e.EventType,
		e.Key,
		e.Namespace,
		oldValue,
		newValue,
		e.VersionID,
		json.RawMessage(e.VectorClock),
		e.Actor,
		targeting,
	)
	if err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return nil
}