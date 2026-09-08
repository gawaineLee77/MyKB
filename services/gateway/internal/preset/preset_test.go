package preset

import (
	"encoding/json"
	"testing"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
)

func TestPlainRAGPresetIsReproducibleAndFutureProfilesAreOff(t *testing.T) {
	definition, err := Build(profile.ModeRAG, "embedding-1", "summary-1")
	if err != nil {
		t.Fatal(err)
	}
	request := definition.UpstreamRequest("kb-1", "Documents", "Synthetic")
	if definition.Config.ProfileID != "plain" || definition.Config.ProfileVersion != 1 ||
		!request.IndexingStrategy.VectorEnabled || !request.IndexingStrategy.KeywordEnabled ||
		request.IndexingStrategy.GraphEnabled || request.IndexingStrategy.WikiEnabled ||
		definition.Config.Retrieval.RerankEnabled || request.StorageProviderConfig.Provider != "local" ||
		definition.Config.Limits.MaxFileBytes != MaxRAGFileBytes {
		t.Fatalf("unexpected preset: %+v request=%+v", definition, request)
	}
	first, _ := definition.JSON()
	second, _ := definition.JSON()
	if string(first) != string(second) || !json.Valid(first) {
		t.Fatalf("preset JSON is not deterministic: %s / %s", first, second)
	}
}

func TestPersonalNotesUsesSmallerApprovedBudget(t *testing.T) {
	definition, err := Build(profile.ModePersonalNotes, "embedding-1", "")
	if err != nil || definition.Config.ProfileID != "notes_plain" || definition.Config.Limits.MaxFileBytes != MaxNoteFileBytes {
		t.Fatalf("Build() = %+v, %v", definition, err)
	}
}

func TestPhase5PresetRecordsManagedReranker(t *testing.T) {
	definition, err := BuildWithRerank(profile.ModeRAG, "embedding-1", "chat-1", "rerank-1")
	if err != nil {
		t.Fatal(err)
	}
	if !definition.Config.Retrieval.RerankEnabled || definition.Config.Retrieval.RerankModelID != "rerank-1" || definition.Config.Models.RerankModelID != "rerank-1" {
		t.Fatalf("Phase 5 model set = %+v", definition.Config)
	}
}

func TestPlainRAGEnablesOptionalManagedVLM(t *testing.T) {
	definition, err := BuildWithManagedModels(profile.ModeRAG, "embedding-1", "chat-1", "rerank-1", "vlm-1")
	if err != nil {
		t.Fatal(err)
	}
	request := definition.UpstreamRequest("kb-1", "Scanned books", "Synthetic")
	if definition.Config.Models.VLMModelID != "vlm-1" || !request.VLMConfig.Enabled || request.VLMConfig.ModelID != "vlm-1" || request.VLMConfig.DescriptionLanguage != "" {
		t.Fatalf("VLM preset = %+v request=%+v", definition.Config.Models, request.VLMConfig)
	}
}

func TestPersonalNotesNeverEnablesManagedVLM(t *testing.T) {
	definition, err := BuildWithManagedModels(profile.ModePersonalNotes, "embedding-1", "chat-1", "rerank-1", "vlm-1")
	if err != nil {
		t.Fatal(err)
	}
	request := definition.UpstreamRequest("kb-1", "Notes", "Synthetic")
	if request.VLMConfig.Enabled || request.VLMConfig.ModelID != "" {
		t.Fatalf("personal notes unexpectedly enabled VLM: %+v", request.VLMConfig)
	}
}
