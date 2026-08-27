package ingestion

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/access"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/preset"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type profileStub struct{ value profile.Profile }

func (s profileStub) Get(context.Context, string) (profile.Profile, error) { return s.value, nil }

type upstreamStub struct {
	uploadCalls int
	retryCalls  int
	cancelCalls int
}

func (s *upstreamStub) GetKnowledgeBase(context.Context, string, http.Header) (weknora.KnowledgeBase, error) {
	return weknora.KnowledgeBase{ID: "kb-rag", TenantID: 42}, nil
}
func (s *upstreamStub) UploadKnowledge(_ context.Context, kbID, filename string, source io.Reader, _ http.Header) (weknora.Knowledge, error) {
	s.uploadCalls++
	content, _ := io.ReadAll(source)
	return weknora.Knowledge{ID: "doc-1", KnowledgeBaseID: kbID, Type: "file", FileName: filename, FileSize: int64(len(content)), ParseStatus: "pending"}, nil
}
func (s *upstreamStub) ListKnowledge(context.Context, string, int, int, http.Header) (weknora.KnowledgePage, error) {
	return weknora.KnowledgePage{Items: []weknora.Knowledge{documentFixture()}, Total: 1, Page: 1, PageSize: 20}, nil
}
func (s *upstreamStub) GetKnowledge(context.Context, string, string, http.Header) (weknora.Knowledge, error) {
	return documentFixture(), nil
}
func (s *upstreamStub) ReparseKnowledge(context.Context, string, string, http.Header) (weknora.Knowledge, error) {
	s.retryCalls++
	return documentFixture(), nil
}
func (s *upstreamStub) CancelKnowledge(context.Context, string, string, http.Header) (weknora.Knowledge, error) {
	s.cancelCalls++
	return documentFixture(), nil
}

func TestApprovedPlainRAGLifecycle(t *testing.T) {
	upstream := &upstreamStub{}
	service := newTestService(t, plainProfile(t), upstream)
	identity := access.Identity{UserID: "alice", TenantID: 42}
	for _, filename := range []string{"guide.md", "guide.pdf", "guide.docx", "guide.xlsx"} {
		result, err := service.Upload(context.Background(), "kb-rag", filename, 9, strings.NewReader("synthetic"), identity, nil)
		if err != nil || result.FileName != filename {
			t.Fatalf("Upload(%q) = %+v, %v", filename, result, err)
		}
	}
	page, err := service.List(context.Background(), "kb-rag", 1, 20, identity, nil)
	if err != nil || page.Total != 1 {
		t.Fatalf("List() = %+v, %v", page, err)
	}
	if _, err := service.Retry(context.Background(), "kb-rag", "doc-1", identity, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Cancel(context.Background(), "kb-rag", "doc-1", identity, nil); err != nil {
		t.Fatal(err)
	}
	if upstream.uploadCalls != 4 || upstream.retryCalls != 1 || upstream.cancelCalls != 1 {
		t.Fatalf("calls = %+v", upstream)
	}
}

func TestInvalidTypeAndFutureProfilesStopBeforeUpstream(t *testing.T) {
	identity := access.Identity{UserID: "alice", TenantID: 42}
	t.Run("type", func(t *testing.T) {
		upstream := &upstreamStub{}
		service := newTestService(t, plainProfile(t), upstream)
		_, err := service.Upload(context.Background(), "kb-rag", "blocked.exe", 4, strings.NewReader("body"), identity, nil)
		assertCode(t, err, "ingestion.file_type_unsupported")
		if upstream.uploadCalls != 0 {
			t.Fatal("invalid type reached upstream")
		}
	})
	t.Run("personal notes", func(t *testing.T) {
		productProfile := plainProfile(t)
		productProfile.ProductMode, productProfile.IndexProfile = profile.ModePersonalNotes, "notes_plain"
		upstream := &upstreamStub{}
		service := newTestService(t, productProfile, upstream)
		_, err := service.Upload(context.Background(), "kb-rag", "blocked.pdf", 4, strings.NewReader("body"), identity, nil)
		assertCode(t, err, "rag.plain_profile_required")
	})
	t.Run("graph modified", func(t *testing.T) {
		definition, _ := preset.Build(profile.ModeRAG, "embedding-1", "")
		definition.Config.Indexing.GraphEnabled = true
		encoded, _ := definition.JSON()
		productProfile := plainProfile(t)
		productProfile.EffectiveConfig = encoded
		upstream := &upstreamStub{}
		service := newTestService(t, productProfile, upstream)
		_, err := service.Upload(context.Background(), "kb-rag", "blocked.pdf", 4, strings.NewReader("body"), identity, nil)
		assertCode(t, err, "rag.profile_invalid")
		if upstream.uploadCalls != 0 {
			t.Fatal("graph profile reached upstream")
		}
	})
}

func plainProfile(t *testing.T) profile.Profile {
	t.Helper()
	definition, err := preset.Build(profile.ModeRAG, "embedding-1", "summary-1")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := definition.JSON()
	return profile.Profile{UpstreamKBID: "kb-rag", TenantID: 42, OwnerUserID: "alice", ProductMode: profile.ModeRAG,
		AccessPolicy: profile.PolicyUpstream, IndexProfile: "plain", IndexProfileVersion: preset.Version, EffectiveConfig: encoded}
}

func newTestService(t *testing.T, productProfile profile.Profile, upstream *upstreamStub) *Service {
	t.Helper()
	service, err := NewService(profileStub{value: productProfile}, upstream)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func documentFixture() weknora.Knowledge {
	return weknora.Knowledge{ID: "doc-1", KnowledgeBaseID: "kb-rag", Type: "file", FileName: "guide.md", ParseStatus: "failed", ErrorMessage: "synthetic failure"}
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	typed, ok := err.(*Error)
	if !ok || typed.Code != code {
		t.Fatalf("error=%+v, want %s", err, code)
	}
}
