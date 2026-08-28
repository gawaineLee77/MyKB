// Package access enforces product authorization before WeKnora receives a request.
package access

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/audit"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/authorization"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/notespolicy"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/profile"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/publication"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

const maxFilteredResponseBytes = 16 << 20
const maxAuthorizationBodyBytes = 52 << 20

type Identity struct {
	UserID   string
	TenantID uint64
}

type identityKey struct{}
type phase3RequestKey struct{}

type phase3Request struct {
	Identity      Identity
	KBIDs         []string
	Action        authorization.Action
	Path          string
	CorrelationID string
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}

type ProfileStore interface {
	Get(context.Context, string) (profile.Profile, error)
	ForbiddenPersonalNoteIDs(context.Context, string) (map[string]struct{}, error)
}

type ResourceResolver interface {
	KnowledgeBaseForKnowledge(context.Context, string, http.Header) (string, error)
	KnowledgeBaseForChunk(context.Context, string, http.Header) (string, error)
	ValidateSession(context.Context, string, http.Header) error
	AgentKnowledgeBases(context.Context, string, http.Header) (weknora.AgentScope, error)
}

type Gate struct {
	profiles     ProfileStore
	resolver     ResourceResolver
	actions      ActionMatcher
	decisions    DecisionService
	sessions     SessionScopeStore
	auditor      audit.Recorder
	revisions    RevisionRecorder
	publications PublicationLifecycle
}

type ActionMatcher interface {
	Match(string, string) (authorization.Action, bool)
}

type DecisionService interface {
	Decide(context.Context, string, authorization.Principal, http.Header) (authorization.Decision, error)
	Authorize(context.Context, string, authorization.Principal, authorization.Action, http.Header) (authorization.Decision, error)
}

type SessionScopeStore interface {
	ListKnowledgeBases(context.Context, string) ([]string, error)
	RecordKnowledgeBases(context.Context, string, []string, time.Time) error
}

type RevisionRecorder interface {
	Increment(context.Context, string, string, string, string, string, time.Time) (int64, error)
}

type PublicationLifecycle interface {
	UnpublishForDeletion(context.Context, string, publication.Actor, string, http.Header) error
}

