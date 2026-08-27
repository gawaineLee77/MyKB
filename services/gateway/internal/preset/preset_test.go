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
		definition.Config.Retrieval.RerankEnabled || request.StorageProviderConfig.Provider != "local" {
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
	if err != nil || definition.Config.ProfileID != "notes_plain" || definition.Config.Limits.MaxFileBytes != 64<<10 {
		t.Fatalf("Build() = %+v, %v", definition, err)
	}
}
