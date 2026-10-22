package resourceacl

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

const maxHeldResourceBytes = 4096
const maxHeldResourceStreams = 256

var physicalPrefixes = []string{"storage://", "local://", "minio://", "s3://", "oss://", "cos://", "tos://", "ks3://", "obs://"}

type heldResourceContent struct {
	text     string
	envelope map[string]any
}

// StreamRewriter is owned by one response reader. It retains incomplete native
// physical locators across delta events, including a prefix split as "loc" +
// "al://". Complete metadata fields still go through ordinary rewriting.
type StreamRewriter struct {
	service *Service
	ctx     context.Context
	headers http.Header
	held    map[string]heldResourceContent
}

func (s *Service) NewStreamRewriter(ctx context.Context, h http.Header) *StreamRewriter {
	return &StreamRewriter{service: s, ctx: ctx, headers: h.Clone(), held: map[string]heldResourceContent{}}
}

func deltaType(kind string) bool {
	return kind == "answer" || kind == "thinking" || kind == "reflection"
}

func locatorDelimiter(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("<>\"'`)]},", r)
}

// holdbackCutoff emits only text that cannot become a physical locator after
// appending another delta. Already opaque resource:// handles need no rewrite.
func holdbackCutoff(text string) int {
	cutoff := len(text)
	for _, prefix := range physicalPrefixes {
		if index := strings.LastIndex(text, prefix); index >= 0 && strings.IndexFunc(text[index:], locatorDelimiter) < 0 {
			if index < cutoff {
				cutoff = index
			}
		}
		for length := 1; length < len(prefix) && length <= len(text); length++ {
			if strings.HasSuffix(text, prefix[:length]) && len(text)-length < cutoff {
				cutoff = len(text) - length
			}
		}
	}
	return cutoff
}

func streamKey(envelope map[string]any, kind string) string {
	eventID := ""
	if data, ok := envelope["data"].(map[string]any); ok {
		eventID, _ = data["event_id"].(string)
	}
	if eventID == "" {
		eventID, _ = envelope["id"].(string)
	}
	return kind + "\x00" + eventID
}

// copyDeltaEnvelope keeps stream identity and metadata but never retains
// document/reference arrays already sent with the original event. The native
// client accumulates content using response_type and data.event_id.
func copyDeltaEnvelope(value map[string]any) (map[string]any, error) {
	envelope := map[string]any{"response_type": value["response_type"], "id": value["id"], "done": false}
	if data, ok := value["data"].(map[string]any); ok {
		raw, err := json.Marshal(data)
		if err != nil || len(raw) > 16<<10 {
			return nil, fail(502, "resource_stream_metadata_too_large")
		}
		var copied map[string]any
		if json.Unmarshal(raw, &copied) != nil {
			return nil, fail(502, "resource_stream_invalid")
		}
		envelope["data"] = copied
	}
	return envelope, nil
}

func (s *StreamRewriter) Rewrite(value any) error {
	envelope, ok := value.(map[string]any)
	if !ok {
		return s.service.RewriteResources(s.ctx, s.headers, value)
	}
	kind, _ := envelope["response_type"].(string)
	if !deltaType(kind) {
		return s.service.RewriteResources(s.ctx, s.headers, value)
	}
	key := streamKey(envelope, kind)
	content, _ := envelope["content"].(string)
	content = s.held[key].text + content
	cutoff := len(content)
	done, _ := envelope["done"].(bool)
	if !done {
		cutoff = holdbackCutoff(content)
	}
	if len(content)-cutoff > maxHeldResourceBytes || len(key) > 512 {
		return fail(502, "resource_stream_reference_too_large")
	}
	envelope["content"] = content[:cutoff]
	if err := s.service.RewriteResources(s.ctx, s.headers, envelope); err != nil {
		return err
	}
	if cutoff == len(content) {
		delete(s.held, key)
		return nil
	}
	metadata, err := copyDeltaEnvelope(envelope)
	if err != nil {
		return err
	}
	if _, existing := s.held[key]; !existing && len(s.held) >= maxHeldResourceStreams {
		return fail(502, "resource_stream_too_many_parts")
	}
	s.held[key] = heldResourceContent{text: content[cutoff:], envelope: metadata}
	return nil
}

// Before returns any pending delta tails before a terminal event. The caller
// must check/scrub these events just like every normal output event.
func (s *StreamRewriter) Before(value any) ([]any, error) {
	envelope, ok := value.(map[string]any)
	if !ok {
		return nil, nil
	}
	kind, _ := envelope["response_type"].(string)
	if kind == "complete" || kind == "error" {
		return s.Flush()
	}
	return nil, nil
}

// Flush is also called at upstream EOF, including user stop. Rewriting a
// truncated locator produces a non-authorizing marker; no raw path is emitted.
func (s *StreamRewriter) Flush() ([]any, error) {
	keys := make([]string, 0, len(s.held))
	for key := range s.held {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, key := range keys {
		part := s.held[key]
		part.envelope["content"] = part.text
		if err := s.service.RewriteResources(s.ctx, s.headers, part.envelope); err != nil {
			return nil, err
		}
		out = append(out, part.envelope)
	}
	clear(s.held)
	return out, nil
}
