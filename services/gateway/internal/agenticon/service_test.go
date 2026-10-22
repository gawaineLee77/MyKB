package agenticon

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type principalFixture struct{ p weknora.Principal }

func (f *principalFixture) CurrentPrincipal(context.Context, http.Header) (weknora.Principal, error) {
	return f.p, nil
}

type iconMemory struct {
	items    map[string]Image
	putError error
}

func (s *iconMemory) Put(_ context.Context, i Image) error {
	if s.putError != nil {
		return s.putError
	}
	s.items[i.ID] = i
	return nil
}
func (s *iconMemory) Get(_ context.Context, t uint64, id string) (Image, error) {
	i, ok := s.items[id]
	if !ok || i.TenantID != t {
		return Image{}, ErrNotFound
	}
	return i, nil
}
func fixture() (*Service, *principalFixture, *iconMemory, http.Header, []byte) {
	p := &principalFixture{weknora.Principal{User: &weknora.User{ID: "alice", IsActive: true}, Tenant: &weknora.Tenant{ID: 7}, Memberships: []weknora.Membership{{TenantID: 7, Role: "contributor"}}}}
	m := &iconMemory{items: map[string]Image{}}
	h := http.Header{"Authorization": {"Bearer synthetic"}, "X-Tenant-Id": {"7"}}
	var out bytes.Buffer
	_ = png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	return &Service{Upstream: p, Store: m}, p, m, h, out.Bytes()
}
func status(err error) int {
	var e *nativeaccess.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

func TestImageUploadAndReadRespectHumanTenantAndAgentVisibility(t *testing.T) {
	s, p, _, h, raw := fixture()
	ctx := context.Background()
	ref, err := s.Upload(ctx, h, raw)
	if err != nil || len(ref) > 64 {
		t.Fatal(ref, err)
	}
	id := strings.TrimPrefix(ref, Prefix)
	if _, err = s.Read(ctx, h, id, ""); err != nil {
		t.Fatal("uploader preview", err)
	}
	p.p.User.ID = "bob"
	if _, err = s.Read(ctx, h, id, ""); status(err) != 404 {
		t.Fatal("private draft exposed", err)
	}
	if err = s.ValidateReference(ctx, h, "", ref); status(err) != 404 {
		t.Fatal("other draft adopted", err)
	}
	s.AgentAvatarForRead = func(context.Context, string, http.Header) (string, error) { return ref, nil }
	if _, err = s.Read(ctx, h, id, "visible-agent"); err != nil {
		t.Fatal("visible image", err)
	}
	if err = s.ValidateReference(ctx, h, "visible-agent", ref); err != nil {
		t.Fatal("keeping/copying existing visible image", err)
	}
	s.AgentAvatarForRead = func(context.Context, string, http.Header) (string, error) {
		return "", &nativeaccess.Error{Status: 404, Code: "agent.not_found"}
	}
	if _, err = s.Read(ctx, h, id, "hidden-agent"); status(err) != 404 {
		t.Fatal("revoked agent image leaked", err)
	}
	p.p.Tenant.ID = 8
	p.p.Memberships = []weknora.Membership{{TenantID: 8, Role: "owner"}}
	h.Set("X-Tenant-ID", "8")
	if _, err = s.Read(ctx, h, id, ""); status(err) != 404 {
		t.Fatal("cross tenant image", err)
	}
}

func TestImageValidationQuotasAndIdentityFailClosed(t *testing.T) {
	s, p, m, h, raw := fixture()
	ctx := context.Background()
	for _, value := range [][]byte{[]byte(`<svg onload="alert(1)"/>`), []byte("not an image"), make([]byte, 2<<20+1)} {
		if _, err := s.Upload(ctx, h, value); status(err) != 400 {
			t.Fatal("invalid upload", err)
		}
	}
	m.putError = ErrQuota
	if _, err := s.Upload(ctx, h, raw); status(err) != 409 {
		t.Fatal("quota", err)
	}
	m.putError = nil
	p.p.Memberships[0].Role = "viewer"
	if _, err := s.Upload(ctx, h, raw); status(err) != 403 {
		t.Fatal("viewer upload", err)
	}
	p.p.Memberships[0].Role = "contributor"
	h.Set("X-API-Key", "synthetic-key")
	if _, err := s.Upload(ctx, h, raw); status(err) != 401 {
		t.Fatal("machine upload", err)
	}
	h.Del("X-API-Key")
	s.IdentityAllowed = func(context.Context, string) error {
		return &nativeaccess.Error{Status: 403, Code: "identity.suspended"}
	}
	if _, err := s.Upload(ctx, h, raw); status(err) != 403 {
		t.Fatal("suspended upload", err)
	}
	if err := s.ValidateReference(ctx, h, "", "mc-avatar:../../secret"); err == nil {
		t.Fatal("malformed reference admitted")
	}
}

func TestImageHTTPReadsArePrivatePNGAndBounded(t *testing.T) {
	s, _, _, h, raw := fixture()
	ref, err := s.Upload(context.Background(), h, raw)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	r := httptest.NewRequest("GET", "/api/v1/mindcreek/agent-icons/"+strings.TrimPrefix(ref, Prefix), nil)
	r.Header = h.Clone()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal(w.Code, w.Header())
	}
	r = httptest.NewRequest("POST", "/api/v1/mindcreek/agent-icons", bytes.NewReader(make([]byte, 2<<20+1)))
	r.Header = h.Clone()
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatal("unbounded upload", w.Code)
	}
}
