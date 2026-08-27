package grant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type Store interface {
	Create(context.Context, Grant) (Grant, error)
	Get(context.Context, string) (Grant, error)
	FindCurrentBySubject(context.Context, string, SubjectType, string) (Grant, error)
	ListCurrentByKB(context.Context, string) ([]Grant, error)
	EffectiveUserGrant(context.Context, string, string, time.Time) (Grant, error)
	Update(context.Context, string, int64, Permission, *time.Time, string, time.Time) (Grant, error)
	Revoke(context.Context, string, int64, string, time.Time) (Grant, error)
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("grant database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Create(ctx context.Context, candidate Grant) (Grant, error) {
	if err := candidate.validate(); err != nil {
		return Grant{}, err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.kb_access_grants
			(id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
			 created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		candidate.ID, candidate.KnowledgeBaseID, candidate.SubjectType, candidate.SubjectID,
		candidate.Permission, candidate.GrantedBy, candidate.CreatedAt, candidate.UpdatedAt,
		candidate.ExpiresAt, candidate.RevokedAt, candidate.Revision, candidate.LastAuditCorrelationID)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return Grant{}, ErrConflict
		}
		return Grant{}, fmt.Errorf("create knowledge-base grant: %w", err)
	}
	return candidate, nil
}

func (r *Repository) Get(ctx context.Context, id string) (Grant, error) {
	return scanGrant(r.db.QueryRowContext(ctx, `
		SELECT id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
		       created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id
		FROM mindcreek.kb_access_grants
		WHERE id = $1`, id))
}

func (r *Repository) FindCurrentBySubject(ctx context.Context, kbID string, subjectType SubjectType, subjectID string) (Grant, error) {
	return scanGrant(r.db.QueryRowContext(ctx, `
		SELECT id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
		       created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id
		FROM mindcreek.kb_access_grants
		WHERE knowledge_base_id = $1 AND subject_type = $2 AND subject_id = $3 AND revoked_at IS NULL`,
		kbID, subjectType, subjectID))
}

func (r *Repository) ListCurrentByKB(ctx context.Context, kbID string) ([]Grant, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
		       created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id
		FROM mindcreek.kb_access_grants
		WHERE knowledge_base_id = $1 AND revoked_at IS NULL
		ORDER BY created_at, id`, kbID)
	if err != nil {
		return nil, fmt.Errorf("list knowledge-base grants: %w", err)
	}
	defer rows.Close()
	result := make([]Grant, 0)
	for rows.Next() {
		item, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) EffectiveUserGrant(ctx context.Context, kbID, userID string, at time.Time) (Grant, error) {
	return scanGrant(r.db.QueryRowContext(ctx, `
		SELECT id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
		       created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id
		FROM mindcreek.kb_access_grants
		WHERE knowledge_base_id = $1 AND subject_type = 'user' AND subject_id = $2
		  AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > $3)
		ORDER BY CASE permission WHEN 'editor' THEN 0 ELSE 1 END
		LIMIT 1`, kbID, userID, at))
}

func (r *Repository) Update(ctx context.Context, id string, expectedRevision int64, permission Permission, expiresAt *time.Time, correlationID string, at time.Time) (Grant, error) {
	if id == "" || expectedRevision < 1 || !permission.Valid() || correlationID == "" || at.IsZero() {
		return Grant{}, ErrInvalid
	}
	result, err := scanGrant(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_access_grants
		SET permission = $3, expires_at = $4, updated_at = $5,
		    revision = revision + 1, last_audit_correlation_id = $6
		WHERE id = $1 AND revision = $2 AND revoked_at IS NULL
		RETURNING id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
		          created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id`,
		id, expectedRevision, permission, expiresAt, at, correlationID))
	if errors.Is(err, ErrNotFound) {
		return Grant{}, ErrRevisionConflict
	}
	return result, err
}

func (r *Repository) Revoke(ctx context.Context, id string, expectedRevision int64, correlationID string, at time.Time) (Grant, error) {
	if id == "" || expectedRevision < 1 || correlationID == "" || at.IsZero() {
		return Grant{}, ErrInvalid
	}
	result, err := scanGrant(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_access_grants
		SET revoked_at = $3, updated_at = $3, revision = revision + 1,
		    last_audit_correlation_id = $4
		WHERE id = $1 AND revision = $2 AND revoked_at IS NULL
		RETURNING id, knowledge_base_id, subject_type, subject_id, permission, granted_by,
		          created_at, updated_at, expires_at, revoked_at, revision, last_audit_correlation_id`,
		id, expectedRevision, at, correlationID))
	if errors.Is(err, ErrNotFound) {
		return Grant{}, ErrRevisionConflict
	}
	return result, err
}

type scanner interface {
	Scan(...any) error
}

func scanGrant(row scanner) (Grant, error) {
	var result Grant
	var subjectType string
	var permission string
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	err := row.Scan(
		&result.ID, &result.KnowledgeBaseID, &subjectType, &result.SubjectID, &permission,
		&result.GrantedBy, &result.CreatedAt, &result.UpdatedAt, &expiresAt, &revokedAt,
		&result.Revision, &result.LastAuditCorrelationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Grant{}, ErrNotFound
	}
	if err != nil {
		return Grant{}, fmt.Errorf("scan knowledge-base grant: %w", err)
	}
	result.SubjectType = SubjectType(subjectType)
	result.Permission = Permission(permission)
	if expiresAt.Valid {
		value := expiresAt.Time
		result.ExpiresAt = &value
	}
	if revokedAt.Valid {
		value := revokedAt.Time
		result.RevokedAt = &value
	}
	if err := result.validate(); err != nil {
		return Grant{}, fmt.Errorf("invalid stored knowledge-base grant: %w", err)
	}
	return result, nil
}
