package nativeaccess

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// List sessions with accurate pagination after product binding checks. Native
// list is still called with the original credential and filters on every page,
// so this does not grant machine chat capability or bypass upstream ownership.
func (s *ScopeService) Local(w http.ResponseWriter, r *http.Request, actor Actor) (bool, error) {
	if r.Method != "GET" || r.URL.Path != "/api/v1/sessions" {
		return false, nil
	}
	page, size := 1, 20
	var err error
	if value := r.URL.Query().Get("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 || page > 10000 {
			return true, &Error{400, "request.invalid_pagination"}
		}
	}
	if value := r.URL.Query().Get("page_size"); value != "" {
		size, err = strconv.Atoi(value)
		if err != nil || size < 1 || size > 100 {
			return true, &Error{400, "request.invalid_pagination"}
		}
	}
	visible := []json.RawMessage{}
	query := r.URL.Query()
	query.Set("page_size", "100")
	for current := 1; current <= 101; current++ {
		query.Set("page", strconv.Itoa(current))
		raw, err := s.Upstream.NativeRequest(r.Context(), "GET", "/api/v1/sessions", query, r.Header, nil)
		if err != nil {
			return true, err
		}
		var envelope struct {
			Success bool              `json:"success"`
			Data    []json.RawMessage `json:"data"`
			Total   int               `json:"total"`
		}
		if json.Unmarshal(raw, &envelope) != nil || !envelope.Success {
			return true, &Error{502, "upstream.invalid_response"}
		}
		if envelope.Total > 10000 {
			return true, &Error{422, "session.history_limit"}
		}
		for _, item := range envelope.Data {
			var value struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(item, &value) != nil || value.ID == "" {
				return true, &Error{502, "upstream.invalid_response"}
			}
			if _, err := s.CheckSession(r.Context(), value.ID, actor, r.Header); err != nil {
				if denied(err) {
					continue
				}
				return true, err
			}
			visible = append(visible, item)
		}
		if current*100 >= envelope.Total || len(envelope.Data) == 0 {
			break
		}
		if current == 101 {
			return true, &Error{422, "session.history_limit"}
		}
	}
	total := len(visible)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	return true, json.NewEncoder(w).Encode(map[string]any{"success": true, "data": visible[start:end], "total": total, "page": page, "page_size": size})
}
