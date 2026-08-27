package space

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/preset"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type fakeRequests struct {
	row       *CreationRequest
	completed int
	failed    int
}

func (f *fakeRequests) Claim(_ context.Context, candidate CreationRequest) (CreationRequest, bool, error) {
	if f.row == nil {
		candidate.Status = StatusPending
		f.row = &candidate
		return candidate, true, nil
	}
	if f.row.RequestHash != candidate.RequestHash {
		return CreationRequest{}, false, ErrIdempotencyConflict
	}
	return *f.row, false, nil
}

func (f *fakeRequests) Complete(_ context.Context, request CreationRequest) error {
	f.completed++
	request.Status = StatusReady
	f.row = &request
	return nil
}

func (f *fakeRequests) Fail(_ context.Context, request CreationRequest, message string) error {
	f.failed++
	request.Status = StatusFailed
	request.LastError = message
	f.row = &request
	return nil
}

type fakeProfileStore struct {
	items    map[string]profile.Profile
	failNext error
}

func (f *fakeProfileStore) Create(_ context.Context, candidate profile.Profile) (profile.Profile, error) {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return profile.Profile{}, err
	}
	if _, exists := f.items[candidate.UpstreamKBID]; exists {
		return profile.Profile{}, profile.ErrConflict
	}
	f.items[candidate.UpstreamKBID] = candidate
	return candidate, nil
}

func (f *fakeProfileStore) Get(_ context.Context, id string) (profile.Profile, error) {
	item, exists := f.items[id]
	if !exists {
		return profile.Profile{}, profile.ErrNotFound
	}
	return item, nil
}

type fakeUpstream struct {
	items       map[string]weknora.KnowledgeBase
	createCalls int
	lastCreate  weknora.CreateKnowledgeBaseRequest
}

func (f *fakeUpstream) GetKnowledgeBase(_ context.Context, id string, _ http.Header) (weknora.KnowledgeBase, error) {
	item, exists := f.items[id]
	if !exists {
		return weknora.KnowledgeBase{}, &weknora.Error{Code: "upstream.not_found", StatusCode: http.StatusNotFound}
	}
	return item, nil
}

func (f *fakeUpstream) CreateKnowledgeBase(_ context.Context, input weknora.CreateKnowledgeBaseRequest, _ http.Header) (weknora.KnowledgeBase, error) {
	f.createCalls++
	f.lastCreate = input
	item := weknora.KnowledgeBase{
		ID: input.ID, Name: input.Name, Type: input.Type, Description: input.Description,
		TenantID: 42, CreatorID: "alice", EmbeddingModelID: input.EmbeddingModelID,
	}
	f.items[item.ID] = item
	return item, nil
}

