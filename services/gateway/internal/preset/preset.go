// Package preset defines immutable product-approved indexing configurations.
package preset

import (
	"encoding/json"
	"fmt"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const Version = 1

type Models struct {
	EmbeddingModelID string `json:"embedding_model_id"`
	SummaryModelID   string `json:"summary_model_id,omitempty"`
	RerankModelID    string `json:"rerank_model_id,omitempty"`
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
	ProfileID      string                        `json:"profile_id"`
	ProfileVersion int                           `json:"profile_version"`
	Storage        weknora.StorageProviderConfig `json:"storage"`
	Chunking       weknora.ChunkingConfig        `json:"chunking"`
	Indexing       weknora.IndexingStrategy      `json:"indexing"`
	Retrieval      Retrieval                     `json:"retrieval"`
	Models         Models                        `json:"models"`
	Limits         Limits                        `json:"limits"`
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
	if embeddingModelID == "" {
		return Definition{}, fmt.Errorf("embedding model is required")
	}
	definition := Definition{
		Mode: mode,
		Config: EffectiveConfig{
			ProfileVersion: Version,
			Storage:        weknora.StorageProviderConfig{Provider: "local"},
			Indexing:       weknora.IndexingStrategy{VectorEnabled: true, KeywordEnabled: true},
			Models:         Models{EmbeddingModelID: embeddingModelID, SummaryModelID: summaryModelID, RerankModelID: rerankModelID},
			Limits:         Limits{MaxFileBytes: 50 << 20, MaxFiles: 1000},
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
		definition.Config.Limits = Limits{MaxFileBytes: 64 << 10, MaxFiles: 500}
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

func (d Definition) JSON() (json.RawMessage, error) {
	encoded, err := json.Marshal(d.Config)
	if err != nil {
		return nil, fmt.Errorf("encode effective preset: %w", err)
	}
	return encoded, nil
}

func (d Definition) UpstreamRequest(id, name, description string) weknora.CreateKnowledgeBaseRequest {
	return weknora.CreateKnowledgeBaseRequest{
		ID: id, Name: name, Description: description, Type: "document",
		EmbeddingModelID: d.Config.Models.EmbeddingModelID, SummaryModelID: d.Config.Models.SummaryModelID,
		StorageProviderConfig: d.Config.Storage, ChunkingConfig: d.Config.Chunking, IndexingStrategy: d.Config.Indexing,
		QuestionGenerationConfig: weknora.QuestionGenerationConfig{Enabled: false, QuestionCount: 0},
	}
}
