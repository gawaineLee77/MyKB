package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/managedmodel"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/nativeaccess"
	"github.com/gawaineLee77/MyKB/services/gateway/internal/weknora"
)

type NativeKnowledge interface {
	Knowledge
	AskKnowledgeWithModel(context.Context, string, string, string, []string, string, http.Header) (weknora.AgentAnswer, error)
}

type NativeService struct {
	Scopes    *nativeaccess.ScopeService
	Models    *nativeaccess.ModelPolicy
	Knowledge NativeKnowledge
}

func (s *NativeService) CallNative(ctx context.Context, name string, arguments json.RawMessage, actor nativeaccess.Actor, h http.Header, correlation string) (result any, err error) {
	var ids []string
	defer func() {
		outcome, code := "allowed", ""
		if err != nil {
			outcome = "denied"
			code = "mcp.denied"
		}
		if auditErr := s.Scopes.Store.Record(ctx, actor, "mcp."+name, outcome, code, ids, correlation); auditErr != nil {
			result = nil
			err = ErrUnavailable
		}
		if err != nil {
			var n *nativeaccess.Error
			var w *weknora.Error
			if errors.As(err, &n) {
				switch n.Status {
				case 400, 413, 422:
					err = ErrInvalid
				case 401, 403, 404:
					err = ErrDenied
				default:
					err = ErrUnavailable
				}
			} else if errors.As(err, &w) {
				if w.StatusCode == 401 || w.StatusCode == 403 || w.StatusCode == 404 {
					err = ErrDenied
				} else {
					err = ErrUnavailable
				}
			}
		}
	}()
	if name == "list_knowledge_bases" {
		if err = decodeArguments(arguments, &struct{}{}); err != nil {
			return nil, err
		}
		ids, err = s.Scopes.Defaults(ctx, h)
		if err != nil {
			return nil, err
		}
		items, err := s.Scopes.Describe(ctx, ids, h)
		if err != nil {
			return nil, err
		}
		return map[string]any{"knowledge_base_ids": ids, "items": items}, nil
	}
	if name == "get_source_excerpt" {
		var input struct {
			ChunkID  string `json:"chunk_id"`
			MaxChars int    `json:"max_chars"`
		}
		if err = decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if input.ChunkID == "" || len(input.ChunkID) > 128 || input.MaxChars < 0 || input.MaxChars > 12000 {
			return nil, ErrInvalid
		}
		if input.MaxChars == 0 {
			input.MaxChars = 4000
		}
		kb, err := s.Knowledge.KnowledgeBaseForChunk(ctx, input.ChunkID, h)
		if err != nil {
			return nil, err
		}
		ids = []string{kb}
		if err := s.Scopes.CheckKB(ctx, kb, h); err != nil {
			return nil, err
		}
		excerpt, err := s.Knowledge.GetChunkExcerpt(ctx, input.ChunkID, h)
		if err != nil {
			return nil, err
		}
		if excerpt.KnowledgeBaseID != kb {
			return nil, ErrDenied
		}
		text := []rune(excerpt.Content)
		if len(text) > input.MaxChars {
			excerpt.Content = string(text[:input.MaxChars])
		}
		return excerpt, nil
	}
	var input struct {
		Query        string   `json:"query"`
		KBIDs        []string `json:"knowledge_base_ids"`
		KnowledgeIDs []string `json:"knowledge_ids"`
		AgentID      string   `json:"agent_id"`
		SessionID    string   `json:"session_id"`
		Limit        int      `json:"limit"`
	}
	if err = decodeArguments(arguments, &input); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(arguments, &fields)
	for field := range fields {
		allowed := field == "query" || field == "knowledge_base_ids" || name == "search_knowledge" && field == "knowledge_ids" || name == "ask_knowledge_agent" && (field == "agent_id" || field == "session_id")
		if !allowed {
			return nil, ErrInvalid
		}
	}
	maxQuery := 8000
	if name == "search_knowledge" {
		maxQuery = 4000
	}
	if input.Query == "" || utf8.RuneCountInString(input.Query) > maxQuery || len(input.KnowledgeIDs) > 64 {
		return nil, ErrInvalid
	}
	if name == "ask_knowledge_agent" && input.AgentID != "" {
		config, e := s.Scopes.AgentConfig(ctx, input.AgentID, h)
		if e != nil {
			return nil, e
		}
		if !readOnlyAgent(config) {
			return nil, ErrDenied
		}
	}
	for _, id := range input.KnowledgeIDs {
		kb, e := s.Scopes.Upstream.KnowledgeBaseForKnowledge(ctx, id, h)
		if e != nil {
			return nil, e
		}
		input.KBIDs = append(input.KBIDs, kb)
	}
	ids, err = s.Scopes.Resolve(ctx, input.KBIDs, len(input.KBIDs) > 0, input.AgentID, actor, h)
	if err != nil {
		return nil, err
	}
	if name == "search_knowledge" {
		rows, err := s.Knowledge.SearchKnowledge(ctx, weknora.SearchRequest{Query: input.Query, KnowledgeBaseIDs: ids, KnowledgeIDs: input.KnowledgeIDs}, h)
		if err != nil {
			return nil, err
		}
		if len(rows) > 20 {
			rows = rows[:20]
		}
		for i := range rows {
			rows[i].Content = truncate(rows[i].Content, 4000)
		}
		return map[string]any{"knowledge_base_ids": ids, "effective_scope": ids, "results": rows}, nil
	}
	if name != "ask_knowledge_agent" {
		return nil, ErrNotFound
	}
	if input.SessionID != "" {
		if _, err := s.Scopes.CheckSession(ctx, input.SessionID, actor, h); err != nil {
			return nil, err
		}
	}
	model := ""
	if input.AgentID == "" {
		model = managedmodel.ManagedChatID
		// Native scoped chat keys may use a model inside the chat handler but
		// cannot read model-management metadata. MCP accepts no model override;
		// the fixed server default is resolved by the native chat handler under
		// the original key. Never obtain a broader credential for this lookup.
		if actor.Kind != "api_key" {
			if err := s.Models.ValidateModel(ctx, model, "KnowledgeQA", h); err != nil {
				return nil, err
			}
		}
	}
	if input.SessionID == "" {
		session, err := s.Knowledge.CreateChatSession(ctx, "MCP knowledge question", h)
		if err != nil {
			return nil, err
		}
		input.SessionID = session.ID
		if err := s.Scopes.Store.Bind(ctx, input.SessionID, actor, ids); err != nil {
			return nil, err
		}
	} else if err := s.Scopes.Store.Bind(ctx, input.SessionID, actor, ids); err != nil {
		return nil, err
	}
	return s.Knowledge.AskKnowledgeWithModel(ctx, input.SessionID, input.Query, input.AgentID, ids, model, h)
}

// A read-only MCP tool must not invoke a native Agent that can edit Wiki pages,
// execute skills or call arbitrary external MCP tools on behalf of an Owner.
// Web Agent authoring/execution retains its native rules independently.
func readOnlyAgent(config map[string]any) bool {
	if config["memory_enabled"] == true {
		return false
	}
	if config["mcp_selection_mode"] == "all" || config["skills_selection_mode"] == "all" {
		return false
	}
	for _, field := range []string{"mcp_services", "selected_skills"} {
		if values, ok := config[field].([]any); ok && len(values) > 0 {
			return false
		}
	}
	if enabled, ok := config["web_search_enabled"].(bool); ok && enabled {
		return false
	}
	values, _ := config["allowed_tools"].([]any)
	if len(values) == 0 && config["agent_mode"] != "quick-answer" {
		return false
	}
	for _, value := range values {
		switch value {
		case "thinking", "todo_write", "knowledge_search", "grep_chunks", "list_knowledge_chunks", "query_knowledge_graph", "get_document_info", "wiki_search", "wiki_read_page", "wiki_read_source_doc", "wiki_read_issue":
		default:
			return false
		}
	}
	return true
}
