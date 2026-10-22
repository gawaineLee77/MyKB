package nativeaccess

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// FilterAgentVisibility prevents an Agent configuration from disclosing private
// KB identifiers through its metadata endpoints. Execution still independently
// intersects the caller's current KB visibility before every retrieval.
func (s *ScopeService) FilterAgentVisibility(response *http.Response) error {
	if s.KnowledgeAccess == nil || response.StatusCode < 200 || response.StatusCode >= 300 || response.Request.Method != "GET" {
		return nil
	}
	path := response.Request.URL.Path
	list := path == "/api/v1/agents"
	parts := strings.Split(strings.Trim(path, "/"), "/")
	detail := len(parts) == 4 && parts[2] == "agents" && parts[3] != "type-presets" && parts[3] != "placeholders"
	if !list && !detail {
		return nil
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "json") {
		return &Error{502, "upstream.invalid_response"}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
	response.Body.Close()
	if err != nil || len(raw) > 16<<20 {
		return &Error{502, "upstream.response_too_large"}
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		return &Error{502, "upstream.invalid_response"}
	}
	visible := func(value any) (bool, error) {
		agent, ok := value.(map[string]any)
		if !ok {
			return false, &Error{502, "upstream.invalid_response"}
		}
		config, ok := agent["config"].(map[string]any)
		if !ok {
			return false, &Error{502, "upstream.invalid_response"}
		}
		for _, kb := range stringsValue(config["knowledge_bases"]) {
			if err := s.CheckKB(response.Request.Context(), kb, response.Request.Header); err != nil {
				if denied(err) {
					return false, nil
				}
				return false, err
			}
		}
		return true, nil
	}
	if list {
		rows, ok := envelope["data"].([]any)
		if envelope["data"] == nil {
			rows = []any{}
			ok = true
		}
		if !ok {
			return &Error{502, "upstream.invalid_response"}
		}
		filtered := []any{}
		allowedIDs := []string{}
		for _, row := range rows {
			ok, err := visible(row)
			if err != nil {
				return err
			}
			if ok {
				filtered = append(filtered, row)
				id, _ := row.(map[string]any)["id"].(string)
				allowedIDs = append(allowedIDs, id)
			}
		}
		envelope["data"] = filtered
		if _, ok := envelope["total"]; ok {
			envelope["total"] = len(filtered)
		}
		disabled := []string{}
		for _, id := range stringsValue(envelope["disabled_own_agent_ids"]) {
			if subset([]string{id}, allowedIDs) {
				disabled = append(disabled, id)
			}
		}
		envelope["disabled_own_agent_ids"] = disabled
	} else {
		ok, err := visible(envelope["data"])
		if err != nil {
			return err
		}
		if !ok {
			return &Error{404, "agent.not_found"}
		}
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	response.Header.Set("Cache-Control", "private, no-store")
	return nil
}
