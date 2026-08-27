package space

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrIdempotencyConflict = errors.New("idempotency key was reused with a different request")

type RequestStatus string

const (
	StatusPending RequestStatus = "pending"
	StatusReady   RequestStatus = "ready"
	StatusFailed  RequestStatus = "failed"
)

type CreationRequest struct {
	TenantID       uint64
	OwnerUserID    string
	IdempotencyKey string
	RequestHash    string
	UpstreamKBID   string
	ProductMode    string
	IndexProfile   string
	Status         RequestStatus
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RequestStore interface {
	Claim(context.Context, CreationRequest) (CreationRequest, bool, error)
	Complete(context.Context, CreationRequest) error
	Fail(context.Context, CreationRequest, string) error
}

type RequestRepository struct {
	db *sql.DB
}

func NewRequestRepository(db *sql.DB) (*RequestRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("creation request database is required")
	}
	return &RequestRepository{db: db}, nil
}

func (r *RequestRepository) Claim(ctx context.Context, candidate CreationRequest) (CreationRequest, bool, error) {
	if err := validateCreationRequest(candidate); err != nil {
		return CreationRequest{}, false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CreationRequest{}, false, fmt.Errorf("begin creation request claim: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO mindcreek.knowledge_space_requests
			(tenant_id, owner_user_id, idempotency_key, request_hash, upstream_kb_id, product_mode, index_profile)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, owner_user_id, idempotency_key) DO NOTHING`,
		candidate.TenantID, candidate.OwnerUserID, candidate.IdempotencyKey, candidate.RequestHash,
		candidate.UpstreamKBID, candidate.ProductMode, candidate.IndexProfile)
	if err != nil {
		return CreationRequest{}, false, fmt.Errorf("claim creation request: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return CreationRequest{}, false, fmt.Errorf("inspect creation request claim: %w", err)
	}
	stored, err := scanCreationRequest(tx.QueryRowContext(ctx, `
		SELECT tenant_id, owner_user_id, idempotency_key, request_hash, upstream_kb_id,
		       product_mode, index_profile, status, last_error, created_at, updated_at
		FROM mindcreek.knowledge_space_requests
		WHERE tenant_id = $1 AND owner_user_id = $2 AND idempotency_key = $3
		FOR UPDATE`, candidate.TenantID, candidate.OwnerUserID, candidate.IdempotencyKey))
	if err != nil {
		return CreationRequest{}, false, fmt.Errorf("read creation request claim: %w", err)
	}
	if stored.RequestHash != candidate.RequestHash || stored.ProductMode != candidate.ProductMode || stored.IndexProfile != candidate.IndexProfile {
		return CreationRequest{}, false, ErrIdempotencyConflict
	}
	if err := tx.Commit(); err != nil {
		return CreationRequest{}, false, fmt.Errorf("commit creation request claim: %w", err)
	}
	return stored, rows == 1, nil
}

func (r *RequestRepository) Complete(ctx context.Context, request CreationRequest) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mindcreek.knowledge_space_requests
		SET status = 'ready', last_error = '', updated_at = now()
		WHERE tenant_id = $1 AND owner_user_id = $2 AND idempotency_key = $3
		  AND request_hash = $4 AND upstream_kb_id = $5`,
		request.TenantID, request.OwnerUserID, request.IdempotencyKey, request.RequestHash, request.UpstreamKBID)
	if err != nil {
		return fmt.Errorf("complete creation request: %w", err)
	}
	return requireOneRow(result, "complete creation request")
}

func (r *RequestRepository) Fail(ctx context.Context, request CreationRequest, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE mindcreek.knowledge_space_requests
		SET status = 'failed', last_error = $6, updated_at = now()
		WHERE tenant_id = $1 AND owner_user_id = $2 AND idempotency_key = $3
		  AND request_hash = $4 AND upstream_kb_id = $5`,
		request.TenantID, request.OwnerUserID, request.IdempotencyKey, request.RequestHash, request.UpstreamKBID, message)
	if err != nil {
		return fmt.Errorf("fail creation request: %w", err)
	}
	return requireOneRow(result, "fail creation request")
}

type rowScanner interface {
	Scan(...any) error
}

func scanCreationRequest(row rowScanner) (CreationRequest, error) {
	var result CreationRequest
	err := row.Scan(
		&result.TenantID, &result.OwnerUserID, &result.IdempotencyKey, &result.RequestHash,
		&result.UpstreamKBID, &result.ProductMode, &result.IndexProfile, &result.Status,
		&result.LastError, &result.CreatedAt, &result.UpdatedAt,
	)
	return result, err
}

func validateCreationRequest(request CreationRequest) error {
	if request.TenantID == 0 || strings.TrimSpace(request.OwnerUserID) == "" ||
		strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 128 ||
		len(request.RequestHash) != 64 || strings.TrimSpace(request.UpstreamKBID) == "" {
		return fmt.Errorf("creation request identity, key, hash, and upstream KB ID are required")
	}
	return nil
}

func requireOneRow(result sql.Result, operation string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("%s did not match its request ledger row", operation)
	}
	return nil
}
