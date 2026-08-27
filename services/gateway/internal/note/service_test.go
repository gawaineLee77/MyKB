package note

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type fakeProfiles struct{ profile profile.Profile }

func (f fakeProfiles) Get(context.Context, string) (profile.Profile, error) { return f.profile, nil }

type fakeUpstream struct {
	item        weknora.ManualKnowledge
	listPage    *weknora.ManualKnowledgePage
	createCalls int
	updateCalls int
	deleteCalls int
}

func (f *fakeUpstream) ListManualKnowledge(context.Context, string, int, int, http.Header) (weknora.ManualKnowledgePage, error) {
	if f.listPage != nil {
		return *f.listPage, nil
	}
	return weknora.ManualKnowledgePage{Items: []weknora.ManualKnowledge{f.item}, Total: 1, Page: 1, PageSize: 10}, nil
}

func TestImportValidationStopsBeforeUpstream(t *testing.T) {
	identity := access.Identity{UserID: "owner-1", TenantID: 42}
	for _, testCase := range []struct {
		name     string
		filename string
		content  []byte
		code     string
		status   int
	}{
		{name: "pdf", filename: "notes.pdf", content: []byte("fake"), code: "note.file_type_unsupported", status: http.StatusUnsupportedMediaType},
		{name: "docx", filename: "notes.docx", content: []byte("fake"), code: "note.file_type_unsupported", status: http.StatusUnsupportedMediaType},
		{name: "invalid utf8", filename: "notes.md", content: []byte{0xff, 0xfe}, code: "note.invalid_utf8", status: http.StatusBadRequest},
		{name: "oversized", filename: "notes.txt", content: []byte(strings.Repeat("x", MaxNoteBytes+1)), code: "note.size_quota_exceeded", status: http.StatusRequestEntityTooLarge},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			upstream := &fakeUpstream{item: manualFixture()}
			service := newTestService(t, notesProfile(), upstream)
			_, err := service.Import(context.Background(), "kb-notes", testCase.filename, testCase.content, identity, nil)
			assertNoteError(t, err, testCase.code, testCase.status)
			if upstream.createCalls != 0 {
				t.Fatal("invalid import reached upstream creation")
			}
		})
	}
}

func TestImportMarkdownAndText(t *testing.T) {
	for _, filename := range []string{"daily.md", "daily.txt"} {
		upstream := &fakeUpstream{item: manualFixture(), listPage: &weknora.ManualKnowledgePage{Page: 1, PageSize: 10}}
		service := newTestService(t, notesProfile(), upstream)
		created, err := service.Import(context.Background(), "kb-notes", filename, []byte("hello 世界"), access.Identity{UserID: "owner-1", TenantID: 42}, nil)
		if err != nil || created.Title != "daily" || upstream.createCalls != 1 {
			t.Fatalf("Import(%q) = %+v, %v calls=%d", filename, created, err, upstream.createCalls)
		}
	}
}

func TestCountAndCorpusQuotaStopBeforeUpstream(t *testing.T) {
	identity := access.Identity{UserID: "owner-1", TenantID: 42}
	t.Run("count", func(t *testing.T) {
		upstream := &fakeUpstream{item: manualFixture(), listPage: &weknora.ManualKnowledgePage{Total: MaxNoteCount, Page: 1, PageSize: 10}}
		service := newTestService(t, notesProfile(), upstream)
		_, err := service.Create(context.Background(), "kb-notes", WriteInput{Title: "Extra", Content: "body"}, identity, nil)
		assertNoteError(t, err, "note.count_quota_exceeded", http.StatusConflict)
		if upstream.createCalls != 0 {
			t.Fatal("over-count request reached upstream")
		}
	})
	t.Run("corpus", func(t *testing.T) {
		item := manualFixture()
		item.Metadata.Content = strings.Repeat("x", MaxCorpusBytes)
		upstream := &fakeUpstream{item: item, listPage: &weknora.ManualKnowledgePage{Items: []weknora.ManualKnowledge{item}, Total: 1, Page: 1, PageSize: 10}}
		service := newTestService(t, notesProfile(), upstream)
		_, err := service.Create(context.Background(), "kb-notes", WriteInput{Title: "Extra", Content: "body"}, identity, nil)
		assertNoteError(t, err, "note.corpus_quota_exceeded", http.StatusConflict)
		if upstream.createCalls != 0 {
			t.Fatal("over-corpus request reached upstream")
		}
	})
}
func (f *fakeUpstream) GetManualKnowledge(context.Context, string, string, http.Header) (weknora.ManualKnowledge, error) {
	return f.item, nil
}
func (f *fakeUpstream) CreateManualKnowledge(_ context.Context, _ string, input weknora.ManualKnowledgeInput, _ http.Header) (weknora.ManualKnowledge, error) {
	f.createCalls++
	f.item.Title, f.item.Metadata.Content, f.item.Metadata.Status = input.Title, input.Content, input.Status
	return f.item, nil
}
func (f *fakeUpstream) UpdateManualKnowledge(_ context.Context, _, _ string, input weknora.ManualKnowledgeInput, _ http.Header) (weknora.ManualKnowledge, error) {
	f.updateCalls++
	f.item.Title, f.item.Metadata.Content, f.item.Metadata.Status = input.Title, input.Content, input.Status
	f.item.Metadata.Version++
	return f.item, nil
}
func (f *fakeUpstream) DeleteManualKnowledge(context.Context, string, http.Header) error {
	f.deleteCalls++
	return nil
}

