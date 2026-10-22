package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/resourceacl"
)

// This command is deliberately local-only. It has no public HTTP equivalent.
// The installation operator reviews ownership outside editable document content
// before explicitly approving an immutable resource snapshot.
func handleImageResources(ctx context.Context, db *sql.DB, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: image-resources audit|apply --tenant ID")
	}
	flags := flag.NewFlagSet("image-resources", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tenant := flags.Uint64("tenant", 0, "workspace ID")
	limit := flags.Int("limit", 1000, "maximum resources to inspect")
	after := flags.String("after", "", "audit page cursor")
	confirmed := flags.Bool("confirm-reviewed", false, "ownership was independently reviewed")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *tenant == 0 {
		return fmt.Errorf("invalid arguments; provide --tenant ID")
	}
	catalog := resourceacl.NewRepository(db)
	switch args[0] {
	case "audit":
		report, err := catalog.AuditPage(ctx, *tenant, *limit, *after)
		if err != nil {
			return fmt.Errorf("resource inspection unavailable")
		}
		return json.NewEncoder(out).Encode(report)
	case "apply":
		if !*confirmed {
			return fmt.Errorf("apply requires --confirm-reviewed after independent ownership review")
		}
		var manifest struct {
			TenantID uint64 `json:"tenant_id"`
			Items    []struct {
				Reference   string `json:"reference"`
				KnowledgeID string `json:"knowledge_id"`
				Fingerprint string `json:"fingerprint"`
				Reason      string `json:"reason"`
			} `json:"items"`
		}
		decoder := json.NewDecoder(io.LimitReader(in, 4<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&manifest) != nil || manifest.TenantID != *tenant || len(manifest.Items) == 0 || len(manifest.Items) > 1000 {
			return fmt.Errorf("invalid reviewed manifest (tenant, items, reference, knowledge_id, fingerprint, reason required)")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return fmt.Errorf("invalid trailing manifest data")
		}
		for _, item := range manifest.Items {
			if item.Reference == "" || item.KnowledgeID == "" || item.Fingerprint == "" || item.Reason == "" {
				return fmt.Errorf("every item requires a reviewed document, fingerprint and reason")
			}
		}
		for index, item := range manifest.Items {
			if err := catalog.Approve(ctx, *tenant, item.Reference, item.KnowledgeID, item.Fingerprint, item.Reason); err != nil {
				return fmt.Errorf("stopped at item %d; %d previous items applied; verify current resource/document and retry the reviewed manifest", index+1, index)
			}
		}
		return json.NewEncoder(out).Encode(map[string]any{"tenant_id": *tenant, "approved": len(manifest.Items)})
	default:
		return fmt.Errorf("expected audit or apply")
	}
}
