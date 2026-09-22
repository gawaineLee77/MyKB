package nativeaccess

import "testing"

func TestPinnedRoutesFailClosed(t *testing.T) {
	routes, err := LoadRoutes("../../../../config/r3-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		present      bool
		disposition  string
	}{
		{"POST", "/api/v1/knowledge-bases", true, "native"},
		{"POST", "/api/v1/knowledge-bases/k/knowledge/manual", true, "native"},
		{"GET", "/api/v1/knowledgebase/k/wiki/pages", true, "native"},
		{"GET", "/api/v1/knowledge/k/download", true, "native"},
		{"GET", "/api/v1/knowledge/k/preview", true, "native"},
		{"PATCH", "/api/v1/knowledge-bases", false, ""},
		{"POST", "/api/v1/knowledge-bases/k/arbitrary-new-action", false, ""},
		{"GET", "/api/v1/embed/c/config", true, "disabled"},
	} {
		r, ok := routes.Match(tc.method, tc.path)
		if ok != tc.present || r.Disposition != tc.disposition {
			t.Errorf("%s %s = %v %s", tc.method, tc.path, ok, r.Disposition)
		}
	}
}

func TestRetiredNeverCatchesNativeDocuments(t *testing.T) {
	for _, path := range []string{"/api/v1/knowledge-spaces", "/api/v1/knowledge-bases/k/notes/n", "/api/v1/knowledge-bases/k/ingestions", "/api/v1/mindcreek/knowledge-bases/k/grants", "/api/v1/mindcreek/me/subscriptions"} {
		if !Retired(path) {
			t.Error(path)
		}
	}
	for _, path := range []string{"/api/v1/knowledge-bases/k/knowledge/manual", "/api/v1/knowledge-bases/k/shares", "/api/v1/knowledgebase/k/wiki/pages", "/api/v1/mindcreek/onboarding", "/api/v1/mindcreek/models"} {
		if Retired(path) {
			t.Error(path)
		}
	}
}