func TestPlainRAGProfileReproducesEffectiveUpstreamConfiguration(t *testing.T) {
	profiles := &fakeProfileStore{items: map[string]profile.Profile{}}
	upstream := &fakeUpstream{items: map[string]weknora.KnowledgeBase{}}
	service, _ := NewService(&fakeRequests{}, profiles, upstream)
	input := CreateInput{Mode: "rag", IndexProfile: "plain", Name: "Documents", EmbeddingModelID: "embedding-1", SummaryModelID: "summary-1", StorageProvider: "local"}
	result, err := service.Create(context.Background(), input, "create-rag-0001", access.Identity{UserID: "alice", TenantID: 42}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stored := profiles.items[result.KnowledgeBaseID]
	var effective preset.EffectiveConfig
	if err := json.Unmarshal(stored.EffectiveConfig, &effective); err != nil {
		t.Fatal(err)
	}
	if stored.IndexProfile != "plain" || stored.IndexProfileVersion != preset.Version ||
		!reflect.DeepEqual(effective.Chunking, upstream.lastCreate.ChunkingConfig) ||
		effective.Indexing != upstream.lastCreate.IndexingStrategy ||
		effective.Models.EmbeddingModelID != upstream.lastCreate.EmbeddingModelID ||
		effective.Models.SummaryModelID != upstream.lastCreate.SummaryModelID {
		t.Fatalf("stored=%+v effective=%+v upstream=%+v", stored, effective, upstream.lastCreate)
	}
}

func validCreateInput() CreateInput {
	return CreateInput{
		Mode: "personal_notes", Name: "Alice Notes", Description: "Private notes",
		EmbeddingModelID: "embedding-1", StorageProvider: "local",
	}
}

func TestCreateIsIdempotentAndUsesApprovedPreset(t *testing.T) {
	requests := &fakeRequests{}
	profiles := &fakeProfileStore{items: map[string]profile.Profile{}}
	upstream := &fakeUpstream{items: map[string]weknora.KnowledgeBase{}}
	service, _ := NewService(requests, profiles, upstream)
	identity := access.Identity{UserID: "alice", TenantID: 42}

	first, err := service.Create(context.Background(), validCreateInput(), "create-note-0001", identity, nil)
	if err != nil || !first.Created || first.ProductMode != profile.ModePersonalNotes || first.AccessPolicy != profile.PolicyOwnerOnly {
		t.Fatalf("first Create() = %+v, %v", first, err)
	}
	second, err := service.Create(context.Background(), validCreateInput(), "create-note-0001", identity, nil)
	if err != nil || second.KnowledgeBaseID != first.KnowledgeBaseID || !second.Reconciled {
		t.Fatalf("second Create() = %+v, %v", second, err)
	}
	if upstream.createCalls != 1 || len(profiles.items) != 1 {
		t.Fatalf("upstream creates=%d profiles=%d", upstream.createCalls, len(profiles.items))
	}
}

func TestCreateReconcilesProfileFailureWithoutDuplicateUpstreamKB(t *testing.T) {
	requests := &fakeRequests{}
	profiles := &fakeProfileStore{items: map[string]profile.Profile{}, failNext: errors.New("database unavailable")}
	upstream := &fakeUpstream{items: map[string]weknora.KnowledgeBase{}}
	service, _ := NewService(requests, profiles, upstream)
	identity := access.Identity{UserID: "alice", TenantID: 42}

	if _, err := service.Create(context.Background(), validCreateInput(), "create-note-0002", identity, nil); errorCode(err) != "space.profile_create_failed" {
		t.Fatalf("first Create() error = %v", err)
	}
	result, err := service.Create(context.Background(), validCreateInput(), "create-note-0002", identity, nil)
	if err != nil || !result.Reconciled || upstream.createCalls != 1 || len(profiles.items) != 1 {
		t.Fatalf("reconciled Create() = %+v, %v; creates=%d profiles=%d", result, err, upstream.createCalls, len(profiles.items))
	}
}

func TestCreateRejectsModifiedModeAndIdempotencyReuse(t *testing.T) {
	service, _ := NewService(
		&fakeRequests{},
		&fakeProfileStore{items: map[string]profile.Profile{}},
		&fakeUpstream{items: map[string]weknora.KnowledgeBase{}},
	)
	identity := access.Identity{UserID: "alice", TenantID: 42}
	modified := validCreateInput()
	modified.IndexProfile = "graph"
	if _, err := service.Create(context.Background(), modified, "create-note-0003", identity, nil); errorCode(err) != "knowledge_mode.disabled" {
		t.Fatalf("modified profile error = %v", err)
	}
	if _, err := service.Create(context.Background(), validCreateInput(), "same-key-0001", identity, nil); err != nil {
		t.Fatal(err)
	}
	different := validCreateInput()
	different.Name = "Different"
	if _, err := service.Create(context.Background(), different, "same-key-0001", identity, nil); errorCode(err) != "request.idempotency_conflict" {
		t.Fatalf("key reuse error = %v", err)
	}
}

func TestGetProfileAppliesOwnerOnlyPolicy(t *testing.T) {
	profiles := &fakeProfileStore{items: map[string]profile.Profile{
		"note-1": {
			UpstreamKBID: "note-1", TenantID: 42, OwnerUserID: "alice",
			ProductMode: profile.ModePersonalNotes, AccessPolicy: profile.PolicyOwnerOnly,
		},
	}}
	upstream := &fakeUpstream{items: map[string]weknora.KnowledgeBase{
		"note-1": {ID: "note-1", TenantID: 42, CreatorID: "alice"},
	}}
	service, _ := NewService(&fakeRequests{}, profiles, upstream)
	if _, err := service.GetProfile(context.Background(), "note-1", access.Identity{UserID: "alice", TenantID: 42}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetProfile(context.Background(), "note-1", access.Identity{UserID: "bob", TenantID: 42}, nil); errorCode(err) != "resource.not_found" {
		t.Fatalf("non-owner GetProfile() error = %v", err)
	}
}

func errorCode(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}
