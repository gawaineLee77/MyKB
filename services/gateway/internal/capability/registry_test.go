package capability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndDocumentAgreeOnEveryFlag(t *testing.T) {
	registry := loadTestRegistry(t, releaseValues)
	document := registry.Document("product-test", "v0.7.2")
	if len(document.Capabilities) != len(Keys()) {
		t.Fatalf("API flags=%d, known flags=%d", len(document.Capabilities), len(Keys()))
	}
	for _, key := range Keys() {
		if document.Capabilities[key] != registry.Capabilities[key] {
			t.Fatalf("API flag %q differs from configuration", key)
		}
	}
	if !document.KnowledgeModes[0].Enabled {
		t.Fatal("Personal Notes is not advertised after Gate B and P1-14")
	}
	if !document.KnowledgeModes[1].Enabled || !document.KnowledgeModes[1].Profiles[0].Enabled {
		t.Fatal("Plain RAG is not advertised")
	}
	if !document.Capabilities["mcp"] {
		t.Fatal("authenticated MCP is not advertised in Phase 4")
	}
	if !document.Capabilities["managed_models"] || document.Capabilities["user_model_overrides"] {
		t.Fatal("Phase 5 managed-model defaults or override default are incorrect")
	}
}

func TestLoadRetainsFrozenPhase4Contract(t *testing.T) {
	filename := writeRegistryForPhase(t, "phase4", phase4Values)
	registry, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Phase != "phase4" || len(registry.Capabilities) != len(phase4Values) {
		t.Fatalf("unexpected Phase 4 registry: %+v", registry)
	}
}

func TestUserModelOverridesMayBeExplicitlyEnabled(t *testing.T) {
	values := clone(releaseValues)
	values["user_model_overrides"] = true
	registry := loadTestRegistry(t, values)
	if !registry.Capabilities["user_model_overrides"] {
		t.Fatal("explicit override capability was not preserved")
	}
}

func TestLoadFailsClosedOnCapabilityDrift(t *testing.T) {
	values := clone(releaseValues)
	values["kb_personal_notes"] = false
	filename := writeRegistry(t, values)
	if _, err := Load(filename); err == nil {
		t.Fatal("Load() accepted a capability value that differs from this release")
	}
}

func TestLoadRejectsUnknownOrMissingFlags(t *testing.T) {
	values := clone(releaseValues)
	delete(values, "ontology")
	values["unknown"] = false
	filename := writeRegistry(t, values)
	if _, err := Load(filename); err == nil {
		t.Fatal("Load() accepted an unknown flag in place of a required flag")
	}
}

func loadTestRegistry(t *testing.T, values map[string]bool) *Registry {
	t.Helper()
	registry, err := Load(writeRegistry(t, values))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func writeRegistry(t *testing.T, values map[string]bool) string {
	return writeRegistryForPhase(t, "phase5", values)
}

func writeRegistryForPhase(t *testing.T, phase string, values map[string]bool) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "capabilities.json")
	payload, err := json.Marshal(Registry{SchemaVersion: 1, Phase: phase, Capabilities: values})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func clone(source map[string]bool) map[string]bool {
	copy := make(map[string]bool, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