func TestOwnerCRUD(t *testing.T) {
	upstream := &fakeUpstream{item: manualFixture()}
	service := newTestService(t, notesProfile(), upstream)
	identity := access.Identity{UserID: "owner-1", TenantID: 42}

	created, err := service.Create(context.Background(), "kb-notes", WriteInput{Title: " New ", Content: "body"}, identity, nil)
	if err != nil || created.Title != "New" || upstream.createCalls != 1 {
		t.Fatalf("Create() = %+v, %v calls=%d", created, err, upstream.createCalls)
	}
	page, err := service.List(context.Background(), "kb-notes", 1, 10, identity, nil)
	if err != nil || page.Total != 1 || page.Items[0].ContentSize != 4 {
		t.Fatalf("List() = %+v, %v", page, err)
	}
	got, err := service.Get(context.Background(), "kb-notes", "note-1", identity, nil)
	if err != nil || got.Content != "body" {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	expected := got.Version
	updated, err := service.Update(context.Background(), "kb-notes", "note-1", WriteInput{Title: "Updated", Content: "line1\r\nline2", Status: "draft", ExpectedVersion: &expected}, identity, nil)
	if err != nil || updated.Content != "line1\nline2" || upstream.updateCalls != 1 {
		t.Fatalf("Update() = %+v, %v calls=%d", updated, err, upstream.updateCalls)
	}
	if err := service.Delete(context.Background(), "kb-notes", "note-1", identity, nil); err != nil || upstream.deleteCalls != 1 {
		t.Fatalf("Delete() error=%v calls=%d", err, upstream.deleteCalls)
	}
}

func TestStaleEditConflictsBeforeUpstreamMutation(t *testing.T) {
	upstream := &fakeUpstream{item: manualFixture()}
	service := newTestService(t, notesProfile(), upstream)
	stale := 0
	_, err := service.Update(context.Background(), "kb-notes", "note-1", WriteInput{Title: "Stale", Content: "body", ExpectedVersion: &stale}, access.Identity{UserID: "owner-1", TenantID: 42}, nil)
	assertNoteError(t, err, "note.version_conflict", http.StatusConflict)
	if upstream.updateCalls != 0 {
		t.Fatal("stale edit reached upstream mutation")
	}
}

func TestRevisionPreviewAndRestorePreserveHistory(t *testing.T) {
	upstream := &fakeUpstream{item: manualFixture()}
	store := newMemoryRevisions()
	service, err := NewService(fakeProfiles{profile: notesProfile()}, upstream, store)
	if err != nil {
		t.Fatal(err)
	}
	identity := access.Identity{UserID: "owner-1", TenantID: 42}
	first, err := service.Get(context.Background(), "kb-notes", "note-1", identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := first.Version
	second, err := service.Update(context.Background(), "kb-notes", "note-1", WriteInput{Title: "Second", Content: "new body", ExpectedVersion: &expected}, identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.GetRevision(context.Background(), "kb-notes", "note-1", 1, identity)
	if err != nil || preview.Content != "body" {
		t.Fatalf("GetRevision() = %+v, %v", preview, err)
	}
	restored, err := service.Restore(context.Background(), "kb-notes", "note-1", RestoreInput{ExpectedVersion: second.Version, TargetVersion: 1}, identity, nil)
	if err != nil || restored.Content != "body" || restored.Version != 3 {
		t.Fatalf("Restore() = %+v, %v", restored, err)
	}
	history, err := service.ListRevisions(context.Background(), "kb-notes", "note-1", identity)
	if err != nil || len(history) != 3 || history[0].Version != 3 || history[0].RestoredFromVersion == nil || *history[0].RestoredFromVersion != 1 {
		t.Fatalf("ListRevisions() = %+v, %v", history, err)
	}
}

func TestNonOwnerAdminCannotReachUpstream(t *testing.T) {
	upstream := &fakeUpstream{item: manualFixture()}
	service := newTestService(t, notesProfile(), upstream)
	_, err := service.Get(context.Background(), "kb-notes", "note-1", access.Identity{UserID: "system-admin", TenantID: 42}, nil)
	assertNoteError(t, err, "resource.not_found", http.StatusNotFound)
	if upstream.createCalls+upstream.updateCalls+upstream.deleteCalls != 0 {
		t.Fatal("denied request reached upstream")
	}
}

func TestOrdinaryRAGCannotUseNotesAPI(t *testing.T) {
	productProfile := notesProfile()
	productProfile.ProductMode = profile.ModeRAG
	productProfile.AccessPolicy = profile.PolicyUpstream
	service := newTestService(t, productProfile, &fakeUpstream{item: manualFixture()})
	_, err := service.List(context.Background(), "kb-rag", 1, 10, access.Identity{UserID: "owner-1", TenantID: 42}, nil)
	assertNoteError(t, err, "note_space.mode_required", http.StatusConflict)
}

func newTestService(t *testing.T, productProfile profile.Profile, upstream *fakeUpstream) *Service {
	t.Helper()
	service, err := NewService(fakeProfiles{profile: productProfile}, upstream, newMemoryRevisions())
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type memoryRevisions struct{ values map[string]Revision }

func newMemoryRevisions() *memoryRevisions {
	return &memoryRevisions{values: make(map[string]Revision)}
}

func revisionKey(kbID, noteID string, version int) string {
	return fmt.Sprintf("%s/%s/%d", kbID, noteID, version)
}

func (m *memoryRevisions) Record(_ context.Context, revision Revision) (Revision, error) {
	revision.ContentSHA256 = revisionHash(revision.Title, revision.Content, revision.Status)
	key := revisionKey(revision.KnowledgeBaseID, revision.NoteID, revision.Version)
	if existing, ok := m.values[key]; ok {
		if existing.ContentSHA256 != revision.ContentSHA256 {
			return Revision{}, ErrRevisionConflict
		}
		return existing, nil
	}
	m.values[key] = revision
	return revision, nil
}
func (m *memoryRevisions) List(_ context.Context, kbID, noteID string) ([]Revision, error) {
	result := make([]Revision, 0)
	for _, revision := range m.values {
		if revision.KnowledgeBaseID == kbID && revision.NoteID == noteID {
			revision.Content = ""
			result = append(result, revision)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version > result[j].Version })
	return result, nil
}
func (m *memoryRevisions) Get(_ context.Context, kbID, noteID string, version int) (Revision, error) {
	revision, ok := m.values[revisionKey(kbID, noteID, version)]
	if !ok {
		return Revision{}, ErrRevisionNotFound
	}
	return revision, nil
}

func notesProfile() profile.Profile {
	return profile.Profile{UpstreamKBID: "kb-notes", TenantID: 42, OwnerUserID: "owner-1", ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly}
}

func manualFixture() weknora.ManualKnowledge {
	return weknora.ManualKnowledge{ID: "note-1", KnowledgeBaseID: "kb-notes", Type: "manual", Title: "Daily", Metadata: weknora.ManualKnowledgeMetadata{Content: "body", Format: "markdown", Status: "publish", Version: 1}}
}

func assertNoteError(t *testing.T, err error, code string, status int) {
	t.Helper()
	var noteError *Error
	if !errors.As(err, &noteError) || noteError.Code != code || noteError.StatusCode != status {
		t.Fatalf("error=%+v, want %s/%d", err, code, status)
	}
}
