package nativeaccess

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
)

type ModelPolicy struct {
	Upstream     NativeReader
	GraphEnabled bool
	MaxFileBytes int64
}

func (p *ModelPolicy) ValidateModel(ctx context.Context, id, kind string, h http.Header) error {
	return p.validate(ctx, id, kind, h)
}

func (p *ModelPolicy) validate(ctx context.Context, id, kind string, h http.Header) error {
	if !validID(id) {
		return &Error{400, "models.invalid_reference"}
	}
	raw, err := p.Upstream.NativeRequest(ctx, "GET", "/api/v1/models/"+url.PathEscape(id), nil, h, nil)
	if err != nil {
		return err
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.ID != id || response.Data.Type != kind || response.Data.Status != "active" {
		return &Error{400, "models.invalid_reference"}
	}
	return nil
}

func (p *ModelPolicy) Check(ctx context.Context, r *http.Request, actor Actor) error {
	if r.Method == "GET" || r.Method == "DELETE" || r.Method == "HEAD" {
		return nil
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return p.upload(r)
	}
	body, err := readObject(r)
	if err != nil {
		return err
	}
	if memory, ok := body["memory_config"].(map[string]any); ok && memory["enabled"] == true {
		return &Error{403, "feature.disabled"}
	}
	if body["memory_enabled"] == true {
		return &Error{403, "feature.disabled"}
	}
	if config, ok := body["config"].(map[string]any); ok && config["memory_enabled"] == true {
		return &Error{403, "feature.disabled"}
	}
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/v1/initialization/initialize/") {
		return &Error{403, "models.managed_policy"}
	}
	kb := strings.HasPrefix(path, "/api/v1/knowledge-bases")
	initialization := strings.HasPrefix(path, "/api/v1/initialization/config/")
	graphPreview := strings.HasPrefix(path, "/api/v1/initialization/extract/")
	agent := strings.HasPrefix(path, "/api/v1/agents")
	chat := strings.HasPrefix(path, "/api/v1/knowledge-chat/") || strings.HasPrefix(path, "/api/v1/agent-chat/")
	if !kb && !agent && !chat && !initialization && !graphPreview {
		return nil
	}
	if graphPreview || graphRequested(body) {
		if !p.GraphEnabled {
			return &Error{409, "graph.dependencies_unavailable"}
		}
		raw, err := p.Upstream.NativeRequest(ctx, "GET", "/api/v1/system/info", nil, r.Header, nil)
		if err != nil {
			return err
		}
		var response struct {
			Data struct {
				Engine string `json:"graph_database_engine"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &response) != nil || response.Data.Engine != "Neo4j" {
			return &Error{409, "graph.dependencies_unavailable"}
		}
	}
	if kb && r.Method == "POST" && path == "/api/v1/knowledge-bases" {
		fill(body, "embedding_model_id", managedmodel.ManagedEmbeddingID)
		fill(body, "summary_model_id", managedmodel.ManagedChatID)
	}
	if agent && (r.Method == "POST" && path == "/api/v1/agents" || r.Method == "PUT") {
		config, ok := body["config"].(map[string]any)
		if !ok && r.Method == "POST" {
			config = map[string]any{}
			body["config"] = config
		}
		if config != nil {
			// PUT may omit the config entirely. Existing native configuration is
			// preserved; a submitted config gets defaults only for absent models.
			fill(config, "model_id", managedmodel.ManagedChatID)
			fill(config, "rerank_model_id", managedmodel.ManagedRerankID)
		}
	}
	if chat {
		// A server-configured Agent owns its model selection. Do not override it
		// with the platform default from a missing request-level override.
		if body["agent_id"] == nil || body["agent_id"] == "" {
			fill(body, "summary_model_id", managedmodel.ManagedChatID)
		}
	}
	if initialization {
		fill(body, "llmModelId", managedmodel.ManagedChatID)
	}
	if graphPreview {
		fill(body, "model_id", managedmodel.ManagedChatID)
		id, ok := body["model_id"].(string)
		if !ok {
			return &Error{400, "models.invalid_reference"}
		}
		if err := p.validate(ctx, id, "KnowledgeQA", r.Header); err != nil {
			return err
		}
	}
	if config, ok := body["vlm_config"].(map[string]any); ok {
		for _, field := range []string{"api_key", "base_url", "model_name"} {
			if value := config[field]; value != nil && value != "" {
				return &Error{403, "models.managed_policy"}
			}
		}
		if config["enabled"] == true {
			fill(config, "model_id", managedmodel.ManagedVLMID)
		}
	}
	if err := p.modelFields(ctx, body, "", r.Header); err != nil {
		return err
	}
	return replaceObject(r, body)
}

func fill(body map[string]any, key, value string) {
	if current, ok := body[key]; !ok || current == "" || current == nil {
		body[key] = value
	}
}

func (p *ModelPolicy) modelFields(ctx context.Context, body map[string]any, parent string, h http.Header) error {
	for key, value := range body {
		kind := ""
		switch key {
		case "embedding_model_id", "embeddingModelId":
			kind = "Embedding"
		case "summary_model_id", "llmModelId":
			kind = "KnowledgeQA"
		case "rerank_model_id":
			kind = "Rerank"
		case "model_id":
			switch parent {
			case "config", "auto_tag_config", "extract_config":
				kind = "KnowledgeQA"
			case "vlm_config", "image_processing_config":
				kind = "VLLM"
			case "asr_config":
				kind = "ASR"
			}
		}
		if kind != "" && value != nil && value != "" {
			id, ok := value.(string)
			if !ok {
				return &Error{400, "models.invalid_reference"}
			}
			if err := p.validate(ctx, id, kind, h); err != nil {
				return err
			}
		}
		// Limit traversal to native configuration objects. Document metadata is
		// content, not a model reference, and must remain unchanged.
		switch key {
		case "config", "auto_tag_config", "extract_config", "vlm_config", "image_processing_config", "asr_config":
			if nested, ok := value.(map[string]any); ok {
				if err := p.modelFields(ctx, nested, key, h); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func graphRequested(body map[string]any) bool {
	if strategy, ok := body["indexing_strategy"].(map[string]any); ok && strategy["graph_enabled"] == true {
		return true
	}
	if extract, ok := body["extract_config"].(map[string]any); ok && extract["enabled"] == true {
		return true
	}
	if extract, ok := body["nodeExtract"].(map[string]any); ok && extract["enabled"] == true {
		return true
	}
	if config, ok := body["config"].(map[string]any); ok {
		return graphRequested(config)
	}
	return false
}

type uploadBody struct{ *os.File }

func (b *uploadBody) Close() error { err := b.File.Close(); _ = os.Remove(b.File.Name()); return err }

func (p *ModelPolicy) upload(r *http.Request) (err error) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return &Error{400, "upload.invalid_multipart"}
	}
	file, err := os.CreateTemp("", "mindcreek-r3-upload-*")
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	limit := p.MaxFileBytes
	if limit <= 0 {
		limit = 500 << 20
	}
	n, err := io.Copy(file, io.LimitReader(r.Body, limit+(8<<20)+1))
	r.Body.Close()
	if err != nil {
		return err
	}
	if n > limit+(8<<20) {
		return &Error{413, "upload.file_too_large"}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader := multipart.NewReader(file, params["boundary"])
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			return &Error{400, "upload.invalid_multipart"}
		}
		if part.FileName() != "" {
			ext := strings.ToLower(filepath.Ext(part.FileName()))
			if !strings.Contains("|.md|.txt|.pdf|.doc|.docx|.xls|.xlsx|.ppt|.pptx|.csv|.html|.htm|.json|.xml|", "|"+ext+"|") {
				return &Error{400, "upload.unsupported_type"}
			}
			count, e := io.Copy(io.Discard, io.LimitReader(part, limit+1))
			if e != nil {
				return e
			}
			if count > limit {
				return &Error{413, "upload.file_too_large"}
			}
		}
		part.Close()
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r.Body = &uploadBody{file}
	r.ContentLength = n
	ok = true
	return nil
}
