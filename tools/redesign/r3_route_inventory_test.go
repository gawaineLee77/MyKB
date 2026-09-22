package router

import (
	"encoding/json"
	"os"
	"testing"
)

// Execute alongside the product-owned Phase 1 inventory overlay. No upstream
// source changes or initialized handlers are needed to enumerate registered routes.
func TestMindCreekR3RouteInventory(t *testing.T) {
	routes := mindCreekUpstreamRoutes(t)
	if len(routes) != 429 {
		t.Fatalf("pinned route count changed: %d", len(routes))
	}
	items := make([]map[string]string, 0, len(routes))
	for _, route := range routes {
		items = append(items, map[string]string{"method": route.Method, "path": route.Path, "handler": route.Handler})
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("MINDCREEK_R3_INVENTORY_OUTPUT"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
