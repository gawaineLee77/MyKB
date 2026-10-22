package resourceacl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLegacyImageReferencesBecomeOpaqueWithoutCreatingOwnership(t *testing.T) {
	catalog := &fakeCatalog{resource: Resource{TenantID: 7, Reference: "resource://abcdefghijklmnopqrstuv", Owners: nil}}
	s := &Service{Catalog: catalog, Access: &fakeAccess{tenant: 7}}
	value := map[string]any{
		"content":    "文字 before ![image](local://7/exports/image.png) after",
		"image_info": `[{"url":"local://7/exports/image.png","caption":"原说明"}]`,
	}
	if err := s.RewriteResources(context.Background(), nil, value); err != nil {
		t.Fatal(err)
	}
	if value["content"] != "文字 before ![image](resource://abcdefghijklmnopqrstuv) after" {
		t.Fatal(value)
	}
	var imageInfo []map[string]any
	if json.Unmarshal([]byte(value["image_info"].(string)), &imageInfo) != nil ||
		imageInfo[0]["url"] != catalog.resource.Reference || imageInfo[0]["caption"] != "原说明" {
		t.Fatal(value)
	}
	// Rewriting a locator cannot grant that resource to the visible document.
	assertStatus(t, s.AuthorizeMessage(context.Background(), nil, "s", "m", []string{"kb"}, catalog.resource.Reference), 409)
}

func TestMissingLegacyImageDoesNotEraseParagraphOrExposePath(t *testing.T) {
	s := &Service{Catalog: &fakeCatalog{err: ErrNotFound}, Access: &fakeAccess{tenant: 7}}
	value := map[string]any{"content": "before ![](storage://backend/local://7/exports/missing.png) after"}
	if err := s.RewriteResources(context.Background(), nil, value); err != nil {
		t.Fatal(err)
	}
	text := value["content"].(string)
	if strings.Contains(text, "local://") || strings.Contains(text, "storage://") || !strings.HasPrefix(text, "before ") || !strings.HasSuffix(text, " after") {
		t.Fatal(text)
	}
}
