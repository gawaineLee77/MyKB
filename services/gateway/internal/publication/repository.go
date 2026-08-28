package publication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type Store interface {
	Create(context.Context, Publication) (Publication, error)
	Get(context.Context, string) (Publication, error)
	GetByKB(context.Context, string) (Publication, error)
	GetPublishedByKB(context.Context, string) (Publication, error)
	ListPublished(context.Context) ([]Publication, error)
	Update(context.Context, Publication, int64) (Publication, error)
	Unpublish(context.Context, string, int64, string, time.Time) (Publication, error)
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("publication database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Create(ctx context.Context, candidate Publication) (Publication, error) {
	if err := candidate.Validate(); err != nil || !candidate.Published() {
		return Publication{}, ErrInvalid
	}
	tags, audience, err := encodeMetadata(candidate)
	if err != nil {
		return Publication{}, err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.kb_publications
			(id, knowledge_base_id, publisher_id, publisher_tenant_id, title, description, tags,
			 usage_guidance, audience_type, audience_config, access_mode, status, published_revision,
			 created_at, published_at, unpublished_at, updated_at, row_version, last_audit_correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		candidate.ID, candidate.KnowledgeBaseID, candidate.PublisherID, candidate.PublisherTenantID,
		candidate.Title, candidate.Description, tags, candidate.UsageGuidance, candidate.Audience.Type,
		audience, candidate.AccessMode, candidate.Status, candidate.PublishedRevision, candidate.CreatedAt,
		candidate.PublishedAt, candidate.UnpublishedAt, candidate.UpdatedAt, candidate.RowVersion,
		candidate.LastAuditCorrelationID)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return Publication{}, ErrConflict
		}
		return Publication{}, fmt.Errorf("create publication: %w", err)
	}
	return candidate, nil
}

func (r *Repository) Get(ctx context.Context, id string) (Publication, error) {
	return scanPublication(r.db.QueryRowContext(ctx, publicationSelect+` WHERE id = $1`, id))
}

func (r *Repository) GetByKB(ctx context.Context, kbID string) (Publication, error) {
	return scanPublication(r.db.QueryRowContext(ctx, publicationSelect+` WHERE knowledge_base_id = $1`, kbID))
}

func (r *Repository) GetPublishedByKB(ctx context.Context, kbID string) (Publication, error) {
	return scanPublication(r.db.QueryRowContext(ctx, publicationSelect+` WHERE knowledge_base_id = $1 AND status = 'published'`, kbID))
}

func (r *Repository) ListPublished(ctx context.Context) ([]Publication, error) {
	rows, err := r.db.QueryContext(ctx, publicationSelect+` WHERE status = 'published' ORDER BY updated_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list publications: %w", err)
	}
	defer rows.Close()
	result := make([]Publication, 0)
	for rows.Next() {
		item, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) Update(ctx context.Context, candidate Publication, expected int64) (Publication, error) {
	if err := candidate.Validate(); err != nil || !candidate.Published() || expected < 1 {
		return Publication{}, ErrInvalid
	}
	tags, audience, err := encodeMetadata(candidate)
	if err != nil {
		return Publication{}, err
	}
	result, err := scanPublication(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_publications
		SET title=$3, description=$4, tags=$5, usage_guidance=$6, audience_type=$7,
		    audience_config=$8, access_mode=$9, status='published', published_revision=$10,
		    published_at=$11, unpublished_at=NULL, updated_at=$12, row_version=row_version+1,
		    last_audit_correlation_id=$13
		WHERE id=$1 AND row_version=$2
		RETURNING id, knowledge_base_id, publisher_id, publisher_tenant_id, title, description, tags,
		          usage_guidance, audience_type, audience_config, access_mode, status, published_revision,
		          created_at, published_at, unpublished_at, updated_at, row_version, last_audit_correlation_id`,
		candidate.ID, expected, candidate.Title, candidate.Description, tags, candidate.UsageGuidance,
		candidate.Audience.Type, audience, candidate.AccessMode, candidate.PublishedRevision,
		candidate.PublishedAt, candidate.UpdatedAt, candidate.LastAuditCorrelationID))
	if errors.Is(err, ErrNotFound) {
		return Publication{}, ErrRevisionConflict
	}
	return result, err
}

func (r *Repository) Unpublish(ctx context.Context, id string, expected int64, correlationID string, at time.Time) (Publication, error) {
	if id == "" || expected < 1 || correlationID == "" || at.IsZero() {
		return Publication{}, ErrInvalid
	}
	result, err := scanPublication(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_publications
		SET status='unpublished', unpublished_at=$3, updated_at=$3, row_version=row_version+1,
		    last_audit_correlation_id=$4
		WHERE id=$1 AND row_version=$2 AND status='published'
		RETURNING id, knowledge_base_id, publisher_id, publisher_tenant_id, title, description, tags,
		          usage_guidance, audience_type, audience_config, access_mode, status, published_revision,
		          created_at, published_at, unpublished_at, updated_at, row_version, last_audit_correlation_id`,
		id, expected, at, correlationID))
	if errors.Is(err, ErrNotFound) {
		return Publication{}, ErrRevisionConflict
	}
	return result, err
}

const publicationSelect = `SELECT id, knowledge_base_id, publisher_id, publisher_tenant_id, title, description, tags,
	usage_guidance, audience_type, audience_config, access_mode, status, published_revision,
	created_at, published_at, unpublished_at, updated_at, row_version, last_audit_correlation_id
	FROM mindcreek.kb_publications`

type scanner interface{ Scan(...any) error }

func scanPublication(row scanner) (Publication, error) {
	var result Publication
	var tags, audience []byte
	var audienceType, accessMode, status string
	var unpublished sql.NullTime
	err := row.Scan(&result.ID, &result.KnowledgeBaseID, &result.PublisherID, &result.PublisherTenantID,
		&result.Title, &result.Description, &tags, &result.UsageGuidance, &audienceType, &audience,
		&accessMode, &status, &result.PublishedRevision, &result.CreatedAt, &result.PublishedAt,
		&unpublished, &result.UpdatedAt, &result.RowVersion, &result.LastAuditCorrelationID)
	if errors.Is(err, sql.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if err != nil {
		return Publication{}, fmt.Errorf("scan publication: %w", err)
	}
	result.Audience.Type = AudienceType(audienceType)
	result.AccessMode = AccessMode(accessMode)
	result.Status = Status(status)
	if unpublished.Valid {
		value := unpublished.Time
		result.UnpublishedAt = &value
	}
	if json.Unmarshal(tags, &result.Tags) != nil || json.Unmarshal(audience, &result.Audience) != nil {
		return Publication{}, fmt.Errorf("scan publication: %w", ErrInvalid)
	}
	if err := result.Validate(); err != nil {
		return Publication{}, fmt.Errorf("invalid stored publication: %w", err)
	}
	return result, nil
}

func encodeMetadata(candidate Publication) ([]byte, []byte, error) {
	tags, err := json.Marshal(candidate.Tags)
	if err != nil {
		return nil, nil, err
	}
	audience, err := json.Marshal(candidate.Audience)
	if err != nil {
		return nil, nil, err
	}
	return tags, audience, nil
}
