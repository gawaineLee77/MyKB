// Package weknora implements MindCreek's narrow, versioned WeKnora boundary.
package weknora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// SupportedVersion is the only upstream contract verified for this release.
	SupportedVersion = "v0.7.2"
	maxResponseBytes = 1 << 20
)

// Error is a stable translation of a transport or upstream HTTP failure.
type Error struct {
	Code           string
	StatusCode     int
	UpstreamStatus int
	Err            error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

// Client calls only the upstream endpoints approved for the v0.7.2 adapter.
type Client struct {
	baseURL         *url.URL
	httpClient      *http.Client
	expectedVersion string
}

// New constructs an adapter and fails closed for an unverified upstream version.
func New(baseURL *url.URL, expectedVersion string, timeout time.Duration) (*Client, error) {
	if expectedVersion != SupportedVersion {
		return nil, &Error{
			Code:       "upstream.version_unsupported",
			StatusCode: http.StatusServiceUnavailable,
			Err:        fmt.Errorf("expected %q; adapter supports %q", expectedVersion, SupportedVersion),
		}
	}
	if baseURL == nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid WeKnora base URL")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("WeKnora timeout must be positive")
	}
	copyURL := *baseURL
	return &Client{
		baseURL:         &copyURL,
		httpClient:      &http.Client{Timeout: timeout},
		expectedVersion: expectedVersion,
	}, nil
}

