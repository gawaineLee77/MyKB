package subscription

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/jackc/pgx/v5/pgconn"
)

type Store interface {
	Create(context.Context, Subscription) (Subscription, error)
	GetByPublicationUser(context.Context, string, string) (Subscription, error)
	ListByUser(context.Context, string) ([]Subscription, error)
	Activate(context.Context, string, uint64, int64, string, time.Time) (Subscription, error)
	Unsubscribe(context.Context, string, string, time.Time) (Subscription, error)
	MarkSeen(context.Context, string, int64, string, time.Time) (Subscription, error)
	InactivatePublication(context.Context, string, string, time.Time) error
	InactivateOutsideAudience(context.Context, string, publication.Audience, string, time.Time) error
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("subscription database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Create(ctx context.Context, candidate Subscription) (Subscription, error) {
	if err := candidate.Validate(); err != nil || !candidate.Active() {
		return Subscription{}, ErrInvalid
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.kb_subscriptions
			(id, publication_id, subscriber_id, subscriber_tenant_id, status, notification_enabled,
			 last_seen_revision, created_at, updated_at, ended_at, last_audit_correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, candidate.ID, candidate.PublicationID,
		candidate.SubscriberID, candidate.SubscriberTenantID, candidate.Status, candidate.NotificationEnabled,
		candidate.LastSeenRevision, candidate.CreatedAt, candidate.UpdatedAt, candidate.EndedAt,
		candidate.LastAuditCorrelationID)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return Subscription{}, ErrInvalid
		}
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return candidate, nil
}

func (r *Repository) GetByPublicationUser(ctx context.Context, publicationID, userID string) (Subscription, error) {
	return scanSubscription(r.db.QueryRowContext(ctx, subscriptionSelect+` WHERE publication_id=$1 AND subscriber_id=$2`, publicationID, userID))
}

func (r *Repository) ListByUser(ctx context.Context, userID string) ([]Subscription, error) {
	rows, err := r.db.QueryContext(ctx, subscriptionSelect+` WHERE subscriber_id=$1 ORDER BY updated_at DESC, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	result := make([]Subscription, 0)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) Activate(ctx context.Context, id string, tenantID uint64, revision int64, correlationID string, at time.Time) (Subscription, error) {
	return scanSubscription(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_subscriptions
		SET subscriber_tenant_id=$2, status='active', last_seen_revision=$3, updated_at=$5,
		    ended_at=NULL, last_audit_correlation_id=$4
		WHERE id=$1
		RETURNING id, publication_id, subscriber_id, subscriber_tenant_id, status, notification_enabled,
		          last_seen_revision, created_at, updated_at, ended_at, last_audit_correlation_id`,
		id, tenantID, revision, correlationID, at))
}

func (r *Repository) Unsubscribe(ctx context.Context, id, correlationID string, at time.Time) (Subscription, error) {
	return scanSubscription(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_subscriptions
		SET status='unsubscribed', updated_at=$3, ended_at=$3, last_audit_correlation_id=$2
		WHERE id=$1
		RETURNING id, publication_id, subscriber_id, subscriber_tenant_id, status, notification_enabled,
		          last_seen_revision, created_at, updated_at, ended_at, last_audit_correlation_id`, id, correlationID, at))
}

func (r *Repository) MarkSeen(ctx context.Context, id string, revision int64, correlationID string, at time.Time) (Subscription, error) {
	return scanSubscription(r.db.QueryRowContext(ctx, `
		UPDATE mindcreek.kb_subscriptions
		SET last_seen_revision=GREATEST(last_seen_revision,$2), updated_at=$4, last_audit_correlation_id=$3
		WHERE id=$1 AND status='active'
		RETURNING id, publication_id, subscriber_id, subscriber_tenant_id, status, notification_enabled,
		          last_seen_revision, created_at, updated_at, ended_at, last_audit_correlation_id`, id, revision, correlationID, at))
}

func (r *Repository) InactivatePublication(ctx context.Context, publicationID, correlationID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE mindcreek.kb_subscriptions SET status='inactive', updated_at=$3, ended_at=$3,
		    last_audit_correlation_id=$2
		WHERE publication_id=$1 AND status='active'`, publicationID, correlationID, at)
	if err != nil {
		return fmt.Errorf("inactivate publication subscriptions: %w", err)
	}
	return nil
}

func (r *Repository) InactivateOutsideAudience(ctx context.Context, publicationID string, audience publication.Audience, correlationID string, at time.Time) error {
	if !audience.Valid() {
		return ErrInvalid
	}
	if audience.Type == publication.AudienceOrganization {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, subscriber_tenant_id FROM mindcreek.kb_subscriptions WHERE publication_id=$1 AND status='active'`, publicationID)
	if err != nil {
		return err
	}
	type candidate struct {
		id     string
		tenant uint64
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.tenant); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range candidates {
		if audience.Allows(item.tenant) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE mindcreek.kb_subscriptions SET status='inactive', updated_at=$2, ended_at=$2, last_audit_correlation_id=$3 WHERE id=$1 AND status='active'`, item.id, at, correlationID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const subscriptionSelect = `SELECT id, publication_id, subscriber_id, subscriber_tenant_id, status, notification_enabled,
	last_seen_revision, created_at, updated_at, ended_at, last_audit_correlation_id FROM mindcreek.kb_subscriptions`

type scanner interface{ Scan(...any) error }

func scanSubscription(row scanner) (Subscription, error) {
	var result Subscription
	var status string
	var ended sql.NullTime
	err := row.Scan(&result.ID, &result.PublicationID, &result.SubscriberID, &result.SubscriberTenantID,
		&status, &result.NotificationEnabled, &result.LastSeenRevision, &result.CreatedAt, &result.UpdatedAt,
		&ended, &result.LastAuditCorrelationID)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("scan subscription: %w", err)
	}
	result.Status = Status(status)
	if ended.Valid {
		value := ended.Time
		result.EndedAt = &value
	}
	if err := result.Validate(); err != nil {
		return Subscription{}, fmt.Errorf("invalid stored subscription: %w", err)
	}
	return result, nil
}
