// Package profile stores product behavior without changing WeKnora tables.
package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("knowledge-base profile not found")
	ErrConflict = errors.New("knowledge-base profile already exists")
)

type ProductMode string

const (
	ModePersonalNotes ProductMode = "personal_notes"
	ModeRAG           ProductMode = "rag"
	ModeOntology      ProductMode = "ontology"
)

type AccessPolicy string

const (
	PolicyOwnerOnly AccessPolicy = "owner_only"
	PolicyUpstream  AccessPolicy = "upstream"
)

type Profile struct {
	UpstreamKBID        string          `json:"upstream_kb_id"`
	TenantID            uint64          `json:"tenant_id"`
	OwnerUserID         string          `json:"owner_user_id"`
	ProductMode         ProductMode     `json:"product_mode"`
	SchemaVersion       int             `json:"schema_version"`
	AccessPolicy        AccessPolicy    `json:"access_policy"`
	IndexProfile        string          `json:"index_profile"`
	IndexProfileVersion int             `json:"index_profile_version"`
	EffectiveConfig     json.RawMessage `json:"effective_config"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type Store interface {
	Create(context.Context, Profile) (Profile, error)
	Get(context.Context, string) (Profile, error)
	ForbiddenPersonalNoteIDs(context.Context, string) (map[string]struct{}, error)
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("profile database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Create(ctx context.Context, candidate Profile) (Profile, error) {
	if err := candidate.validate(); err != nil {
		return Profile{}, err
	}
	if candidate.SchemaVersion == 0 {
		candidate.SchemaVersion = 1
	}
	now := time.Now().UTC()
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = now
	}
	if candidate.UpdatedAt.IsZero() {
		candidate.UpdatedAt = candidate.CreatedAt
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.kb_profiles
			(upstream_kb_id, tenant_id, owner_user_id, product_mode, schema_version, access_policy,
			 index_profile, index_profile_version, effective_config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		candidate.UpstreamKBID, candidate.TenantID, candidate.OwnerUserID, candidate.ProductMode,
		candidate.SchemaVersion, candidate.AccessPolicy, candidate.IndexProfile, candidate.IndexProfileVersion,
		[]byte(candidate.EffectiveConfig), candidate.CreatedAt, candidate.UpdatedAt)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return Profile{}, ErrConflict
		}
		return Profile{}, fmt.Errorf("create knowledge-base profile: %w", err)
	}
	return candidate, nil
}

func (r *Repository) Get(ctx context.Context, upstreamKBID string) (Profile, error) {
	if strings.TrimSpace(upstreamKBID) == "" {
		return Profile{}, ErrNotFound
	}
	var result Profile
	err := r.db.QueryRowContext(ctx, `
		SELECT upstream_kb_id, tenant_id, owner_user_id, product_mode, schema_version, access_policy,
		       index_profile, index_profile_version, effective_config, created_at, updated_at
		FROM mindcreek.kb_profiles
		WHERE upstream_kb_id = $1`, upstreamKBID).Scan(
		&result.UpstreamKBID, &result.TenantID, &result.OwnerUserID, &result.ProductMode,
		&result.SchemaVersion, &result.AccessPolicy, &result.IndexProfile, &result.IndexProfileVersion,
		&result.EffectiveConfig, &result.CreatedAt, &result.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("get knowledge-base profile: %w", err)
	}
	return result, nil
}

func (r *Repository) ForbiddenPersonalNoteIDs(ctx context.Context, ownerUserID string) (map[string]struct{}, error) {
	if strings.TrimSpace(ownerUserID) == "" {
		return nil, fmt.Errorf("owner user ID is required")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT upstream_kb_id
		FROM mindcreek.kb_profiles
		WHERE product_mode = 'personal_notes' AND owner_user_id <> $1`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list forbidden Personal Notes profiles: %w", err)
	}
	defer rows.Close()
	result := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan forbidden Personal Notes profile: %w", err)
		}
		result[id] = struct{}{}
	}
	return result, rows.Err()
}

func (p Profile) validate() error {
	if strings.TrimSpace(p.UpstreamKBID) == "" || strings.TrimSpace(p.OwnerUserID) == "" || p.TenantID == 0 {
		return fmt.Errorf("upstream KB ID, tenant ID, and owner user ID are required")
	}
	switch p.ProductMode {
	case ModePersonalNotes, ModeRAG, ModeOntology:
	default:
		return fmt.Errorf("unsupported product mode %q", p.ProductMode)
	}
	switch p.AccessPolicy {
	case PolicyOwnerOnly, PolicyUpstream:
	default:
		return fmt.Errorf("unsupported access policy %q", p.AccessPolicy)
	}
	if p.ProductMode == ModePersonalNotes && p.AccessPolicy != PolicyOwnerOnly {
		return fmt.Errorf("Personal Notes requires owner_only access policy")
	}
	expectedProfile := "ontology_draft"
	if p.ProductMode == ModePersonalNotes {
		expectedProfile = "notes_plain"
	} else if p.ProductMode == ModeRAG {
		expectedProfile = "plain"
	}
	if p.IndexProfile != expectedProfile || p.IndexProfileVersion < 1 || !json.Valid(p.EffectiveConfig) {
		return fmt.Errorf("invalid effective index profile")
	}
	if p.SchemaVersion < 0 {
		return fmt.Errorf("schema version must not be negative")
	}
	return nil
}