type Error struct {
	Code       string
	Message    string
	StatusCode int
	Err        error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func NewGate(profiles ProfileStore, resolver ResourceResolver) (*Gate, error) {
	if profiles == nil || resolver == nil {
		return nil, fmt.Errorf("profile store and resource resolver are required")
	}
	return &Gate{profiles: profiles, resolver: resolver}, nil
}

func NewPhase2Gate(profiles ProfileStore, resolver ResourceResolver, actions ActionMatcher, decisions DecisionService, sessions SessionScopeStore, recorders ...audit.Recorder) (*Gate, error) {
	if profiles == nil || resolver == nil || actions == nil || decisions == nil {
		return nil, fmt.Errorf("profile store, resource resolver, route actions, and decisions are required")
	}
	if len(recorders) > 1 {
		return nil, fmt.Errorf("at most one access audit recorder is supported")
	}
	gate := &Gate{profiles: profiles, resolver: resolver, actions: actions, decisions: decisions, sessions: sessions}
	if len(recorders) == 1 {
		gate.auditor = recorders[0]
	}
	return gate, nil
}

func NewPhase3Gate(profiles ProfileStore, resolver ResourceResolver, actions ActionMatcher, decisions DecisionService,
	sessions SessionScopeStore, auditor audit.Recorder, revisions RevisionRecorder, publications PublicationLifecycle) (*Gate, error) {
	if revisions == nil || publications == nil {
		return nil, fmt.Errorf("Phase 3 revision recorder and publication lifecycle are required")
	}
	gate, err := NewPhase2Gate(profiles, resolver, actions, decisions, sessions, auditor)
	if err != nil {
		return nil, err
	}
	gate.revisions = revisions
	gate.publications = publications
	return gate, nil
}

// AuthorizeRequest protects direct KB and indirect source/chunk routes.
func (g *Gate) AuthorizeRequest(ctx context.Context, request *http.Request, identity Identity) error {
	if isUnscopedDerivedTaskPath(request.URL.Path) {
		// v0.7.2 task progress endpoints expose only a tenant-scoped opaque
		// task ID, not a resolvable parent KB. Deny until a product-owned task
		// mapping can prove Note Space ownership.
		return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
	}
	operation := operationFor(request)
	var action authorization.Action
	if g.decisions != nil {
		var matched bool
		action, matched = g.actions.Match(request.Method, request.URL.Path)
		if !matched {
			return &Error{Code: "security.action_unclassified", Message: "Resource policy is unavailable", StatusCode: http.StatusServiceUnavailable}
		}
	}
	kbIDs, err := g.resolvePathKBIDs(ctx, request)
	if err != nil {
		return err
	}
	scope, err := g.resolveRequestScope(ctx, request)
	if err != nil {
		return err
	}
	if g.decisions != nil && strings.TrimSuffix(request.URL.Path, "/") == "/api/v1/messages/search" && len(scope.sessionIDs) == 0 {
		return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
	}
	kbIDs = append(kbIDs, scope.kbIDs...)
	for _, knowledgeID := range unique(scope.knowledgeIDs) {
		kbID, err := g.resolver.KnowledgeBaseForKnowledge(ctx, knowledgeID, request.Header)
		if err != nil {
			return translateResolverError(err)
		}
		kbIDs = append(kbIDs, kbID)
	}
	for _, agentID := range unique(scope.agentIDs) {
		agentScope, err := g.resolver.AgentKnowledgeBases(ctx, agentID, request.Header)
		if err != nil {
			return translateResolverError(err)
		}
		kbIDs = append(kbIDs, agentScope.KnowledgeBaseIDs...)
	}
	sessionIDs := unique(append(sessionIDsFromPath(request.URL.Path), scope.sessionIDs...))
	for _, sessionID := range sessionIDs {
		if err := g.resolver.ValidateSession(ctx, sessionID, request.Header); err != nil {
			return translateResolverError(err)
		}
		if g.sessions != nil {
			stored, err := g.sessions.ListKnowledgeBases(ctx, sessionID)
			if err != nil {
				return &Error{Code: "security.session_scope_unavailable", Message: "Session policy is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
			}
			kbIDs = append(kbIDs, stored...)
		}
	}
	resolvedKBIDs := unique(kbIDs)
	for _, kbID := range resolvedKBIDs {
		var err error
		if g.decisions != nil {
			err = g.authorizeKBAction(ctx, kbID, identity, action, request)
		} else {
			err = g.authorizeKB(ctx, kbID, identity, operation)
		}
		if err != nil {
			return err
		}
	}
	if g.publications != nil && action == authorization.ActionDelete {
		correlationID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if correlationID == "" {
			return &Error{Code: "publication.unavailable", Message: "Publication lifecycle is unavailable", StatusCode: http.StatusServiceUnavailable}
		}
		for _, kbID := range resolvedKBIDs {
			if err := g.publications.UnpublishForDeletion(ctx, kbID, publication.Actor{UserID: identity.UserID, TenantID: identity.TenantID}, correlationID, request.Header); err != nil {
				return &Error{Code: "publication.unavailable", Message: "Publication lifecycle is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
			}
		}
	}
	if g.sessions != nil && len(resolvedKBIDs) > 0 {
		for _, sessionID := range sessionIDs {
			if err := g.sessions.RecordKnowledgeBases(ctx, sessionID, resolvedKBIDs, time.Now().UTC()); err != nil {
				return &Error{Code: "security.session_scope_unavailable", Message: "Session policy is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
			}
		}
	}
	if g.revisions != nil && len(resolvedKBIDs) > 0 {
		*request = *request.WithContext(context.WithValue(request.Context(), phase3RequestKey{}, phase3Request{
			Identity: identity, KBIDs: resolvedKBIDs, Action: action, Path: request.URL.Path,
			CorrelationID: strings.TrimSpace(request.Header.Get("X-Request-ID")),
		}))
	}
	return nil
}

type requestScope struct {
	kbIDs        []string
	knowledgeIDs []string
	agentIDs     []string
	sessionIDs   []string
}

func (g *Gate) resolveRequestScope(_ context.Context, request *http.Request) (requestScope, error) {
	var scope requestScope
	query := request.URL.Query()
	scope.kbIDs = append(scope.kbIDs, query["knowledge_base_id"]...)
	scope.kbIDs = append(scope.kbIDs, query["knowledge_base_ids"]...)
	scope.knowledgeIDs = append(scope.knowledgeIDs, query["knowledge_id"]...)
	scope.knowledgeIDs = append(scope.knowledgeIDs, query["knowledge_ids"]...)
	if isKnowledgeBatchPath(request.URL.Path) {
		scope.knowledgeIDs = append(scope.knowledgeIDs, query["ids"]...)
	}
	if agentID := query.Get("agent_id"); agentID != "" {
		scope.agentIDs = append(scope.agentIDs, agentID)
	}
	if request.Body == nil || !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		return scope, nil
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxAuthorizationBodyBytes+1))
	request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) > maxAuthorizationBodyBytes {
		return requestScope{}, &Error{Code: "request.body_too_large", Message: "Request body is too large for authorization", StatusCode: http.StatusRequestEntityTooLarge, Err: err}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return scope, nil
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return requestScope{}, &Error{Code: "request.invalid_json", Message: "Request body is not valid JSON", StatusCode: http.StatusBadRequest, Err: err}
	}
	collectScope(document, &scope, isKnowledgeBatchPath(request.URL.Path), isSessionBatchPath(request.URL.Path))
	return scope, nil
}

func collectScope(value any, scope *requestScope, collectGenericKnowledgeIDs, collectGenericSessionIDs bool) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectScope(item, scope, collectGenericKnowledgeIDs, collectGenericSessionIDs)
		}
	case map[string]any:
		resourceType, _ := typed["type"].(string)
		resourceID, _ := typed["id"].(string)
		if resourceID != "" {
			switch resourceType {
			case "kb", "knowledge_base":
				scope.kbIDs = append(scope.kbIDs, resourceID)
			case "agent":
				scope.agentIDs = append(scope.agentIDs, resourceID)
			}
		}
		for key, item := range typed {
			switch key {
			case "knowledge_base_id", "kb_id", "source_id", "target_id":
				scope.kbIDs = append(scope.kbIDs, stringValues(item)...)
			case "knowledge_base_ids", "knowledge_bases":
				scope.kbIDs = append(scope.kbIDs, stringValues(item)...)
			case "knowledge_id", "knowledge_ids":
				scope.knowledgeIDs = append(scope.knowledgeIDs, stringValues(item)...)
			case "agent_id":
				scope.agentIDs = append(scope.agentIDs, stringValues(item)...)
			case "session_id", "session_ids":
				scope.sessionIDs = append(scope.sessionIDs, stringValues(item)...)
			case "ids":
				if collectGenericKnowledgeIDs {
					scope.knowledgeIDs = append(scope.knowledgeIDs, stringValues(item)...)
				}
				if collectGenericSessionIDs {
					scope.sessionIDs = append(scope.sessionIDs, stringValues(item)...)
				}
			}
			collectScope(item, scope, collectGenericKnowledgeIDs, collectGenericSessionIDs)
		}
	}
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		return one(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func (g *Gate) resolvePathKBIDs(ctx context.Context, request *http.Request) ([]string, error) {
	segments := splitPath(request.URL.Path)
	if len(segments) < 3 || segments[0] != "api" || segments[1] != "v1" {
		return nil, nil
	}
	resource := segments[2]
	switch resource {
	case "knowledge-bases":
		if len(segments) >= 4 && segments[3] != "copy" {
			return []string{segments[3]}, nil
		}
	case "knowledgebase":
		if len(segments) >= 4 {
			return []string{segments[3]}, nil
		}
	case "knowledge":
		if len(segments) >= 5 && (segments[3] == "manual" || segments[3] == "image") {
			kbID, err := g.resolver.KnowledgeBaseForKnowledge(ctx, segments[4], request.Header)
			return one(kbID), translateResolverError(err)
		}
		if len(segments) >= 4 && !isKnowledgeCollectionPath(segments[3]) {
			kbID, err := g.resolver.KnowledgeBaseForKnowledge(ctx, segments[3], request.Header)
			return one(kbID), translateResolverError(err)
		}
	case "chunks":
		if len(segments) >= 5 && segments[3] == "by-id" {
			kbID, err := g.resolver.KnowledgeBaseForChunk(ctx, segments[4], request.Header)
			return one(kbID), translateResolverError(err)
		}
		if len(segments) >= 4 {
			kbID, err := g.resolver.KnowledgeBaseForKnowledge(ctx, segments[3], request.Header)
			return one(kbID), translateResolverError(err)
		}
	case "agents":
		if len(segments) >= 4 && !isAgentCollectionPath(segments[3]) {
			scope, err := g.resolver.AgentKnowledgeBases(ctx, segments[3], request.Header)
			return scope.KnowledgeBaseIDs, translateResolverError(err)
		}
	case "user":
		if len(segments) >= 6 && segments[3] == "favorites" {
			switch segments[4] {
			case "kb", "knowledge_base":
				return one(segments[5]), nil
			case "agent":
				scope, err := g.resolver.AgentKnowledgeBases(ctx, segments[5], request.Header)
				return scope.KnowledgeBaseIDs, translateResolverError(err)
			}
		}
	case "initialization":
		if len(segments) >= 5 && (segments[3] == "config" || segments[3] == "initialize") {
			return one(segments[4]), nil
		}
	}
	return nil, nil
}

func (g *Gate) authorizeKB(ctx context.Context, kbID string, identity Identity, operation notespolicy.Operation) error {
	kbProfile, err := g.profiles.Get(ctx, kbID)
	if errors.Is(err, profile.ErrNotFound) {
		return nil
	}
	if err != nil {
		return &Error{Code: "security.profile_unavailable", Message: "Knowledge-base policy is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	err = notespolicy.Authorize(kbProfile, notespolicy.Principal{UserID: identity.UserID, TenantID: identity.TenantID}, operation)
	if err == nil {
		return nil
	}
	var policyError *notespolicy.Error
	if errors.As(err, &policyError) {
		return &Error{Code: policyError.Code, Message: policyError.Message, StatusCode: policyError.StatusCode, Err: err}
	}
	return &Error{Code: "security.authorization_failed", Message: "Authorization failed", StatusCode: http.StatusInternalServerError, Err: err}
}

func (g *Gate) authorizeKBAction(ctx context.Context, kbID string, identity Identity, action authorization.Action, request *http.Request) error {
	principal := authorization.Principal{UserID: identity.UserID, TenantID: identity.TenantID}
	decision, err := g.decisions.Authorize(ctx, kbID, principal, action, request.Header)
	if err == nil {
		if isOriginalSourceDownload(request.URL.Path) &&
			(decision.Source == authorization.SourceSubscription || decision.Source == authorization.SourceOrganizationPublic) {
			g.recordDenied(ctx, kbID, identity, action, request.Header)
			return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound, Err: authorization.ErrDenied}
		}
		return nil
	}
	if errors.Is(err, authorization.ErrDenied) && action == authorization.ActionConfigure && isLimitedEditorConfiguration(request) {
		decision, decideErr := g.decisions.Decide(ctx, kbID, principal, request.Header)
		if decideErr == nil && decision.Role == authorization.RoleEditor {
			return nil
		}
		if decideErr != nil {
			err = decideErr
		}
	}
	switch {
	case errors.Is(err, authorization.ErrDenied), errors.Is(err, authorization.ErrNotFound):
		g.recordDenied(ctx, kbID, identity, action, request.Header)
		return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound, Err: err}
	case errors.Is(err, authorization.ErrInvalid):
		return &Error{Code: "security.authorization_invalid", Message: "Authorization request is invalid", StatusCode: http.StatusBadRequest, Err: err}
	default:
		return &Error{Code: "security.authorization_unavailable", Message: "Authorization service is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
}

func isOriginalSourceDownload(requestPath string) bool {
	return strings.HasSuffix(strings.TrimSuffix(requestPath, "/"), "/download")
}

func isLimitedEditorConfiguration(request *http.Request) bool {
	if request.Method != http.MethodPut || request.Body == nil {
		return false
	}
	segments := splitPath(request.URL.Path)
	if len(segments) != 4 || segments[0] != "api" || segments[1] != "v1" || segments[2] != "knowledge-bases" {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, (64<<10)+1))
	request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) == 0 || len(body) > 64<<10 {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || len(fields) == 0 {
		return false
	}
	for key := range fields {
		if key != "name" && key != "description" {
			return false
		}
	}
	var name string
	if raw, ok := fields["name"]; !ok || json.Unmarshal(raw, &name) != nil || strings.TrimSpace(name) == "" || len([]rune(name)) > 120 {
		return false
	}
	if raw, ok := fields["description"]; ok {
		var description string
		if json.Unmarshal(raw, &description) != nil || len([]rune(description)) > 1000 {
			return false
		}
	}
	return true
}

func (g *Gate) recordDenied(ctx context.Context, kbID string, identity Identity, action authorization.Action, headers http.Header) {
	if g.auditor == nil || !highValueAction(action) {
		return
	}
	correlationID := strings.TrimSpace(headers.Get("X-Request-ID"))
	if correlationID == "" {
		return
	}
	_ = g.auditor.Record(ctx, audit.Event{
		TenantID: identity.TenantID, KnowledgeBaseID: kbID, ActorUserID: identity.UserID,
		Action: "authorization.denied." + string(action), TargetType: "knowledge_base", TargetID: kbID,
		Outcome: audit.OutcomeDenied, ErrorCode: "resource.not_found", CorrelationID: correlationID,
		CreatedAt: time.Now().UTC(),
	})
}

func highValueAction(action authorization.Action) bool {
	switch action {
	case authorization.ActionEditContent, authorization.ActionConfigure, authorization.ActionManageGrants, authorization.ActionDelete:
		return true
	default:
		return false
	}
}

// FilterResponse removes Note Spaces not owned by the authenticated caller.
func (g *Gate) FilterResponse(response *http.Response) error {
	if g.revisions != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
		if metadata, ok := response.Request.Context().Value(phase3RequestKey{}).(phase3Request); ok {
			if eventType := revisionEvent(metadata.Action, metadata.Path); eventType != "" {
				if metadata.CorrelationID == "" {
					return &Error{Code: "activity.unavailable", Message: "Knowledge-base activity is unavailable", StatusCode: http.StatusServiceUnavailable}
				}
				for _, kbID := range metadata.KBIDs {
					if _, err := g.revisions.Increment(response.Request.Context(), kbID, metadata.Identity.UserID, eventType, "", metadata.CorrelationID, time.Now().UTC()); err != nil {
						return &Error{Code: "activity.unavailable", Message: "Knowledge-base activity is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
					}
				}
			}
		}
	}
	if response.Request.Method != http.MethodGet || !isFilterableListPath(response.Request.URL.Path) || response.StatusCode != http.StatusOK {
		return nil
	}
	identity, ok := IdentityFromContext(response.Request.Context())
	if !ok {
		return &Error{Code: "auth.principal_invalid", Message: "Authenticated principal is missing", StatusCode: http.StatusBadGateway}
	}
	if g.decisions != nil {
		return g.filterPhase2Response(response, identity)
	}
	forbidden, err := g.profiles.ForbiddenPersonalNoteIDs(response.Request.Context(), identity.UserID)
	if err != nil {
		return &Error{Code: "security.profile_unavailable", Message: "Knowledge-base policy is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
	}
	if len(forbidden) == 0 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxFilteredResponseBytes+1))
	response.Body.Close()
	if err != nil || len(body) > maxFilteredResponseBytes {
		return &Error{Code: "upstream.invalid_response", Message: "Upstream list response cannot be filtered", StatusCode: http.StatusBadGateway, Err: err}
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &Error{Code: "upstream.invalid_response", Message: "Upstream list response cannot be filtered", StatusCode: http.StatusBadGateway, Err: err}
	}
	items, setItems, ok := filterableItems(envelope)
	if !ok {
		return &Error{Code: "upstream.invalid_response", Message: "Upstream KB list has an unexpected shape", StatusCode: http.StatusBadGateway}
	}
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if !listItemReferencesForbiddenKB(response.Request.URL.Path, object, forbidden) {
			filtered = append(filtered, item)
		}
	}
	setItems(filtered)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	return nil
}

