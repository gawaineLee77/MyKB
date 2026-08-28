// Package revision maintains monotonic KB content revisions and sanitized activity.
package revision

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid content revision request")
	ErrNotFound = errors.New("content revision not found")
)

type Activity struct {
	ID              string    `json:"id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	ActorID         string    `json:"actor_id,omitempty"`
	EventType       string    `json:"event_type"`
	ContentRevision int64     `json:"content_revision"`
	Summary         string    `json:"summary"`
	CorrelationID   string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("revision database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Ensure(ctx context.Context, kbID, actorID, eventType, summary, correlationID string, at time.Time) (int64, error) {
	if err := validate(kbID, actorID, eventType, summary, correlationID, at); err != nil {
		return 0, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin content revision: %w", err)
	}
	defer tx.Rollback()
	var revision int64
	var inserted bool
	err = tx.QueryRowContext(ctx, `
		INSERT INTO mindcreek.kb_content_revisions (knowledge_base_id, content_revision, updated_at)
		VALUES ($1, 1, $2)
		ON CONFLICT (knowledge_base_id) DO UPDATE SET knowledge_base_id=EXCLUDED.knowledge_base_id
		RETURNING content_revision, (xmax = 0)`, kbID, at.UTC()).Scan(&revision, &inserted)
	if err != nil {
		return 0, fmt.Errorf("ensure content revision: %w", err)
	}
	if inserted {
		if err := insertActivity(ctx, tx, kbID, actorID, eventType, summary, correlationID, revision, at); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit content revision: %w", err)
	}
	return revision, nil
}

func (r *Repository) Current(ctx context.Context, kbID string) (int64, error) {
	if strings.TrimSpace(kbID) == "" {
		return 0, ErrInvalid
	}
	var result int64
	err := r.db.QueryRowContext(ctx, `SELECT content_revision FROM mindcreek.kb_content_revisions WHERE knowledge_base_id=$1`, kbID).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("read content revision: %w", err)
	}
	return result, nil
}

func (r *Repository) Increment(ctx context.Context, kbID, actorID, eventType, summary, correlationID string, at time.Time) (int64, error) {
	if err := validate(kbID, actorID, eventType, summary, correlationID, at); err != nil {
		return 0, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin content revision: %w", err)
	}
	defer tx.Rollback()
	var result int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO mindcreek.kb_content_revisions (knowledge_base_id, content_revision, updated_at)
		VALUES ($1, 1, $2)
		ON CONFLICT (knowledge_base_id) DO UPDATE
		SET content_revision=mindcreek.kb_content_revisions.content_revision+1, updated_at=EXCLUDED.updated_at
		RETURNING content_revision`, kbID, at.UTC()).Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("increment content revision: %w", err)
	}
	if err := insertActivity(ctx, tx, kbID, actorID, eventType, summary, correlationID, result, at); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit content revision: %w", err)
	}
	return result, nil
}

func (r *Repository) ListActivity(ctx context.Context, kbID string, limit int) ([]Activity, error) {
	if strings.TrimSpace(kbID) == "" || limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, knowledge_base_id, COALESCE(actor_id,''), event_type, content_revision, summary, correlation_id, created_at
		FROM mindcreek.kb_activity_events WHERE knowledge_base_id=$1
		ORDER BY content_revision DESC, created_at DESC LIMIT $2`, kbID, limit)
	if err != nil {
		return nil, fmt.Errorf("list KB activity: %w", err)
	}
	defer rows.Close()
	result := make([]Activity, 0)
	for rows.Next() {
		var item Activity
		if err := rows.Scan(&item.ID, &item.KnowledgeBaseID, &item.ActorID, &item.EventType,
			&item.ContentRevision, &item.Summary, &item.CorrelationID, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan KB activity: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func validate(kbID, actorID, eventType, summary, correlationID string, at time.Time) error {
	if strings.TrimSpace(kbID) == "" || len(kbID) > 36 || len(actorID) > 512 ||
		strings.TrimSpace(eventType) == "" || len(eventType) > 64 || len([]rune(summary)) > 500 ||
		strings.TrimSpace(correlationID) == "" || len(correlationID) > 128 || at.IsZero() {
		return ErrInvalid
	}
	return nil
}

func insertActivity(ctx context.Context, tx *sql.Tx, kbID, actorID, eventType, summary, correlationID string, value int64, at time.Time) error {
	id, err := newID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO mindcreek.kb_activity_events
			(id, knowledge_base_id, actor_id, event_type, content_revision, summary, correlation_id, created_at)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8)`, id, kbID, actorID, eventType, value,
		summary, correlationID, at.UTC())
	if err != nil {
		return fmt.Errorf("record KB activity: %w", err)
	}
	return nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
