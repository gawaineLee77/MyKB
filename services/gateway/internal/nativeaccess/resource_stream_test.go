package nativeaccess

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestResourceStreamFlushesBeforeTerminalAndAtEOF(t *testing.T) {
	for _, ending := range []string{
		"event: message\ndata: {\"response_type\":\"complete\",\"content\":\"\"}\n\n",
		"data: [DONE]\n\n",
		"",
	} {
		t.Run(ending, func(t *testing.T) {
			source := io.NopCloser(strings.NewReader(ending))
			pending := true
			flush := func() ([]any, error) {
				if !pending {
					return nil, nil
				}
				pending = false
				return []any{map[string]any{"response_type": "answer", "content": "held-tail", "data": map[string]any{"event_id": "original"}}}, nil
			}
			checks := 0
			stream := &checkedStream{source: source, reader: bufio.NewReader(source), scrub: true,
				check:  func(any) error { checks++; return nil },
				before: func(any) ([]any, error) { return flush() },
				flush:  flush}
			raw, err := io.ReadAll(stream)
			if err != nil {
				t.Fatal(err)
			}
			result := string(raw)
			if strings.Count(result, "held-tail") != 1 || !strings.Contains(result, `"event_id":"original"`) || checks == 0 {
				t.Fatal(result, checks)
			}
			for _, terminal := range []string{`"complete"`, "[DONE]"} {
				if at := strings.Index(result, terminal); at >= 0 && at < strings.Index(result, "held-tail") {
					t.Fatal("tail emitted after terminal", result)
				}
			}
		})
	}
}

func TestReleasedImageTailStillPassesResponseAuthorization(t *testing.T) {
	source := io.NopCloser(strings.NewReader(""))
	stream := &checkedStream{source: source, reader: bufio.NewReader(source), scrub: true,
		check: func(any) error { return &Error{403, "scope.denied"} },
		flush: func() ([]any, error) {
			return []any{map[string]any{"content": "should-not-escape"}}, nil
		}}
	raw, err := io.ReadAll(stream)
	if err == nil || len(raw) != 0 {
		t.Fatal("denied held content escaped", string(raw), err)
	}
}