func revisionEvent(action authorization.Action, requestPath string) string {
	switch action {
	case authorization.ActionEditContent:
		return "kb.content_updated"
	case authorization.ActionDelete:
		return "kb.deleted"
	case authorization.ActionConfigure:
		segments := splitPath(requestPath)
		if len(segments) >= 4 && segments[0] == "api" && segments[1] == "v1" &&
			(segments[2] == "knowledge-bases" || segments[2] == "initialization") {
			return "kb.content_updated"
		}
	}
	return ""
}

func (g *Gate) filterPhase2Response(response *http.Response, identity Identity) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, maxFilteredResponseBytes+1))
	response.Body.Close()
	if err != nil || len(body) > maxFilteredResponseBytes {
		return &Error{Code: "upstream.invalid_response", Message: "Upstream list response cannot be filtered", StatusCode: http.StatusBadGateway, Err: err}
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &Error{Code: "upstream.invalid_response", Message: "Upstream list response cannot be filtered", StatusCode: http.StatusBadGateway, Err: err}
	}
	items, setItems, ok := filterableItems(envelope)
	if !ok {
		return &Error{Code: "upstream.invalid_response", Message: "Upstream KB list has an unexpected shape", StatusCode: http.StatusBadGateway}
	}
	principal := authorization.Principal{UserID: identity.UserID, TenantID: identity.TenantID}
	filtered := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		allowed := true
		for _, kbID := range listItemKBReferences(response.Request.URL.Path, object) {
			decision, err := g.decisions.Decide(response.Request.Context(), kbID, principal, response.Request.Header)
			if errors.Is(err, authorization.ErrNotFound) || (err == nil && decision.Role == authorization.RoleNone) {
				allowed = false
				break
			}
			if err != nil {
				return &Error{Code: "security.authorization_unavailable", Message: "Authorization service is unavailable", StatusCode: http.StatusServiceUnavailable, Err: err}
			}
		}
		if allowed {
			filtered = append(filtered, item)
		}
	}
	setItems(filtered)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Set("Content-Length", strconv.Itoa(len(encoded)))
	return nil
}

