package branding

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/enterprise"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type memoryStore struct {
	record Record
	actor  string
	fail   bool
}

func (s *memoryStore) Get(context.Context) (Record, error) {
	if s.fail {
		return Record{}, errors.New("db secret")
	}
	return s.record, nil
}
func (s *memoryStore) Update(_ context.Context, c Change, actor, _ string) (Record, error) {
	if s.fail {
		return Record{}, errors.New("db secret")
	}
	if s.record.Revision != c.Revision {
		return Record{}, ErrConflict
	}
	s.record.Name = c.Name
	s.record.Revision++
	s.actor = actor
	if c.ReplaceLogo {
		s.record.Logo = c.Logo
	}
	return s.record, nil
}

type installation struct{}

func (installation) Installation(context.Context) (enterprise.Installation, error) {
	return enterprise.Installation{AdminUserID: "installed"}, nil
}
func admin() weknora.Principal {
	return weknora.Principal{User: &weknora.User{ID: "installed", IsActive: true, IsSystemAdmin: true}}
}
func TestOnlyInstalledActiveHumanAdminCanManage(t *testing.T) {
	s := &Service{Store: &memoryStore{record: Record{Name: DefaultName, Revision: 1}}, Installations: installation{}}
	for _, p := range []weknora.Principal{{}, {User: &weknora.User{ID: "other", IsActive: true, IsSystemAdmin: true}}, {User: &weknora.User{ID: "installed", IsActive: false, IsSystemAdmin: true}}, {User: &weknora.User{ID: "installed", IsActive: true}}} {
		if _, err := s.Update(context.Background(), p, Input{Name: "全站", Revision: 1}, ""); err == nil {
			t.Fatal("unauthorized update")
		}
	}
	if _, err := s.Update(context.Background(), admin(), Input{Name: "新名称", Revision: 1}, "audit"); err != nil {
		t.Fatal(err)
	}
}
func TestNameLogoLifecycleAndConflict(t *testing.T) {
	store := &memoryStore{record: Record{Name: DefaultName, Revision: 1}}
	s := &Service{Store: store, Installations: installation{}}
	var img bytes.Buffer
	_ = png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	data, _ := json.Marshal("data:image/png;base64," + base64.StdEncoding.EncodeToString(img.Bytes()))
	got, err := s.Update(context.Background(), admin(), Input{Name: "  小纳实验室  ", Revision: 1, LogoData: data}, "synthetic")
	if err != nil || got.Name != "小纳实验室" || got.Revision != 2 || got.Snapshot().LogoURL != Route+"/logo?v=2" || store.actor != "installed" {
		t.Fatal(got, err)
	}
	_, err = s.Update(context.Background(), admin(), Input{Name: "stale", Revision: 1}, "")
	var e *Error
	if !errors.As(err, &e) || e.Status != 409 {
		t.Fatal("stale edit", err)
	}
	got, err = s.Update(context.Background(), admin(), Input{Name: "再改名", Revision: 2}, "")
	if err != nil || len(got.Logo) == 0 {
		t.Fatal("omission removed image", err)
	}
	got, err = s.Update(context.Background(), admin(), Input{Name: "再改名", Revision: 3, LogoData: json.RawMessage(`null`)}, "")
	if err != nil || len(got.Logo) != 0 || got.Snapshot().LogoURL != "" {
		t.Fatal("reset", err)
	}
	for _, name := range []string{"", "\n", strings.Repeat("名", 65), "name\u202Eabc", "<script>", "two\nlines"} {
		if _, err = s.Update(context.Background(), admin(), Input{Name: name, Revision: 4}, ""); err == nil {
			t.Fatal("invalid name", name)
		}
	}
	store.fail = true
	_, err = s.Get(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("private DB error", err)
	}
}
