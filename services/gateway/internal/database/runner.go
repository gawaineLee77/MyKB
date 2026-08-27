// Package database owns MindCreek's isolated PostgreSQL schema and migrations.
package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const advisoryLockID int64 = 5065495338276654593

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version  int64
	Name     string
	UpSQL    string
	DownSQL  string
	Checksum string
}

type AppliedMigration struct {
	Version   int64
	Name      string
	Checksum  string
	AppliedAt time.Time
}

type Runner struct {
	db         *sql.DB
	migrations []Migration
}

// Open connects to the product database. The URL must be supplied by deployment configuration.
func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("MINDCREEK_DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open product database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping product database: %w", err)
	}
	return db, nil
}

func NewRunner(db *sql.DB) (*Runner, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	migrations, err := loadMigrations(migrationFiles)
	if err != nil {
		return nil, err
	}
	return &Runner{db: db, migrations: migrations}, nil
}

// Up applies every pending product migration in one locked connection.
func (r *Runner) Up(ctx context.Context) error {
	return r.withLock(ctx, func(conn *sql.Conn) error {
		if err := bootstrap(ctx, conn); err != nil {
			return err
		}
		applied, err := appliedMigrations(ctx, conn)
		if err != nil {
			return err
		}
		for _, migration := range r.migrations {
			if previous, ok := applied[migration.Version]; ok {
				if previous.Checksum != migration.Checksum || previous.Name != migration.Name {
					return fmt.Errorf("migration %d changed after application", migration.Version)
				}
				continue
			}
			if err := applyMigration(ctx, conn, migration); err != nil {
				return err
			}
		}
		return nil
	})
}

// Down rolls back the requested number of applied migrations, newest first.
func (r *Runner) Down(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("migration down steps must be positive")
	}
	return r.withLock(ctx, func(conn *sql.Conn) error {
		if err := bootstrap(ctx, conn); err != nil {
			return err
		}
		applied, err := r.StatusOnConn(ctx, conn)
		if err != nil {
			return err
		}
		if steps > len(applied) {
			steps = len(applied)
		}
		byVersion := make(map[int64]Migration, len(r.migrations))
		for _, migration := range r.migrations {
			byVersion[migration.Version] = migration
		}
		for index := 0; index < steps; index++ {
			current := applied[index]
			migration, ok := byVersion[current.Version]
			if !ok {
				return fmt.Errorf("cannot roll back unknown migration %d", current.Version)
			}
			if err := rollbackMigration(ctx, conn, migration); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Runner) Status(ctx context.Context) ([]AppliedMigration, error) {
	var result []AppliedMigration
	err := r.withLock(ctx, func(conn *sql.Conn) error {
		if err := bootstrap(ctx, conn); err != nil {
			return err
		}
		var err error
		result, err = r.StatusOnConn(ctx, conn)
		return err
	})
	return result, err
}

func (r *Runner) StatusOnConn(ctx context.Context, conn *sql.Conn) ([]AppliedMigration, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT version, name, checksum, applied_at
		FROM mindcreek.schema_migrations
		ORDER BY version DESC`)
	if err != nil {
		return nil, fmt.Errorf("list product migrations: %w", err)
	}
	defer rows.Close()
	var result []AppliedMigration
	for rows.Next() {
		var item AppliedMigration
		if err := rows.Scan(&item.Version, &item.Name, &item.Checksum, &item.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan product migration: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Runner) withLock(ctx context.Context, operation func(*sql.Conn) error) error {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("lock product migrations: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	return operation(conn)
}

func bootstrap(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS mindcreek;
		CREATE TABLE IF NOT EXISTS mindcreek.schema_migrations (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("bootstrap product migration history: %w", err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *sql.Conn) (map[int64]AppliedMigration, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version, name, checksum, applied_at FROM mindcreek.schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read product migration history: %w", err)
	}
	defer rows.Close()
	result := make(map[int64]AppliedMigration)
	for rows.Next() {
		var item AppliedMigration
		if err := rows.Scan(&item.Version, &item.Name, &item.Checksum, &item.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan product migration history: %w", err)
		}
		result[item.Version] = item
	}
	return result, rows.Err()
}

func applyMigration(ctx context.Context, conn *sql.Conn, migration Migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
		return fmt.Errorf("apply product migration %d: %w", migration.Version, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO mindcreek.schema_migrations (version, name, checksum)
		VALUES ($1, $2, $3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
		return fmt.Errorf("record product migration %d: %w", migration.Version, err)
	}
	return tx.Commit()
}

func rollbackMigration(ctx context.Context, conn *sql.Conn, migration Migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migration.DownSQL); err != nil {
		return fmt.Errorf("roll back product migration %d: %w", migration.Version, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mindcreek.schema_migrations WHERE version = $1`, migration.Version); err != nil {
		return fmt.Errorf("remove product migration %d: %w", migration.Version, err)
	}
	return tx.Commit()
}

func loadMigrations(files fs.FS) ([]Migration, error) {
	entries, err := fs.Glob(files, "migrations/*.up.sql")
	if err != nil {
		return nil, err
	}
	var migrations []Migration
	for _, upPath := range entries {
		base := strings.TrimSuffix(strings.TrimPrefix(upPath, "migrations/"), ".up.sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename %q", upPath)
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", upPath)
		}
		upSQL, err := fs.ReadFile(files, upPath)
		if err != nil {
			return nil, err
		}
		downSQL, err := fs.ReadFile(files, "migrations/"+base+".down.sql")
		if err != nil {
			return nil, fmt.Errorf("migration %d has no down file: %w", version, err)
		}
		digest := sha256.Sum256(upSQL)
		migrations = append(migrations, Migration{
			Version: version, Name: parts[1], UpSQL: string(upSQL), DownSQL: string(downSQL), Checksum: hex.EncodeToString(digest[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for index := 1; index < len(migrations); index++ {
		if migrations[index-1].Version == migrations[index].Version {
			return nil, fmt.Errorf("duplicate migration version %d", migrations[index].Version)
		}
	}
	if len(migrations) == 0 {
		return nil, errors.New("no product migrations embedded")
	}
	return migrations, nil
}