func listItemKBReferences(requestPath string, object map[string]any) []string {
	trimmed := strings.TrimSuffix(requestPath, "/")
	var references []string
	if trimmed == "/api/v1/knowledge-bases" || strings.HasSuffix(trimmed, "/move-targets") {
		references = append(references, stringValues(object["id"])...)
	}
	if trimmed == "/api/v1/user/favorites" {
		resourceType, _ := object["resource_type"].(string)
		if resourceType == "kb" || resourceType == "knowledge_base" {
			references = append(references, stringValues(object["resource_id"])...)
		}
	}
	collectReferences(object, &references)
	return unique(references)
}

func operationFor(request *http.Request) notespolicy.Operation {
	requestPath := request.URL.Path
	if strings.Contains(requestPath, "/shares") || strings.HasPrefix(requestPath, "/api/v1/shared-knowledge-bases") {
		return notespolicy.Share
	}
	if strings.Contains(requestPath, "/publish") || strings.Contains(requestPath, "/catalog") {
		return notespolicy.Publish
	}
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		return notespolicy.Read
	}
	return notespolicy.Write
}

func splitPath(value string) []string {
	trimmed := strings.Trim(value, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func isKnowledgeCollectionPath(segment string) bool {
	switch segment {
	case "batch", "search", "move", "tags", "batch-reparse", "batch-delete", "folder":
		return true
	default:
		return false
	}
}

func isKnowledgeBatchPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/api/v1/knowledge/batch") ||
		strings.HasPrefix(requestPath, "/api/v1/knowledge/tags") ||
		strings.HasPrefix(requestPath, "/api/v1/knowledge/folder") ||
		strings.HasPrefix(requestPath, "/api/v1/knowledge/move")
}

func isSessionBatchPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/api/v1/sessions/batch")
}

