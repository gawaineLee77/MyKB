// Package capability loads and exposes MindCreek's authoritative feature set.
package capability

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

var phase4Values = map[string]bool{
	"im":                  false,
	"miniprogram":         false,
	"cli":                 false,
	"embed":               false,
	"browser_extension":   false,
	"web_search":          false,
	"mcp":                 true,
	"asr":                 false,
	"data_analysis":       false,
	"external_connectors": false,
	"kb_personal_notes":   true,
	"rag_plain":           true,
	"rag_graph":           false,
	"rag_pixel":           false,
	"ontology":            false,
}

var releaseValues = func() map[string]bool {
	values := make(map[string]bool, len(phase4Values)+2)
	for key, value := range phase4Values {
		values[key] = value
	}
	values["managed_models"] = true
	values["user_model_overrides"] = false
	return values
}()

// Registry is the validated, immutable capability source for this process.
type Registry struct {
	SchemaVersion int             `json:"schema_version"`
	Phase         string          `json:"phase"`
	Capabilities  map[string]bool `json:"capabilities"`
}

// Document is returned by the capability API.
type Document struct {
	SchemaVersion   int             `json:"schema_version"`
	Phase           string          `json:"phase"`
	ProductVersion  string          `json:"product_version"`
	UpstreamVersion string          `json:"upstream_version"`
	Capabilities    map[string]bool `json:"capabilities"`
	KnowledgeModes  []KnowledgeMode `json:"knowledge_modes"`
}

type KnowledgeMode struct {
	ID       string         `json:"id"`
	Enabled  bool           `json:"enabled"`
	Profiles []IndexProfile `json:"profiles,omitempty"`
}

type IndexProfile struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// Load reads a capability registry and supports the frozen Phase 4 contract and
// the current Phase 5 Gate A contract.
func Load(filename string) (*Registry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open capability registry: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	var registry Registry
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("decode capability registry: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, err
	}
	if err := registry.validate(); err != nil {
		return nil, err
	}
	return &registry, nil
}

func (r *Registry) validate() error {
	if r.SchemaVersion != 1 || (r.Phase != "phase4" && r.Phase != "phase5") {
		return fmt.Errorf("capability registry must use schema_version 1 and a supported phase")
	}
	expected := releaseValues
	if r.Phase == "phase4" {
		expected = phase4Values
	}
	if len(r.Capabilities) != len(expected) {
		return fmt.Errorf("capability registry has %d flags; expected %d", len(r.Capabilities), len(expected))
	}
	for key, expectedValue := range expected {
		actual, ok := r.Capabilities[key]
		if !ok {
			return fmt.Errorf("capability registry is missing %q", key)
		}
		if key != "user_model_overrides" && actual != expectedValue {
			return fmt.Errorf("capability %q must be %t in the %s release", key, expectedValue, r.Phase)
		}
	}
	return nil
}

// Document returns a defensive copy plus knowledge modes derived from flags.
func (r *Registry) Document(productVersion, upstreamVersion string) Document {
	flags := make(map[string]bool, len(r.Capabilities))
	for key, enabled := range r.Capabilities {
		flags[key] = enabled
	}
	return Document{
		SchemaVersion:   r.SchemaVersion,
		Phase:           r.Phase,
		ProductVersion:  productVersion,
		UpstreamVersion: upstreamVersion,
		Capabilities:    flags,
		KnowledgeModes: []KnowledgeMode{
			{ID: "personal_notes", Enabled: flags["kb_personal_notes"]},
			{ID: "rag", Enabled: flags["rag_plain"] || flags["rag_graph"] || flags["rag_pixel"], Profiles: []IndexProfile{
				{ID: "plain", Enabled: flags["rag_plain"]},
				{ID: "graph", Enabled: flags["rag_graph"]},
				{ID: "pixel", Enabled: flags["rag_pixel"]},
			}},
			{ID: "ontology", Enabled: flags["ontology"]},
		},
	}
}

// Keys returns the stable sorted flag names for diagnostics and tests.
func Keys() []string {
	keys := make([]string, 0, len(releaseValues))
	for key := range releaseValues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("capability registry must contain one JSON document")
		}
		return fmt.Errorf("decode capability registry suffix: %w", err)
	}
	return nil
}
