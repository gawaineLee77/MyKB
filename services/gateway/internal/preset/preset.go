// Package preset defines immutable product-approved indexing configurations.
package preset

import (
	"encoding/json"
	"fmt"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const (
	Version          = 1
	KBTypeDocument   = "document"
	KBTypeFAQ        = "faq"
	MaxRAGFileBytes  = int64(500 << 20)
	MaxRAGFiles      = 1000
	MaxNoteFileBytes = int64(64 << 10)
	MaxPersonalNotes = 500
)

type Models struct {
	EmbeddingModelID string `json:"embedding_model_id"`
	SummaryModelID   string `json:"summary_model_id,omitempty"`
	RerankModelID    string `json:"rerank_model_id,omitempty"`
	VLMModelID       string `json:"vlm_model_id,omitempty"`
}

type Retrieval struct {
	Mode          string `json:"mode"`
	VectorTopK    int    `json:"vector_top_k"`
	KeywordTopK   int    `json:"keyword_top_k"`
	FinalTopK     int    `json:"final_top_k"`
	RerankEnabled bool   `json:"rerank_enabled"`
	RerankModelID string `json:"rerank_model_id,omitempty"`
}

type Limits struct {
	MaxFileBytes int64 `json:"max_file_bytes"`
	MaxFiles     int   `json:"max_files"`
}

type EffectiveConfig struct {
	ProfileID         string                        `json:"profile_id"`
	ProfileVersion    int                           `json:"profile_version"`
	KnowledgeBaseType string                        `json:"knowledge_base_type,omitempty"`
	Storage           weknora.StorageProviderConfig `json:"storage"`
	Chunking          weknora.ChunkingConfig        `json:"chunking"`
	Indexing          weknora.IndexingStrategy      `json:"indexing"`
	Retrieval         Retrieval                     `json:"retrieval"`
	Models            Models                        `json:"models"`
	Limits            Limits                        `json:"limits"`
}

type Definition struct {
	Mode         profile.ProductMode
	AccessPolicy profile.AccessPolicy
	Config       EffectiveConfig
}

func Build(mode profile.ProductMode, embeddingModelID, summaryModelID string) (Definition, error) {
	return BuildWithRerank(mode, embeddingModelID, summaryModelID, "")
}

// BuildWithRerank records the complete server-selected Phase 5 model set in
// the immutable product profile. WeKnora's retrieval pipeline auto-selects the
// available default reranker; this field makes that product decision explicit
// for diagnostics and future migrations.
func BuildWithRerank(mode profile.ProductMode, embeddingModelID, summaryModelID, rerankModelID string) (Definition, error) {
	return BuildWithManagedModels(mode, embeddingModelID, summaryModelID, rerankModelID, "")
}

// BuildWithManagedModels records the full organization-managed model set. A
// VLM is optional so existing three-model deployments remain upgrade-safe.
// When present, only Document RAG enables it; Personal Notes remain text-only.
func BuildWithManagedModels(mode profile.ProductMode, embeddingModelID, summaryModelID, rerankModelID, vlmModelID string) (Definition, error) {
	if embeddingModelID == "" {
		return Definition{}, fmt.Errorf("embedding model is required")
	}
	definition := Definition{
		Mode: mode,
		Config: EffectiveConfig{
			ProfileVersion: Version,
			Storage:        weknora.StorageProviderConfig{Provider: "local"},
			Indexing:       weknora.IndexingStrategy{VectorEnabled: true, KeywordEnabled: true},
			Models:         Models{EmbeddingModelID: embeddingModelID, SummaryModelID: summaryModelID, RerankModelID: rerankModelID, VLMModelID: vlmModelID},
			Limits:         Limits{MaxFileBytes: MaxRAGFileBytes, MaxFiles: MaxRAGFiles},
		},
	}
	definition.Config.Chunking.Separators = []string{"\n\n", "\n", "。", ". "}
	switch mode {
	case profile.ModePersonalNotes:
		definition.AccessPolicy = profile.PolicyOwnerOnly
		definition.Config.ProfileID = "notes_plain"
		definition.Config.Chunking.ChunkSize = 800
		definition.Config.Chunking.ChunkOverlap = 80
		definition.Config.Chunking.Strategy = "heading"
		definition.Config.Retrieval = Retrieval{Mode: "hybrid", VectorTopK: 12, KeywordTopK: 12, FinalTopK: 8, RerankEnabled: rerankModelID != "", RerankModelID: rerankModelID}
		definition.Config.Limits = Limits{MaxFileBytes: MaxNoteFileBytes, MaxFiles: MaxPersonalNotes}
	case profile.ModeRAG:
		definition.AccessPolicy = profile.PolicyUpstream
		definition.Config.ProfileID = "plain"
		definition.Config.Chunking.ChunkSize = 1024
		definition.Config.Chunking.ChunkOverlap = 128
		definition.Config.Chunking.Strategy = "auto"
		definition.Config.Retrieval = Retrieval{Mode: "hybrid", VectorTopK: 20, KeywordTopK: 20, FinalTopK: 10, RerankEnabled: rerankModelID != "", RerankModelID: rerankModelID}
	default:
		return Definition{}, fmt.Errorf("unsupported preset mode %q", mode)
	}
	return definition, nil
}

// ForKnowledgeBaseType applies an upstream-native knowledge-base type without
// introducing a fourth MindCreek product mode. FAQ remains part of the governed
// RAG family while using WeKnora's dedicated FAQ storage and retrieval path.
func (d Definition) ForKnowledgeBaseType(knowledgeBaseType string) (Definition, error) {
	switch knowledgeBaseType {
	case "", KBTypeDocument:
		// Keep the legacy document profile JSON byte-for-byte compatible. An
		// omitted type means document; only the non-default FAQ type is recorded.
		d.Config.KnowledgeBaseType = ""
	case KBTypeFAQ:
		if d.Mode != profile.ModeRAG {
			return Definition{}, fmt.Errorf("FAQ knowledge bases require RAG mode")
		}
		d.Config.KnowledgeBaseType = KBTypeFAQ
		d.Config.Models.VLMModelID = ""
	default:
		return Definition{}, fmt.Errorf("unsupported knowledge-base type %q", knowledgeBaseType)
	}
	return d, nil
}

func (d Definition) JSON() (json.RawMessage, error) {
	encoded, err := json.Marshal(d.Config)
	if err != nil {
		return nil, fmt.Errorf("encode effective preset: %w", err)
	}
	return encoded, nil
}

func (d Definition) UpstreamRequest(id, name, description string) weknora.CreateKnowledgeBaseRequest {
	knowledgeBaseType := d.Config.KnowledgeBaseType
	if knowledgeBaseType == "" {
		knowledgeBaseType = KBTypeDocument
	}
	vlmEnabled := d.Mode == profile.ModeRAG && knowledgeBaseType == KBTypeDocument && d.Config.Models.VLMModelID != ""
	request := weknora.CreateKnowledgeBaseRequest{
		ID: id, Name: name, Description: description, Type: knowledgeBaseType,
		EmbeddingModelID: d.Config.Models.EmbeddingModelID, SummaryModelID: d.Config.Models.SummaryModelID,
		VLMConfig:             weknora.VLMConfig{Enabled: vlmEnabled, ModelID: enabledModelID(vlmEnabled, d.Config.Models.VLMModelID)},
		StorageProviderConfig: d.Config.Storage, ChunkingConfig: d.Config.Chunking, IndexingStrategy: d.Config.Indexing,
		QuestionGenerationConfig: weknora.QuestionGenerationConfig{Enabled: false, QuestionCount: 0},
	}
	if knowledgeBaseType == KBTypeFAQ {
		// Match the defaults selected by WeKnora's native FAQ creation UI.
		request.FAQConfig = &weknora.FAQConfig{IndexMode: "question_only", QuestionIndexMode: "separate"}
	}
	return request
}

func enabledModelID(enabled bool, id string) string {
	if !enabled {
		return ""
	}
	return id
}