func isAgentCollectionPath(segment string) bool {
	switch segment {
	case "placeholders", "type-presets", "mcp-oauth-resolutions", "tool-approvals":
		return true
	default:
		return false
	}
}

func sessionIDsFromPath(requestPath string) []string {
	segments := splitPath(requestPath)
	if len(segments) < 4 || segments[0] != "api" || segments[1] != "v1" {
		return nil
	}
	switch segments[2] {
	case "knowledge-chat", "agent-chat", "messages":
		return one(segments[3])
	case "sessions":
		if segments[3] == "continue-stream" && len(segments) >= 5 {
			return one(segments[4])
		}
		if segments[3] != "batch" {
			return one(segments[3])
		}
	}
	return nil
}

func isFilterableListPath(requestPath string) bool {
	trimmed := strings.TrimSuffix(requestPath, "/")
	return trimmed == "/api/v1/knowledge-bases" || trimmed == "/api/v1/agents" ||
		trimmed == "/api/v1/knowledge/search" || strings.HasSuffix(trimmed, "/move-targets") ||
		trimmed == "/api/v1/shared-knowledge-bases" || trimmed == "/api/v1/shared-agents" ||
		trimmed == "/api/v1/user/favorites" ||
		strings.HasSuffix(trimmed, "/shared-knowledge-bases") || strings.HasSuffix(trimmed, "/shared-agents") ||
		strings.HasSuffix(trimmed, "/shares") || strings.HasSuffix(trimmed, "/agent-shares")
}

