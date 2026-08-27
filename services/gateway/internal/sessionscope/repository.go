// Package sessionscope persists the union of KBs used by each chat session.
package sessionscope

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("session-scope database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) ListKnowledgeBases(ctx context.Context, sessionID string) ([]string, error) {
	if err := validateID(sessionID, 128); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT knowledge_base_id
		FROM mindcreek.session_kb_scopes
		WHERE session_id = $1
		ORDER BY knowledge_base_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session KB scopes: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var kbID string
		if err := rows.Scan(&kbID); err != nil {
			return nil, fmt.Errorf("scan session KB scope: %w", err)
		}
		result = append(result, kbID)
	}
	return result, rows.Err()
}

func (r *Repository) RecordKnowledgeBases(ctx context.Context, sessionID string, kbIDs []string, at time.Time) error {
	if err := validateID(sessionID, 128); err != nil || at.IsZero() {
		return fmt.Errorf("invalid session scope")
	}
	unique := make(map[string]struct{}, len(kbIDs))
	for _, kbID := range kbIDs {
		if err := validateID(kbID, 36); err != nil {
			return fmt.Errorf("invalid knowledge-base scope")
		}
		unique[kbID] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session-scope update: %w", err)
	}
	defer tx.Rollback()
	for kbID := range unique {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO mindcreek.session_kb_scopes
				(session_id, knowledge_base_id, first_recorded_at, last_seen_at)
			VALUES ($1, $2, $3, $3)
			ON CONFLICT (session_id, knowledge_base_id) DO UPDATE
			SET last_seen_at = GREATEST(mindcreek.session_kb_scopes.last_seen_at, EXCLUDED.last_seen_at)`,
			sessionID, kbID, at.UTC()); err != nil {
			return fmt.Errorf("record session KB scope: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session-scope update: %w", err)
	}
	return nil
}

func validateID(value string, maxLength int) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxLength {
		return fmt.Errorf("invalid ID")
	}
	return nil
}
