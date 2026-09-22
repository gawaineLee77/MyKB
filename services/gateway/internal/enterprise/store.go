// Package enterprise owns installation and one-time workspace onboarding.
package enterprise

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"
)

var ErrNotFound = errors.New("enterprise record not found")

type Installation struct {
	Stage           string    `json:"stage"`
	AdminEmail      string    `json:"admin_email,omitempty"`
	AdminUserID     string    `json:"admin_user_id,omitempty"`
	DefaultTenantID uint64    `json:"default_tenant_id,omitempty"`
	MemberKeyID     string    `json:"member_key_id,omitempty"`
	MemberKeyRef    string    `json:"-"`
	ErrorCode       string    `json:"error_code,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Store interface {
	Installation(context.Context) (Installation, error)
	SaveInstallation(context.Context, Installation) error
	WithLock(context.Context, string, func(Store) error) error
	Onboarding(context.Context, string) (OnboardingRecord, error)
	SaveOnboarding(context.Context, OnboardingRecord) error
}

type OnboardingRecord struct {
	Subject, UserID string
	TenantID        uint64
	State           string
	OutcomeUnknown  bool
	CompletedAt     *time.Time
	ErrorCode       string
}

func (r *Repository) Onboarding(ctx context.Context, subject string) (OnboardingRecord, error) {
	var o OnboardingRecord
	err := r.q.QueryRowContext(ctx, `SELECT broker_subject,local_user_id,default_tenant_id,state,outcome_unknown,completed_at,error_code
        FROM mindcreek.employee_onboarding WHERE broker_subject=$1`, subject).Scan(&o.Subject, &o.UserID, &o.TenantID, &o.State, &o.OutcomeUnknown, &o.CompletedAt, &o.ErrorCode)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return o, err
}

func (r *Repository) SaveOnboarding(ctx context.Context, o OnboardingRecord) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO mindcreek.employee_onboarding
        (broker_subject,local_user_id,default_tenant_id,state,outcome_unknown,completed_at,error_code)
        VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(broker_subject) DO UPDATE SET
        state=EXCLUDED.state,outcome_unknown=EXCLUDED.outcome_unknown,
        completed_at=COALESCE(employee_onboarding.completed_at,EXCLUDED.completed_at),error_code=EXCLUDED.error_code,updated_at=now()`,
		o.Subject, o.UserID, o.TenantID, o.State, o.OutcomeUnknown, o.CompletedAt, o.ErrorCode)
	return err
}

type queries interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Repository struct {
	db *sql.DB
	q  queries
}

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db, q: db} }

func (r *Repository) Installation(ctx context.Context) (Installation, error) {
	var i Installation
	err := r.q.QueryRowContext(ctx, `SELECT stage,admin_email,admin_user_id,COALESCE(default_tenant_id,0),member_key_id,member_key_ref,error_code,updated_at FROM mindcreek.enterprise_installation WHERE id=1`).Scan(&i.Stage, &i.AdminEmail, &i.AdminUserID, &i.DefaultTenantID, &i.MemberKeyID, &i.MemberKeyRef, &i.ErrorCode, &i.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return i, err
}

func (r *Repository) SaveInstallation(ctx context.Context, i Installation) error {
	_, err := r.q.ExecContext(ctx, `INSERT INTO mindcreek.enterprise_installation
        (id,stage,admin_email,admin_user_id,default_tenant_id,member_key_id,member_key_ref,error_code)
        VALUES (1,$1,$2,$3,NULLIF($4,0),$5,$6,$7)
        ON CONFLICT (id) DO UPDATE SET stage=EXCLUDED.stage,admin_email=EXCLUDED.admin_email,
        admin_user_id=EXCLUDED.admin_user_id,default_tenant_id=EXCLUDED.default_tenant_id,
        member_key_id=EXCLUDED.member_key_id,member_key_ref=EXCLUDED.member_key_ref,
        error_code=EXCLUDED.error_code,updated_at=now()`, i.Stage, i.AdminEmail, i.AdminUserID, i.DefaultTenantID, i.MemberKeyID, i.MemberKeyRef, i.ErrorCode)
	return err
}

// Session locks span autocommitted progress saves and remote REST calls. A
// remote failure must not roll back the record describing that attempted write.
func (r *Repository) WithLock(ctx context.Context, key string, fn func(Store) error) error {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 20260915))`, key); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1, 20260915))`, key); err != nil {
			// Never return a connection with an outstanding session lock.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return fn(&Repository{db: r.db, q: conn})
}