// Health checks the unauthenticated upstream liveness contract.
func (c *Client) Health(ctx context.Context) error {
	var response struct {
		Status string `json:"status"`
	}
	if err := c.getJSON(ctx, "/health", nil, &response); err != nil {
		return err
	}
	if response.Status != "ok" {
		return &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return nil
}

// Version returns system information and verifies the live upstream version.
func (c *Client) Version(ctx context.Context, inbound http.Header) (SystemInfo, error) {
	var response struct {
		Code int        `json:"code"`
		Data SystemInfo `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/system/info", inbound, &response); err != nil {
		return SystemInfo{}, err
	}
	if response.Code != 0 || response.Data.Version == "" {
		return SystemInfo{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	if response.Data.Version != c.expectedVersion {
		return SystemInfo{}, &Error{
			Code:       "upstream.version_unsupported",
			StatusCode: http.StatusServiceUnavailable,
			Err:        fmt.Errorf("live version %q does not match %q", response.Data.Version, c.expectedVersion),
		}
	}
	return response.Data, nil
}

// CurrentPrincipal resolves the authenticated user and active tenant upstream.
func (c *Client) CurrentPrincipal(ctx context.Context, inbound http.Header) (Principal, error) {
	var response struct {
		Success bool      `json:"success"`
		Data    Principal `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/auth/me", inbound, &response); err != nil {
		return Principal{}, err
	}
	if !response.Success || response.Data.User == nil || response.Data.User.ID == "" {
		return Principal{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

// KnowledgeBaseForKnowledge resolves a knowledge/source ID to its parent KB.
func (c *Client) KnowledgeBaseForKnowledge(ctx context.Context, knowledgeID string, inbound http.Header) (string, error) {
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			KnowledgeBaseID string `json:"knowledge_base_id"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/knowledge/"+url.PathEscape(knowledgeID), inbound, &response); err != nil {
		return "", err
	}
	if !response.Success || response.Data.KnowledgeBaseID == "" {
		return "", &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data.KnowledgeBaseID, nil
}

// KnowledgeBaseForChunk resolves a chunk ID directly to its parent KB.
func (c *Client) KnowledgeBaseForChunk(ctx context.Context, chunkID string, inbound http.Header) (string, error) {
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			KnowledgeBaseID string `json:"knowledge_base_id"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/chunks/by-id/"+url.PathEscape(chunkID), inbound, &response); err != nil {
		return "", err
	}
	if !response.Success || response.Data.KnowledgeBaseID == "" {
		return "", &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data.KnowledgeBaseID, nil
}

// ValidateSession confirms that the current credential owns or may access a session.
func (c *Client) ValidateSession(ctx context.Context, sessionID string, inbound http.Header) error {
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/sessions/"+url.PathEscape(sessionID), inbound, &response); err != nil {
		return err
	}
	if !response.Success || response.Data.ID != sessionID {
		return &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return nil
}

type AgentScope struct {
	SelectionMode    string
	KnowledgeBaseIDs []string
}

type KnowledgeBase struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Description      string `json:"description"`
	TenantID         uint64 `json:"tenant_id"`
	CreatorID        string `json:"creator_id"`
	EmbeddingModelID string `json:"embedding_model_id"`
}

type TenantMember struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type TenantMemberPage struct {
	Items    []TenantMember `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type CreateKnowledgeBaseRequest struct {
	ID                       string                   `json:"id"`
	Name                     string                   `json:"name"`
	Description              string                   `json:"description"`
	Type                     string                   `json:"type"`
	EmbeddingModelID         string                   `json:"embedding_model_id"`
	SummaryModelID           string                   `json:"summary_model_id,omitempty"`
	StorageProviderConfig    StorageProviderConfig    `json:"storage_provider_config"`
	ChunkingConfig           ChunkingConfig           `json:"chunking_config"`
	IndexingStrategy         IndexingStrategy         `json:"indexing_strategy"`
	QuestionGenerationConfig QuestionGenerationConfig `json:"question_generation_config"`
}

type StorageProviderConfig struct {
	Provider string `json:"provider"`
}

type ChunkingConfig struct {
	ChunkSize    int      `json:"chunk_size"`
	ChunkOverlap int      `json:"chunk_overlap"`
	Separators   []string `json:"separators"`
	Strategy     string   `json:"strategy"`
}

type IndexingStrategy struct {
	VectorEnabled  bool `json:"vector_enabled"`
	KeywordEnabled bool `json:"keyword_enabled"`
	WikiEnabled    bool `json:"wiki_enabled"`
	GraphEnabled   bool `json:"graph_enabled"`
}

type QuestionGenerationConfig struct {
	Enabled       bool `json:"enabled"`
	QuestionCount int  `json:"question_count"`
}

type ManualKnowledgeMetadata struct {
	Content   string `json:"content"`
	Format    string `json:"format"`
	Status    string `json:"status"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

type ManualKnowledge struct {
	ID              string                  `json:"id"`
	KnowledgeBaseID string                  `json:"knowledge_base_id"`
	Type            string                  `json:"type"`
	Title           string                  `json:"title"`
	ParseStatus     string                  `json:"parse_status"`
	ErrorMessage    string                  `json:"error_message"`
	Metadata        ManualKnowledgeMetadata `json:"metadata"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

type Knowledge struct {
	ID              string    `json:"id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	FileName        string    `json:"file_name"`
	FileType        string    `json:"file_type"`
	FileSize        int64     `json:"file_size"`
	ParseStatus     string    `json:"parse_status"`
	ErrorMessage    string    `json:"error_message"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type KnowledgePage struct {
	Items    []Knowledge `json:"items"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

type ManualKnowledgeInput struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Status  string `json:"status"`
	Channel string `json:"channel,omitempty"`
}

type ManualKnowledgePage struct {
	Items    []ManualKnowledge
	Total    int
	Page     int
	PageSize int
}

func (c *Client) GetKnowledgeBase(ctx context.Context, kbID string, inbound http.Header) (KnowledgeBase, error) {
	var response struct {
		Success bool          `json:"success"`
		Data    KnowledgeBase `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/knowledge-bases/"+url.PathEscape(kbID), inbound, &response); err != nil {
		return KnowledgeBase{}, err
	}
	if !response.Success || response.Data.ID != kbID || response.Data.TenantID == 0 {
		return KnowledgeBase{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) ListKnowledgeBases(ctx context.Context, inbound http.Header) ([]KnowledgeBase, error) {
	var response struct {
		Success bool            `json:"success"`
		Data    []KnowledgeBase `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/knowledge-bases", inbound, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	for _, item := range response.Data {
		if item.ID == "" || item.TenantID == 0 {
			return nil, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
		}
	}
	return response.Data, nil
}

func (c *Client) ListTenantMembers(ctx context.Context, tenantID uint64, query string, page, pageSize int, inbound http.Header) (TenantMemberPage, error) {
	if tenantID == 0 || page < 1 || pageSize < 1 || pageSize > 100 {
		return TenantMemberPage{}, &Error{Code: "upstream.request_invalid", StatusCode: http.StatusBadRequest}
	}
	values := url.Values{
		"page":      {fmt.Sprintf("%d", page)},
		"page_size": {fmt.Sprintf("%d", pageSize)},
	}
	if query = strings.TrimSpace(query); query != "" {
		values.Set("q", query)
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Members  []TenantMember `json:"members"`
			Total    int            `json:"total"`
			Page     int            `json:"page"`
			PageSize int            `json:"page_size"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/api/v1/tenants/%d/members", tenantID)
	if err := c.getJSONQuery(ctx, path, values, inbound, &response); err != nil {
		return TenantMemberPage{}, err
	}
	if !response.Success || response.Data.Total < 0 || response.Data.Page != page || response.Data.PageSize != pageSize {
		return TenantMemberPage{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	for _, item := range response.Data.Members {
		if item.UserID == "" || item.Status != "active" {
			return TenantMemberPage{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
		}
	}
	return TenantMemberPage{Items: response.Data.Members, Total: response.Data.Total, Page: page, PageSize: pageSize}, nil
}

func (c *Client) CreateKnowledgeBase(ctx context.Context, input CreateKnowledgeBaseRequest, inbound http.Header) (KnowledgeBase, error) {
	var response struct {
		Success bool          `json:"success"`
		Data    KnowledgeBase `json:"data"`
	}
	if err := c.sendJSON(ctx, http.MethodPost, "/api/v1/knowledge-bases", nil, inbound, input, &response); err != nil {
		return KnowledgeBase{}, err
	}
	if !response.Success || response.Data.ID != input.ID || response.Data.TenantID == 0 {
		return KnowledgeBase{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) ListManualKnowledge(ctx context.Context, kbID string, page, pageSize int, inbound http.Header) (ManualKnowledgePage, error) {
	query := url.Values{
		"page":      {fmt.Sprintf("%d", page)},
		"page_size": {fmt.Sprintf("%d", pageSize)},
		"source":    {"manual"},
	}
	var response struct {
		Success  bool              `json:"success"`
		Data     []ManualKnowledge `json:"data"`
		Total    int               `json:"total"`
		Page     int               `json:"page"`
		PageSize int               `json:"page_size"`
	}
	path := "/api/v1/knowledge-bases/" + url.PathEscape(kbID) + "/knowledge"
	if err := c.getJSONQuery(ctx, path, query, inbound, &response); err != nil {
		return ManualKnowledgePage{}, err
	}
	if !response.Success || response.Page < 1 || response.PageSize < 1 || response.Total < 0 {
		return ManualKnowledgePage{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	for _, item := range response.Data {
		if !validManualKnowledge(item, kbID) {
			return ManualKnowledgePage{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
		}
	}
	return ManualKnowledgePage{Items: response.Data, Total: response.Total, Page: response.Page, PageSize: response.PageSize}, nil
}

func (c *Client) GetManualKnowledge(ctx context.Context, kbID, noteID string, inbound http.Header) (ManualKnowledge, error) {
	var response struct {
		Success bool            `json:"success"`
		Data    ManualKnowledge `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/knowledge/"+url.PathEscape(noteID), inbound, &response); err != nil {
		return ManualKnowledge{}, err
	}
	if !response.Success || !validManualKnowledge(response.Data, kbID) {
		return ManualKnowledge{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) CreateManualKnowledge(ctx context.Context, kbID string, input ManualKnowledgeInput, inbound http.Header) (ManualKnowledge, error) {
	var response struct {
		Success bool            `json:"success"`
		Data    ManualKnowledge `json:"data"`
	}
	path := "/api/v1/knowledge-bases/" + url.PathEscape(kbID) + "/knowledge/manual"
	if err := c.sendJSON(ctx, http.MethodPost, path, nil, inbound, input, &response); err != nil {
		return ManualKnowledge{}, err
	}
	if !response.Success || !validManualKnowledge(response.Data, kbID) {
		return ManualKnowledge{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) UpdateManualKnowledge(ctx context.Context, kbID, noteID string, input ManualKnowledgeInput, inbound http.Header) (ManualKnowledge, error) {
	var response struct {
		Success bool            `json:"success"`
		Data    ManualKnowledge `json:"data"`
	}
	path := "/api/v1/knowledge/manual/" + url.PathEscape(noteID)
	if err := c.sendJSON(ctx, http.MethodPut, path, nil, inbound, input, &response); err != nil {
		return ManualKnowledge{}, err
	}
	if !response.Success || !validManualKnowledge(response.Data, kbID) {
		return ManualKnowledge{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) DeleteManualKnowledge(ctx context.Context, noteID string, inbound http.Header) error {
	var response struct {
		Success bool `json:"success"`
	}
	if err := c.sendJSON(ctx, http.MethodDelete, "/api/v1/knowledge/"+url.PathEscape(noteID), nil, inbound, nil, &response); err != nil {
		return err
	}
	if !response.Success {
		return &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return nil
}

func validManualKnowledge(item ManualKnowledge, kbID string) bool {
	return item.ID != "" && item.KnowledgeBaseID == kbID && item.Type == "manual" && item.Metadata.Format == "markdown"
}

func (c *Client) UploadKnowledge(ctx context.Context, kbID, filename string, source io.Reader, inbound http.Header) (Knowledge, error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		part, err := multipartWriter.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, source)
		}
		if err == nil {
			err = multipartWriter.WriteField("channel", "mindcreek")
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	path := "/api/v1/knowledge-bases/" + url.PathEscape(kbID) + "/knowledge/file"
	var response struct {
		Success bool      `json:"success"`
		Data    Knowledge `json:"data"`
	}
	if err := c.sendStream(ctx, http.MethodPost, path, multipartWriter.FormDataContentType(), reader, inbound, &response); err != nil {
		return Knowledge{}, err
	}
	if !response.Success || !validKnowledge(response.Data, kbID) {
		return Knowledge{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) ListKnowledge(ctx context.Context, kbID string, page, pageSize int, inbound http.Header) (KnowledgePage, error) {
	query := url.Values{"page": {fmt.Sprintf("%d", page)}, "page_size": {fmt.Sprintf("%d", pageSize)}}
	var response struct {
		Success  bool        `json:"success"`
		Data     []Knowledge `json:"data"`
		Total    int         `json:"total"`
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
	}
	path := "/api/v1/knowledge-bases/" + url.PathEscape(kbID) + "/knowledge"
	if err := c.getJSONQuery(ctx, path, query, inbound, &response); err != nil {
		return KnowledgePage{}, err
	}
	if !response.Success || response.Page < 1 || response.PageSize < 1 || response.Total < 0 {
		return KnowledgePage{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	for _, item := range response.Data {
		if !validKnowledge(item, kbID) {
			return KnowledgePage{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
		}
	}
	return KnowledgePage{Items: response.Data, Total: response.Total, Page: response.Page, PageSize: response.PageSize}, nil
}

func (c *Client) GetKnowledge(ctx context.Context, kbID, knowledgeID string, inbound http.Header) (Knowledge, error) {
	var response struct {
		Success bool      `json:"success"`
		Data    Knowledge `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/knowledge/"+url.PathEscape(knowledgeID), inbound, &response); err != nil {
		return Knowledge{}, err
	}
	if !response.Success || !validKnowledge(response.Data, kbID) {
		return Knowledge{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func (c *Client) ReparseKnowledge(ctx context.Context, kbID, knowledgeID string, inbound http.Header) (Knowledge, error) {
	return c.mutateKnowledge(ctx, kbID, knowledgeID, "reparse", inbound)
}

func (c *Client) CancelKnowledge(ctx context.Context, kbID, knowledgeID string, inbound http.Header) (Knowledge, error) {
	return c.mutateKnowledge(ctx, kbID, knowledgeID, "cancel-parse", inbound)
}

func (c *Client) mutateKnowledge(ctx context.Context, kbID, knowledgeID, action string, inbound http.Header) (Knowledge, error) {
	var response struct {
		Success bool      `json:"success"`
		Data    Knowledge `json:"data"`
	}
	path := "/api/v1/knowledge/" + url.PathEscape(knowledgeID) + "/" + action
	if err := c.sendJSON(ctx, http.MethodPost, path, nil, inbound, nil, &response); err != nil {
		return Knowledge{}, err
	}
	if !response.Success || !validKnowledge(response.Data, kbID) {
		return Knowledge{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	return response.Data, nil
}

func validKnowledge(item Knowledge, kbID string) bool {
	return item.ID != "" && item.KnowledgeBaseID == kbID && item.Type != ""
}

// AgentKnowledgeBases resolves the effective KB scope of an owned or shared agent.
func (c *Client) AgentKnowledgeBases(ctx context.Context, agentID string, inbound http.Header) (AgentScope, error) {
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			TenantID uint64 `json:"tenant_id"`
			Config   *struct {
				KBSelectionMode string   `json:"kb_selection_mode"`
				KnowledgeBases  []string `json:"knowledge_bases"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/v1/agents/"+url.PathEscape(agentID), inbound, &response); err != nil {
		return AgentScope{}, err
	}
	if !response.Success || response.Data.Config == nil {
		return AgentScope{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	scope := AgentScope{
		SelectionMode:    response.Data.Config.KBSelectionMode,
		KnowledgeBaseIDs: response.Data.Config.KnowledgeBases,
	}
	if scope.SelectionMode != "all" {
		return scope, nil
	}

	query := url.Values{"agent_id": {agentID}}
	if response.Data.TenantID > 0 {
		query.Set("agent_source_tenant_id", fmt.Sprintf("%d", response.Data.TenantID))
	}
	var listResponse struct {
		Success bool `json:"success"`
		Data    []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.getJSONQuery(ctx, "/api/v1/knowledge-bases", query, inbound, &listResponse); err != nil {
		return AgentScope{}, err
	}
	if !listResponse.Success {
		return AgentScope{}, &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway}
	}
	for _, kb := range listResponse.Data {
		if kb.ID != "" {
			scope.KnowledgeBaseIDs = append(scope.KnowledgeBaseIDs, kb.ID)
		}
	}
	return scope, nil
}

func (c *Client) getJSON(ctx context.Context, path string, inbound http.Header, destination any) error {
	return c.getJSONQuery(ctx, path, nil, inbound, destination)
}

func (c *Client) getJSONQuery(ctx context.Context, path string, query url.Values, inbound http.Header, destination any) error {
	return c.sendJSON(ctx, http.MethodGet, path, query, inbound, nil, destination)
}

func (c *Client) sendJSON(ctx context.Context, method, path string, query url.Values, inbound http.Header, payload, destination any) error {
	target := c.baseURL.ResolveReference(&url.URL{Path: path, RawQuery: query.Encode()})
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &Error{Code: "upstream.request_invalid", StatusCode: http.StatusInternalServerError, Err: err}
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return &Error{Code: "upstream.request_invalid", StatusCode: http.StatusInternalServerError, Err: err}
	}
	copyIdentityHeaders(request.Header, inbound)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		code := "upstream.unavailable"
		var netError interface{ Timeout() bool }
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netError) && netError.Timeout()) {
			code = "upstream.timeout"
		}
		return &Error{Code: code, StatusCode: http.StatusBadGateway, Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return translateStatus(response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(destination); err != nil {
		return &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway, Err: err}
	}
	return nil
}

func (c *Client) sendStream(ctx context.Context, method, path, contentType string, body io.Reader, inbound http.Header, destination any) error {
	target := c.baseURL.ResolveReference(&url.URL{Path: path})
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return &Error{Code: "upstream.request_invalid", StatusCode: http.StatusInternalServerError, Err: err}
	}
	copyIdentityHeaders(request.Header, inbound)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", contentType)
	response, err := c.httpClient.Do(request)
	if err != nil {
		code := "upstream.unavailable"
		var netError interface{ Timeout() bool }
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netError) && netError.Timeout()) {
			code = "upstream.timeout"
		}
		return &Error{Code: code, StatusCode: http.StatusBadGateway, Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return translateStatus(response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(destination); err != nil {
		return &Error{Code: "upstream.invalid_response", StatusCode: http.StatusBadGateway, Err: err}
	}
	return nil
}

func copyIdentityHeaders(destination, source http.Header) {
	for _, name := range []string{"Authorization", "X-API-Key", "X-Tenant-ID", "X-Request-ID", "Accept-Language"} {
		if value := strings.TrimSpace(source.Get(name)); value != "" {
			destination.Set(name, value)
		}
	}
}

func translateStatus(status int) error {
	translated := &Error{UpstreamStatus: status, StatusCode: status}
	switch status {
	case http.StatusUnauthorized:
		translated.Code = "upstream.unauthorized"
	case http.StatusForbidden:
		translated.Code = "upstream.forbidden"
	case http.StatusNotFound:
		translated.Code = "upstream.not_found"
	case http.StatusTooManyRequests:
		translated.Code = "upstream.rate_limited"
	default:
		if status >= 500 {
			translated.Code = "upstream.unavailable"
			translated.StatusCode = http.StatusBadGateway
		} else {
			translated.Code = "upstream.rejected"
		}
	}
	return translated
}

// SystemInfo is the subset of GET /api/v1/system/info used at the boundary.
type SystemInfo struct {
	Version  string `json:"version"`
	Edition  string `json:"edition"`
	CommitID string `json:"commit_id,omitempty"`
}

// Principal is the identity returned by GET /api/v1/auth/me.
type Principal struct {
	User        *User        `json:"user"`
	Tenant      *Tenant      `json:"tenant"`
	Memberships []Membership `json:"memberships,omitempty"`
}

type User struct {
	ID                  string `json:"id"`
	Username            string `json:"username"`
	Email               string `json:"email"`
	TenantID            uint64 `json:"tenant_id"`
	CanAccessAllTenants bool   `json:"can_access_all_tenants,omitempty"`
}

type Tenant struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type Membership struct {
	TenantID   uint64 `json:"tenant_id"`
	TenantName string `json:"tenant_name,omitempty"`
	Role       string `json:"role"`
}
