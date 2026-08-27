// Package audit stores redacted product authorization and grant events.
package audit

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid audit event")

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeDenied  Outcome = "denied"
	OutcomeFailure Outcome = "failure"
)

type Event struct {
	ID              string          `json:"id"`
	TenantID        uint64          `json:"tenant_id"`
	KnowledgeBaseID string          `json:"knowledge_base_id"`
	ActorUserID     string          `json:"actor_user_id"`
	Action          string          `json:"action"`
	TargetType      string          `json:"target_type"`
	TargetID        string          `json:"target_id"`
	Outcome         Outcome         `json:"outcome"`
	ErrorCode       string          `json:"error_code,omitempty"`
	CorrelationID   string          `json:"correlation_id"`
	OldValue        json.RawMessage `json:"old_value,omitempty"`
	NewValue        json.RawMessage `json:"new_value,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

type Recorder interface {
	Record(context.Context, Event) error
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("audit database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Record(ctx context.Context, event Event) error {
	if event.ID == "" {
		id, err := newID()
		if err != nil {
			return fmt.Errorf("generate audit event ID: %w", err)
		}
		event.ID = id
	}
	if err := event.validate(); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.kb_access_audit_events
			(id, tenant_id, knowledge_base_id, actor_user_id, action, target_type, target_id,
			 outcome, error_code, correlation_id, old_value, new_value, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, $12, $13)`,
		event.ID, event.TenantID, event.KnowledgeBaseID, event.ActorUserID, event.Action,
		event.TargetType, event.TargetID, event.Outcome, event.ErrorCode, event.CorrelationID,
		nullableJSON(event.OldValue), nullableJSON(event.NewValue), event.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("record KB access audit event: %w", err)
	}
	return nil
}

func (event Event) validate() error {
	if strings.TrimSpace(event.ID) == "" || len(event.ID) > 36 || event.TenantID == 0 ||
		strings.TrimSpace(event.KnowledgeBaseID) == "" || len(event.KnowledgeBaseID) > 36 ||
		strings.TrimSpace(event.ActorUserID) == "" || len(event.ActorUserID) > 512 ||
		strings.TrimSpace(event.Action) == "" || len(event.Action) > 64 ||
		strings.TrimSpace(event.TargetType) == "" || len(event.TargetType) > 32 ||
		strings.TrimSpace(event.TargetID) == "" || len(event.TargetID) > 128 ||
		strings.TrimSpace(event.CorrelationID) == "" || len(event.CorrelationID) > 128 || event.CreatedAt.IsZero() {
		return ErrInvalid
	}
	if event.Outcome != OutcomeSuccess && event.Outcome != OutcomeDenied && event.Outcome != OutcomeFailure {
		return ErrInvalid
	}
	for _, value := range []json.RawMessage{event.OldValue, event.NewValue} {
		if len(value) > 16<<10 {
			return ErrInvalid
		}
		if len(value) > 0 {
			var object map[string]any
			if json.Unmarshal(value, &object) != nil || object == nil {
				return ErrInvalid
			}
		}
	}
	return nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
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
