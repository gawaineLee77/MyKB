package nativeaccess

import (
	"context"
	"net/http"
	"net/url"
)

// These pinned tools receive server-owned SearchTargets. Unclassified tools
// cannot run under the product ACL, even if an old Agent configuration enables
// them. Write tools additionally require KB management on the entire turn scope.
var scopedReadTools = map[string]bool{
	"thinking": true, "todo_write": true, "knowledge_search": true,
	"grep_chunks": true, "list_knowledge_chunks": true, "get_document_info": true,
	"query_knowledge_graph": true,
	"data_schema":           true, "wiki_search": true, "wiki_read_page": true,
	"wiki_read_source_doc": true, "wiki_read_issue": true,
}
var scopedWriteTools = map[string]bool{
	"wiki_flag_issue": true, "wiki_write_page": true, "wiki_replace_text": true,
	"wiki_rename_page": true, "wiki_delete_page": true, "wiki_update_issue": true,
}

// v0.8.0's agent all-mode applies this union of tool requirements before
// choosing KBs (internal/agent/tools/capabilities.go). Scope expansion in the
// product must preserve that selection when materializing an explicit list.
// Feature flags come from the native KB response's computed capabilities.
func (s *ScopeService) compatible(ctx context.Context, ids []string, config map[string]any, h http.Header) ([]string, error) {
	rag, wiki := config["agent_mode"] == "quick-answer", false
	for _, tool := range stringsValue(config["allowed_tools"]) {
		switch tool {
		case "knowledge_search", "grep_chunks", "list_knowledge_chunks", "query_knowledge_graph", "get_document_info", "database_query", "data_analysis", "data_schema":
			rag = true
		case "wiki_search", "wiki_read_page", "wiki_read_source_doc", "wiki_flag_issue", "wiki_write_page", "wiki_replace_text", "wiki_rename_page", "wiki_delete_page", "wiki_read_issue", "wiki_update_issue":
			wiki = true
		}
	}
	if !rag && !wiki {
		return ids, nil
	}
	result := []string{}
	for _, id := range ids {
		value, err := s.data(ctx, "/api/v1/knowledge-bases/"+url.PathEscape(id), nil, h)
		if err != nil {
			return nil, err
		}
		kb, ok := value.(map[string]any)
		if !ok {
			return nil, &Error{502, "upstream.invalid_response"}
		}
		caps, ok := kb["capabilities"].(map[string]any)
		if !ok {
			return nil, &Error{502, "upstream.capabilities_missing"}
		}
		if rag && (caps["vector"] == true || caps["keyword"] == true) || wiki && caps["wiki"] == true {
			result = append(result, id)
		}
	}
	return result, nil
}
