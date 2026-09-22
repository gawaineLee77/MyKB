package nativeaccess

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *ScopeService) checkReferences(ctx context.Context, value any, state *requestState, h http.Header) error {
	refs := references{}
	collect(value, &refs)
	for _, id := range unique(refs.knowledge) {
		kb, err := s.Upstream.KnowledgeBaseForKnowledge(ctx, id, h)
		if err != nil {
			return err
		}
		refs.kbs = append(refs.kbs, kb)
	}
	for _, id := range unique(refs.chunks) {
		kb, err := s.Upstream.KnowledgeBaseForChunk(ctx, id, h)
		if err != nil {
			return err
		}
		refs.kbs = append(refs.kbs, kb)
	}
	if state.StrictScope && !subset(unique(refs.kbs), state.KBIDs) {
		return &Error{502, "upstream.scope_violation"}
	}
	for _, id := range unique(refs.kbs) {
		if err := s.CheckKB(ctx, id, h); err != nil {
			return err
		}
	}
	return nil
}

func (s *ScopeService) FilterResponse(response *http.Response) error {
	state, _ := response.Request.Context().Value(scopeContextKey{}).(*requestState)
	if state == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		response.Body = &checkedStream{source: response.Body, reader: bufio.NewReader(response.Body), check: func(v any) error {
			return s.checkReferences(response.Request.Context(), v, state, response.Request.Header)
		}}
		response.ContentLength = -1
		response.Header.Del("Content-Length")
		return nil
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "json") {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
	response.Body.Close()
	if err != nil || len(raw) > 16<<20 {
		return &Error{502, "upstream.response_too_large"}
	}
	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		return &Error{502, "upstream.invalid_response"}
	}
	ctx, h := response.Request.Context(), response.Request.Header
	if response.Request.URL.Path == "/api/v1/messages/chat-history-stats" {
		if data, ok := envelope["data"].(map[string]any); ok && data["enabled"] == true {
			return &Error{409, "history.aggregate_scope_unavailable"}
		}
	}
	if state.Create {
		data, ok := envelope["data"].(map[string]any)
		if !ok {
			return &Error{502, "upstream.invalid_response"}
		}
		id, ok := data["id"].(string)
		if !ok || !validID(id) {
			return &Error{502, "upstream.invalid_response"}
		}
		if err := s.Store.Bind(ctx, id, state.Actor, nil); err != nil {
			return err
		}
	}
	if response.Request.URL.Path == "/api/v1/sessions" && response.Request.Method == "GET" {
		items, ok := envelope["data"].([]any)
		if !ok {
			return &Error{502, "upstream.invalid_response"}
		}
		visible := []any{}
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return &Error{502, "upstream.invalid_response"}
			}
			id, _ := object["id"].(string)
			if _, err := s.CheckSession(ctx, id, state.Actor, h); err != nil {
				if denied(err) {
					continue
				}
				return err
			}
			visible = append(visible, item)
		}
		envelope["data"] = visible
		// Do not disclose counts of another credential's or historical sessions.
		// The custom list adapter supplies correct filtered pagination in R3.
		envelope["total"] = len(visible)
	}
	if err := s.checkReferences(ctx, envelope["data"], state, h); err != nil {
		return err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	return nil
}

// Validate one complete SSE event before exposing any bytes of that event. A
// rejected event ends this response; it does not retract earlier emitted data.
type checkedStream struct {
	source   io.ReadCloser
	reader   *bufio.Reader
	pending  []byte
	check    func(any) error
	finished bool
}

func (s *checkedStream) Close() error { return s.source.Close() }
func (s *checkedStream) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	if len(s.pending) > 0 {
		n := copy(target, s.pending)
		s.pending = s.pending[n:]
		return n, nil
	}
	if s.finished {
		return 0, io.EOF
	}
	var event, data bytes.Buffer
	for {
		var lineBuffer bytes.Buffer
		var err error
		for {
			fragment, readErr := s.reader.ReadSlice('\n')
			if event.Len()+lineBuffer.Len()+len(fragment) > 2<<20 {
				return 0, &Error{502, "upstream.event_too_large"}
			}
			lineBuffer.Write(fragment)
			if readErr != bufio.ErrBufferFull {
				err = readErr
				break
			}
		}
		line := lineBuffer.String()
		if event.Len()+len(line) > 2<<20 {
			return 0, &Error{502, "upstream.event_too_large"}
		}
		event.WriteString(line)
		trim := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trim, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(trim, "data:")))
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
		if trim == "" || err == io.EOF {
			if err == io.EOF {
				s.finished = true
			}
			break
		}
	}
	if data.Len() > 0 && data.String() != "[DONE]" {
		var value any
		if json.Unmarshal(data.Bytes(), &value) != nil {
			return 0, &Error{502, "upstream.invalid_event"}
		}
		if err := s.check(value); err != nil {
			return 0, err
		}
	}
	s.pending = event.Bytes()
	if len(s.pending) == 0 {
		return 0, io.EOF
	}
	return s.Read(target)
}

func denied(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Status == 403 || e.Status == 404
	}
	return upstreamDenied(err)
}
