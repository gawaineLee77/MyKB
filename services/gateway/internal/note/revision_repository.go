package note

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRevisionNotFound = errors.New("note revision not found")
	ErrRevisionConflict = errors.New("note revision content conflict")
)

type Revision struct {
	KnowledgeBaseID     string    `json:"knowledge_base_id"`
	NoteID              string    `json:"note_id"`
	Version             int       `json:"version"`
	Title               string    `json:"title"`
	Content             string    `json:"content,omitempty"`
	Status              string    `json:"status"`
	Operation           string    `json:"operation"`
	RestoredFromVersion *int      `json:"restored_from_version,omitempty"`
	ContentSHA256       string    `json:"content_sha256"`
	ActorUserID         string    `json:"actor_user_id"`
	RecordedAt          time.Time `json:"recorded_at"`
}

type RevisionStore interface {
	Record(context.Context, Revision) (Revision, error)
	List(context.Context, string, string) ([]Revision, error)
	Get(context.Context, string, string, int) (Revision, error)
}

type RevisionRepository struct{ db *sql.DB }

func NewRevisionRepository(db *sql.DB) (*RevisionRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("revision database is required")
	}
	return &RevisionRepository{db: db}, nil
}

func (r *RevisionRepository) Record(ctx context.Context, candidate Revision) (Revision, error) {
	if err := validateRevision(candidate); err != nil {
		return Revision{}, err
	}
	candidate.ContentSHA256 = revisionHash(candidate.Title, candidate.Content, candidate.Status)
	if candidate.RecordedAt.IsZero() {
		candidate.RecordedAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO mindcreek.note_revisions
			(upstream_kb_id, upstream_note_id, version, title, content, status, operation,
			 restored_from_version, content_sha256, actor_user_id, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (upstream_kb_id, upstream_note_id, version) DO NOTHING`,
		candidate.KnowledgeBaseID, candidate.NoteID, candidate.Version, candidate.Title, candidate.Content,
		candidate.Status, candidate.Operation, candidate.RestoredFromVersion, candidate.ContentSHA256,
		candidate.ActorUserID, candidate.RecordedAt)
	if err != nil {
		return Revision{}, fmt.Errorf("record note revision: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Revision{}, fmt.Errorf("inspect note revision insert: %w", err)
	}
	if rows == 1 {
		return candidate, nil
	}
	existing, err := r.Get(ctx, candidate.KnowledgeBaseID, candidate.NoteID, candidate.Version)
	if err != nil {
		return Revision{}, err
	}
	if existing.ContentSHA256 != candidate.ContentSHA256 {
		return Revision{}, ErrRevisionConflict
	}
	return existing, nil
}

func (r *RevisionRepository) List(ctx context.Context, kbID, noteID string) ([]Revision, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT upstream_kb_id, upstream_note_id, version, title, status, operation,
		       restored_from_version, content_sha256, actor_user_id, recorded_at
		FROM mindcreek.note_revisions
		WHERE upstream_kb_id = $1 AND upstream_note_id = $2
		ORDER BY version DESC`, kbID, noteID)
	if err != nil {
		return nil, fmt.Errorf("list note revisions: %w", err)
	}
	defer rows.Close()
	result := make([]Revision, 0)
	for rows.Next() {
		var revision Revision
		if err := rows.Scan(&revision.KnowledgeBaseID, &revision.NoteID, &revision.Version, &revision.Title,
			&revision.Status, &revision.Operation, &revision.RestoredFromVersion, &revision.ContentSHA256,
			&revision.ActorUserID, &revision.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan note revision: %w", err)
		}
		result = append(result, revision)
	}
	return result, rows.Err()
}

func (r *RevisionRepository) Get(ctx context.Context, kbID, noteID string, version int) (Revision, error) {
	var revision Revision
	err := r.db.QueryRowContext(ctx, `
		SELECT upstream_kb_id, upstream_note_id, version, title, content, status, operation,
		       restored_from_version, content_sha256, actor_user_id, recorded_at
		FROM mindcreek.note_revisions
		WHERE upstream_kb_id = $1 AND upstream_note_id = $2 AND version = $3`, kbID, noteID, version).Scan(
		&revision.KnowledgeBaseID, &revision.NoteID, &revision.Version, &revision.Title, &revision.Content,
		&revision.Status, &revision.Operation, &revision.RestoredFromVersion, &revision.ContentSHA256,
		&revision.ActorUserID, &revision.RecordedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, ErrRevisionNotFound
	}
	if err != nil {
		return Revision{}, fmt.Errorf("get note revision: %w", err)
	}
	return revision, nil
}

func validateRevision(revision Revision) error {
	if strings.TrimSpace(revision.KnowledgeBaseID) == "" || strings.TrimSpace(revision.NoteID) == "" ||
		revision.Version < 1 || strings.TrimSpace(revision.Title) == "" || strings.TrimSpace(revision.Content) == "" ||
		strings.TrimSpace(revision.ActorUserID) == "" {
		return fmt.Errorf("revision knowledge base, note, version, title, content, and actor are required")
	}
	if revision.Status != "draft" && revision.Status != "publish" {
		return fmt.Errorf("invalid revision status %q", revision.Status)
	}
	switch revision.Operation {
	case "create", "edit", "import", "restore", "snapshot":
	default:
		return fmt.Errorf("invalid revision operation %q", revision.Operation)
	}
	if len([]byte(revision.Content)) > MaxNoteBytes {
		return fmt.Errorf("revision content exceeds note limit")
	}
	return nil
}

func revisionHash(title, content, status string) string {
	digest := sha256.Sum256([]byte(title + "\x00" + status + "\x00" + content))
	return hex.EncodeToString(digest[:])
}