func filterableItems(envelope map[string]any) ([]any, func([]any), bool) {
	if items, ok := envelope["data"].([]any); ok {
		return items, func(filtered []any) {
			envelope["data"] = filtered
			if _, hasTotal := envelope["total"]; hasTotal {
				envelope["total"] = len(filtered)
			}
		}, true
	}
	container, ok := envelope["data"].(map[string]any)
	if !ok {
		return nil, nil, false
	}
	items, ok := container["shares"].([]any)
	if !ok {
		return nil, nil, false
	}
	return items, func(filtered []any) {
		container["shares"] = filtered
		container["total"] = len(filtered)
	}, true
}

func isUnscopedDerivedTaskPath(requestPath string) bool {
	segments := splitPath(requestPath)
	if len(segments) < 6 || segments[0] != "api" || segments[1] != "v1" {
		return false
	}
	return (segments[2] == "knowledge" && segments[3] == "move" && segments[4] == "progress") ||
		(segments[2] == "knowledge-bases" && segments[3] == "copy" && segments[4] == "progress") ||
		(segments[2] == "faq" && segments[3] == "import" && segments[4] == "progress")
}

func listItemReferencesForbiddenKB(requestPath string, object map[string]any, forbidden map[string]struct{}) bool {
	if strings.TrimSuffix(requestPath, "/") == "/api/v1/knowledge-bases" {
		id, _ := object["id"].(string)
		_, denied := forbidden[id]
		return denied
	}
	if strings.TrimSuffix(requestPath, "/") == "/api/v1/user/favorites" {
		resourceType, _ := object["resource_type"].(string)
		resourceID, _ := object["resource_id"].(string)
		if resourceType == "kb" || resourceType == "knowledge_base" {
			_, denied := forbidden[resourceID]
			return denied
		}
	}
	var references []string
	collectReferences(object, &references)
	for _, id := range references {
		if _, denied := forbidden[id]; denied {
			return true
		}
	}
	return false
}

func collectReferences(value any, references *[]string) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectReferences(item, references)
		}
	case map[string]any:
		for key, item := range typed {
			switch key {
			case "knowledge_base_id", "kb_id", "knowledge_bases", "knowledge_base_ids":
				*references = append(*references, stringValues(item)...)
			}
			if key == "knowledge_base" {
				if nested, ok := item.(map[string]any); ok {
					*references = append(*references, stringValues(nested["id"])...)
				}
			}
			collectReferences(item, references)
		}
	}
}

func one(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func translateResolverError(err error) error {
	if err == nil {
		return nil
	}
	var upstreamError *weknora.Error
	if errors.As(err, &upstreamError) {
		switch upstreamError.Code {
		case "upstream.not_found", "upstream.forbidden", "upstream.unauthorized":
			return &Error{Code: "resource.not_found", Message: "Resource not found", StatusCode: http.StatusNotFound}
		}
	}
	return &Error{Code: "security.resource_resolution_failed", Message: "Unable to resolve resource policy", StatusCode: http.StatusBadGateway, Err: err}
}
