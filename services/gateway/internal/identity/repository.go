// Package identity owns the corporate-subject mapping used by MindCreek SSO.
package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("corporate identity not found")
	ErrSuspended = errors.New("corporate identity suspended")
	ErrInvalid   = errors.New("corporate identity invalid")
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

type Identity struct {
	Issuer         string
	Subject        string
	BrokerSubject  string
	UpstreamEmail  string
	CorporateEmail string
	Username       string
	DisplayName    string
	Groups         []string
	Status         Status
	LocalUserID    string
	LocalTenantID  uint64
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	SuspendedAt    *time.Time
}

type Claims struct {
	Issuer         string
	Subject        string
	CorporateEmail string
	Username       string
	DisplayName    string
	Groups         []string
}

type Store interface {
	Upsert(context.Context, Claims, time.Time) (Identity, error)
	GetByUpstreamEmail(context.Context, string) (Identity, error)
	GetByBrokerSubject(context.Context, string) (Identity, error)
	BindLocalPrincipal(context.Context, string, string, uint64) error
	SetStatus(context.Context, string, Status, time.Time) (Identity, error)
	RecordAudit(context.Context, AuditEvent) error
}

type AuditEvent struct {
	ID            string
	Issuer        string
	Subject       string
	LocalUserID   string
	Action        string
	Outcome       string
	ErrorCode     string
	CorrelationID string
	SourceIP      string
	CreatedAt     time.Time
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("identity database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Upsert(ctx context.Context, claims Claims, now time.Time) (Identity, error) {
	claims = normalizeClaims(claims)
	if err := validateClaims(claims); err != nil || now.IsZero() {
		return Identity{}, ErrInvalid
	}
	groups, err := json.Marshal(claims.Groups)
	if err != nil {
		return Identity{}, ErrInvalid
	}
	brokerSubject, upstreamEmail := stableAliases(claims.Issuer, claims.Subject)
	return scanIdentity(r.db.QueryRowContext(ctx, `
		INSERT INTO mindcreek.corporate_identities
			(issuer, subject, broker_subject, upstream_email, corporate_email, username,
			 display_name, groups_json, status, first_seen_at, last_seen_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$9)
		ON CONFLICT (issuer, subject) DO UPDATE SET
			corporate_email=EXCLUDED.corporate_email,
			username=EXCLUDED.username,
			display_name=EXCLUDED.display_name,
			groups_json=EXCLUDED.groups_json,
			last_seen_at=EXCLUDED.last_seen_at
		RETURNING issuer, subject, broker_subject, upstream_email, corporate_email, username,
		          display_name, groups_json, status, local_user_id, local_tenant_id,
		          first_seen_at, last_seen_at, suspended_at`,
		claims.Issuer, claims.Subject, brokerSubject, upstreamEmail, claims.CorporateEmail,
		claims.Username, claims.DisplayName, groups, now.UTC()))
}

func (r *Repository) GetByUpstreamEmail(ctx context.Context, email string) (Identity, error) {
	return scanIdentity(r.db.QueryRowContext(ctx, identitySelect+` WHERE upstream_email=$1`, strings.ToLower(strings.TrimSpace(email))))
}

func (r *Repository) GetByBrokerSubject(ctx context.Context, subject string) (Identity, error) {
	return scanIdentity(r.db.QueryRowContext(ctx, identitySelect+` WHERE broker_subject=$1`, strings.TrimSpace(subject)))
}

func (r *Repository) BindLocalPrincipal(ctx context.Context, upstreamEmail, userID string, tenantID uint64) error {
	if strings.TrimSpace(upstreamEmail) == "" || strings.TrimSpace(userID) == "" || tenantID == 0 {
		return ErrInvalid
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE mindcreek.corporate_identities
		SET local_user_id=$2, local_tenant_id=$3
		WHERE upstream_email=$1 AND (local_user_id IS NULL OR local_user_id=$2)`,
		strings.ToLower(strings.TrimSpace(upstreamEmail)), strings.TrimSpace(userID), tenantID)
	if err != nil {
		return fmt.Errorf("bind corporate identity: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("bind corporate identity: %w", err)
	}
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetStatus(ctx context.Context, brokerSubject string, status Status, now time.Time) (Identity, error) {
	if strings.TrimSpace(brokerSubject) == "" || len(brokerSubject) > 64 || now.IsZero() || (status != StatusActive && status != StatusSuspended) {
		return Identity{}, ErrInvalid
	}
	return scanIdentity(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.corporate_identities
		SET status=$2, suspended_at=CASE WHEN $2='suspended' THEN $3 ELSE NULL END
		WHERE broker_subject=$1
		RETURNING issuer, subject, broker_subject, upstream_email, corporate_email, username,
		          display_name, groups_json, status, local_user_id, local_tenant_id,
		          first_seen_at, last_seen_at, suspended_at`, brokerSubject, status, now.UTC()))
}

