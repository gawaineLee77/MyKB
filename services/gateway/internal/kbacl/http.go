package kbacl

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const routePrefix = "/api/v1/mindcreek/kb-permissions/"

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle(routePrefix, s.Handler())
	mux.Handle(creationPrefix, s.Handler())
	mux.Handle(creationPrefix+"/", s.Handler())
}
func (s *Service) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }
func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == creationPrefix || strings.HasPrefix(r.URL.Path, creationPrefix+"/") {
		data, err := s.creationHTTP(w, r)
		if err != nil {
			writeError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, routePrefix)
	parts := strings.Split(path, "/")
	if !strings.HasPrefix(r.URL.Path, routePrefix) || len(parts) > 2 || !validID(parts[0]) {
		writeError(w, fail(404, "not_found"))
		return
	}
	kb := parts[0]
	suffix := ""
	if len(parts) == 2 {
		suffix = parts[1]
	}
	var data any
	var err error
	switch {
	case r.Method == http.MethodGet && suffix == "effective":
		data, err = s.Effective(r.Context(), r.Header, kb)
	case r.Method == http.MethodGet && suffix == "":
		_, data, err = s.managerPolicy(r.Context(), r.Header, kb)
	case r.Method == http.MethodPut && suffix == "":
		var input PolicyInput
		if err = decode(w, r, &input); err == nil {
			data, err = s.UpdatePolicy(r.Context(), r.Header, kb, input)
		}
	case r.Method == http.MethodPost && suffix == "restore-creator":
		var input struct {
			Revision uint64 `json:"revision"`
		}
		if err = decode(w, r, &input); err == nil {
			data, err = s.RestoreCreator(r.Context(), r.Header, kb, input.Revision)
		}
	case r.Method == http.MethodGet && suffix == "members":
		data, err = s.memberSearch(r, kb)
	case r.Method == http.MethodGet && suffix == "events":
		var p Policy
		_, p, err = s.managerPolicy(r.Context(), r.Header, kb)
		if err == nil {
			data, err = s.Store.Events(r.Context(), p.TenantID, kb)
		}
	default:
		err = fail(404, "not_found")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fail(400, "request_invalid")
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return fail(400, "request_invalid")
	}
	return nil
}
func writeError(w http.ResponseWriter, err error) {
	code := "kb_acl.unavailable"
	status := 503
	var e *Error
	if errors.As(err, &e) {
		code = e.Code
		status = e.Status
	}
	if status < 400 || status > 599 {
		status = 503
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"code": code, "message": code}})
}

type MemberSummary struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

func (s *Service) memberSearch(r *http.Request, kb string) ([]MemberSummary, error) {
	p, _, err := s.managerPolicy(r.Context(), r.Header, kb)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len(q) > 100 {
		return nil, fail(400, "query_invalid")
	}
	for k := range r.URL.Query() {
		if k != "q" {
			return nil, fail(400, "query_invalid")
		}
	}
	roster, err := s.roster(r.Context(), r.Header, p.Tenant.ID)
	if err != nil {
		return nil, err
	}
	result := []MemberSummary{}
	for _, m := range roster {
		if m.Status != "active" || (!isEditor(m.Role) && m.Role != "viewer") || !validID(m.UserID) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(m.Username), q) && !strings.Contains(strings.ToLower(m.UserID), q) {
			continue
		}
		if err := s.IdentityAllowed(r.Context(), m.UserID); err != nil {
			if statusOf(err) == 403 {
				continue
			}
			return nil, err
		}
		result = append(result, MemberSummary{UserID: m.UserID, Username: m.Username, DisplayName: m.Username, Role: m.Role})
		if len(result) == 100 {
			break
		}
	}
	return result, nil
}
