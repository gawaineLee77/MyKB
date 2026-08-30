// Package agentaudit records redacted web-agent and MCP operation metadata.
package agentaudit

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxScope = 64

var ErrInvalid = errors.New("invalid agent operation audit event")

type ClientKind string

const (
	ClientWeb ClientKind = "web"
	ClientMCP ClientKind = "mcp"
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeDenied  Outcome = "denied"
	OutcomeFailure Outcome = "failure"
)

type Event struct {
	ID               string
	TenantID         uint64
	ActorUserID      string
	ClientKind       ClientKind
	Operation        string
	KnowledgeBaseIDs []string
	Outcome          Outcome
	ErrorCode        string
	CorrelationID    string
	Duration         time.Duration
	CreatedAt        time.Time
}

type Recorder interface {
	Record(context.Context, Event) error
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("agent audit database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Record(ctx context.Context, event Event) error {
	if event.ID == "" {
		id, err := newID()
		if err != nil {
			return fmt.Errorf("generate agent audit ID: %w", err)
		}
		event.ID = id
	}
	if err := event.validate(); err != nil {
		return err
	}
	scope := append(make([]string, 0, len(event.KnowledgeBaseIDs)), event.KnowledgeBaseIDs...)
	sort.Strings(scope)
	encoded, err := json.Marshal(scope)
	if err != nil {
		return ErrInvalid
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.agent_operation_audit_events
			(id, tenant_id, actor_user_id, client_kind, operation, knowledge_base_ids,
			 outcome, error_code, correlation_id, duration_ms, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, $11)`,
		event.ID, event.TenantID, event.ActorUserID, event.ClientKind, event.Operation, encoded,
		event.Outcome, event.ErrorCode, event.CorrelationID, event.Duration.Milliseconds(), event.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("record agent operation audit event: %w", err)
	}
	return nil
}

func (event Event) validate() error {
	if strings.TrimSpace(event.ID) == "" || len(event.ID) > 36 || event.TenantID == 0 ||
		strings.TrimSpace(event.ActorUserID) == "" || len(event.ActorUserID) > 512 ||
		strings.TrimSpace(event.Operation) == "" || len(event.Operation) > 64 ||
		strings.TrimSpace(event.CorrelationID) == "" || len(event.CorrelationID) > 128 ||
		event.Duration < 0 || event.CreatedAt.IsZero() || len(event.KnowledgeBaseIDs) > maxScope {
		return ErrInvalid
	}
	if event.ClientKind != ClientWeb && event.ClientKind != ClientMCP {
		return ErrInvalid
	}
	if event.Outcome != OutcomeSuccess && event.Outcome != OutcomeDenied && event.Outcome != OutcomeFailure {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(event.KnowledgeBaseIDs))
	for _, id := range event.KnowledgeBaseIDs {
		if strings.TrimSpace(id) == "" || len(id) > 128 || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
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
