package nativeaccess

import (
	"encoding/json"
	"testing"
)

func TestNativeModelDefaultsPreserveIndexingAndUnknownFields(t *testing.T) {
	_, f, _, actor := testScope()
	policy := &ModelPolicy{Upstream: f}
	r := requestJSON("POST", "/api/v1/knowledge-bases", `{"name":"native","summary_model_id":"existing-model","indexing_strategy":{"vector_enabled":false,"keyword_enabled":false,"wiki_enabled":true},"chunking_config":{"strategy":"auto","token_limit":777},"future_native_option":{"keep":true}}`)
	if err := policy.Check(r.Context(), r, actor); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.NewDecoder(r.Body).Decode(&got)
	if got["embedding_model_id"] != "builtin-mindcreek-embedding" || got["summary_model_id"] != "existing-model" {
		t.Fatal(got)
	}
	strategy := got["indexing_strategy"].(map[string]any)
	if strategy["wiki_enabled"] != true || strategy["vector_enabled"] != false || strategy["keyword_enabled"] != false {
		t.Fatal(strategy)
	}
	if got["chunking_config"].(map[string]any)["token_limit"] != float64(777) || got["future_native_option"].(map[string]any)["keep"] != true {
		t.Fatal(got)
	}
}

func TestInvalidExplicitModelsAndGraphFailWithoutReplacement(t *testing.T) {
	_, f, _, actor := testScope()
	policy := &ModelPolicy{Upstream: f}
	for _, body := range []string{`{"summary_model_id":"private-model"}`, `{"embedding_model_id":"builtin-mindcreek-chat"}`, `{"indexing_strategy":{"graph_enabled":true}}`, `{"extract_config":{"enabled":true}}`} {
		r := requestJSON("POST", "/api/v1/knowledge-bases", body)
		if err := policy.Check(r.Context(), r, actor); err == nil {
			t.Error(body)
		}
	}
	r := requestJSON("PUT", "/api/v1/initialization/config/k", `{"nodeExtract":{"enabled":true}}`)
	if err := policy.Check(r.Context(), r, actor); err == nil {
		t.Fatal("native wizard bypassed graph condition")
	}
}

func TestPartialNativeUpdatesDoNotInjectMissingConfig(t *testing.T) {
	_, f, _, actor := testScope()
	policy := &ModelPolicy{Upstream: f}
	for _, path := range []string{"/api/v1/knowledge-bases/k", "/api/v1/agents/a"} {
		r := requestJSON("PUT", path, `{"name":"renamed"}`)
		if err := policy.Check(r.Context(), r, actor); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		json.NewDecoder(r.Body).Decode(&got)
		if len(got) != 1 || got["name"] != "renamed" {
			t.Fatal(got)
		}
	}
}

func TestGraphPreviewRequiresDependenciesAndAuthorizedChatModel(t *testing.T) {
	for _, action := range []string{"fabri-tag", "fabri-text", "text-relation"} {
		t.Run(action, func(t *testing.T) {
			_, f, _, actor := testScope()
			p := &ModelPolicy{Upstream: f}
			path := "/api/v1/initialization/extract/" + action
			for _, state := range []struct {
				enabled bool
				engine  string
				body    string
				allow   bool
			}{
				{false, "Neo4j", `{}`, false},
				{true, "", `{}`, false},
				{true, "Neo4j", `{"model_id":"private-model"}`, false},
				{true, "Neo4j", `{"model_id":"builtin-mindcreek-embedding"}`, false},
				{true, "Neo4j", `{"model_id":42}`, false},
				{true, "Neo4j", `{"text":"synthetic","tags":["DEPENDS_ON"]}`, true},
			} {
				p.GraphEnabled, f.graphEngine = state.enabled, state.engine
				r := requestJSON("POST", path, state.body)
				err := p.Check(r.Context(), r, actor)
				if (err == nil) != state.allow {
					t.Fatalf("%+v: %v", state, err)
				}
				if state.allow {
					var got map[string]any
					json.NewDecoder(r.Body).Decode(&got)
					if got["model_id"] != "builtin-mindcreek-chat" || got["text"] != "synthetic" {
						t.Fatal(got)
					}
				}
			}
		})
	}
}
