package resourceacl

import (
	"context"
	"strings"
	"testing"
)

func delta(kind, id, text string, done bool) map[string]any {
	return map[string]any{"id": "request", "response_type": kind, "content": text, "done": done, "data": map[string]any{"event_id": id}}
}

func TestStreamRewriterHoldsEverySplitOfPhysicalLocator(t *testing.T) {
	original := "before ![图](storage://backend/local://7/exports/image.png) after"
	want := "before ![图](resource://abcdefghijklmnopqrstuv) after"
	for split := 0; split <= len(original); split++ {
		service := &Service{Catalog: &fakeCatalog{resource: Resource{TenantID: 7, Reference: "resource://abcdefghijklmnopqrstuv"}}, Access: &fakeAccess{tenant: 7}}
		stream := service.NewStreamRewriter(context.Background(), nil)
		first := delta("answer", "a", original[:split], false)
		second := delta("answer", "a", original[split:], true)
		if err := stream.Rewrite(first); err != nil {
			t.Fatal(split, err)
		}
		if text := first["content"].(string); strings.Contains(text, "local://") || strings.Contains(text, "storage://") {
			t.Fatal("partial locator escaped", split, text)
		}
		if err := stream.Rewrite(second); err != nil {
			t.Fatal(split, err)
		}
		got := first["content"].(string) + second["content"].(string)
		if got != want {
			t.Fatalf("split %d: got %q, want %q", split, got, want)
		}
		if flushed, err := stream.Flush(); err != nil || len(flushed) != 0 {
			t.Fatal("done duplicated stream tail", flushed, err)
		}
	}
}

func TestStreamRewriterInterleavingTerminalAndEOF(t *testing.T) {
	service := &Service{Catalog: &fakeCatalog{resource: Resource{TenantID: 7, Reference: "resource://abcdefghijklmnopqrstuv"}}, Access: &fakeAccess{tenant: 7}}
	stream := service.NewStreamRewriter(context.Background(), nil)
	answer := delta("answer", "a", "A ![](local://7/exports/image.png", false)
	thinking := delta("thinking", "t", "notes", false) // trailing s is a possible s3/storage prefix
	if err := stream.Rewrite(answer); err != nil {
		t.Fatal(err)
	}
	if err := stream.Rewrite(thinking); err != nil {
		t.Fatal(err)
	}
	flushed, err := stream.Before(map[string]any{"response_type": "complete", "done": true})
	if err != nil || len(flushed) != 2 {
		t.Fatal(flushed, err)
	}
	seen := map[string]string{}
	for _, raw := range flushed {
		event := raw.(map[string]any)
		seen[event["response_type"].(string)] = event["content"].(string)
		if event["data"].(map[string]any)["event_id"] == "" {
			t.Fatal("stream key lost")
		}
	}
	if seen["answer"] != "resource://abcdefghijklmnopqrstuv" || seen["thinking"] != "s" {
		t.Fatal(seen)
	}
	if flushed, err := stream.Flush(); err != nil || len(flushed) != 0 {
		t.Fatal("terminal tails repeated at EOF")
	}
	unfinished := delta("reflection", "r", "value loc", false)
	if err := stream.Rewrite(unfinished); err != nil {
		t.Fatal(err)
	}
	flushed, err = stream.Flush()
	if err != nil || len(flushed) != 1 || flushed[0].(map[string]any)["content"] != "loc" {
		t.Fatal("EOF lost ordinary prefix text", flushed, err)
	}
}

func TestStreamRewriterBoundsAndOpaqueHandles(t *testing.T) {
	service := &Service{Catalog: &fakeCatalog{resource: Resource{TenantID: 7, Reference: "resource://abcdefghijklmnopqrstuv"}}, Access: &fakeAccess{tenant: 7}}
	stream := service.NewStreamRewriter(context.Background(), nil)
	if err := stream.Rewrite(delta("answer", "a", "local://7/exports/"+strings.Repeat("x", 4096), false)); err == nil {
		t.Fatal("unbounded locator accepted")
	}
	stream = service.NewStreamRewriter(context.Background(), nil)
	text := "![图](resource://abcdefghijklmnopqrstuv)"
	event := delta("answer", "a", text, false)
	if err := stream.Rewrite(event); err != nil || event["content"] != text {
		t.Fatal("opaque native handle changed", event, err)
	}
}