func (r *Repository) RecordAudit(ctx context.Context, event AuditEvent) error {
	if event.ID == "" || event.Issuer == "" || event.Subject == "" || event.Action == "" ||
		event.CorrelationID == "" || event.CreatedAt.IsZero() ||
		(event.Outcome != "success" && event.Outcome != "denied" && event.Outcome != "failure") {
		return ErrInvalid
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.identity_audit_events
			(id, issuer, subject_hash, local_user_id, action, outcome, error_code,
			 correlation_id, source_ip_hash, created_at)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''),$8,NULLIF($9,''),$10)`,
		event.ID, event.Issuer, hashText(event.Subject), event.LocalUserID, event.Action,
		event.Outcome, event.ErrorCode, event.CorrelationID, hashText(event.SourceIP), event.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("record identity audit event: %w", err)
	}
	return nil
}

const identitySelect = `SELECT issuer, subject, broker_subject, upstream_email, corporate_email, username,
	display_name, groups_json, status, local_user_id, local_tenant_id,
	first_seen_at, last_seen_at, suspended_at FROM mindcreek.corporate_identities`

type scanner interface{ Scan(...any) error }

func scanIdentity(row scanner) (Identity, error) {
	var result Identity
	var groups []byte
	var status string
	var localUser sql.NullString
	var localTenant sql.NullInt64
	var suspended sql.NullTime
	err := row.Scan(&result.Issuer, &result.Subject, &result.BrokerSubject, &result.UpstreamEmail,
		&result.CorporateEmail, &result.Username, &result.DisplayName, &groups, &status,
		&localUser, &localTenant, &result.FirstSeenAt, &result.LastSeenAt, &suspended)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, ErrNotFound
	}
	if err != nil {
		return Identity{}, fmt.Errorf("scan corporate identity: %w", err)
	}
	result.Status = Status(status)
	result.LocalUserID = localUser.String
	if localTenant.Valid && localTenant.Int64 > 0 {
		result.LocalTenantID = uint64(localTenant.Int64)
	}
	if suspended.Valid {
		value := suspended.Time
		result.SuspendedAt = &value
	}
	if json.Unmarshal(groups, &result.Groups) != nil {
		return Identity{}, ErrInvalid
	}
	if result.Status != StatusActive && result.Status != StatusSuspended {
		return Identity{}, ErrInvalid
	}
	return result, nil
}

func normalizeClaims(claims Claims) Claims {
	claims.Issuer = strings.TrimRight(strings.TrimSpace(claims.Issuer), "/")
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.CorporateEmail = strings.ToLower(strings.TrimSpace(claims.CorporateEmail))
	claims.Username = strings.TrimSpace(claims.Username)
	claims.DisplayName = strings.TrimSpace(claims.DisplayName)
	unique := make(map[string]bool)
	originalGroups := append([]string(nil), claims.Groups...)
	claims.Groups = claims.Groups[:0]
	for _, group := range originalGroups {
		if value := strings.ToLower(strings.TrimSpace(group)); value != "" && !unique[value] {
			unique[value] = true
			claims.Groups = append(claims.Groups, value)
		}
	}
	sort.Strings(claims.Groups)
	return claims
}

func validateClaims(claims Claims) error {
	if claims.Issuer == "" || len(claims.Issuer) > 2048 || claims.Subject == "" || len(claims.Subject) > 1024 ||
		claims.CorporateEmail == "" || len(claims.CorporateEmail) > 320 || !strings.Contains(claims.CorporateEmail, "@") ||
		claims.Username == "" || len(claims.Username) > 160 || len(claims.DisplayName) > 320 || len(claims.Groups) > 256 {
		return ErrInvalid
	}
	return nil
}

func stableAliases(issuer, subject string) (string, string) {
	digest := sha256.Sum256([]byte(issuer + "\x00" + subject))
	brokerSubject := base64.RawURLEncoding.EncodeToString(digest[:])
	return brokerSubject, "mc-" + hex.EncodeToString(digest[:16]) + "@identity.invalid"
}

func hashText(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
