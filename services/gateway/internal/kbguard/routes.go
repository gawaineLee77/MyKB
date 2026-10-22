// Package kbguard enforces product KB permissions before native content APIs.
package kbguard

import (
	"net/http"
	"strings"
)

type route struct {
	method, pattern, kind string
	actions               []string
	fields                []string
}

func r(method, pattern, kind, actions, fields string) route {
	return route{method, pattern, kind, strings.Fields(actions), strings.Fields(fields)}
}

// Every native content surface is classified explicitly. Unsupported surfaces
// stay closed until their object and side-effect contracts have been verified.
var routes = []route{
	r("GET", "/knowledge-bases", "list", "", ""),
	r("POST", "/knowledge-bases", "create", "", ""),
	r("GET", "/knowledge-bases/:kb", "kb", "", ""),
	r("PUT", "/knowledge-bases/:kb", "kb", "manage", ""),
	r("DELETE", "/knowledge-bases/:kb", "delete", "manage", ""),
	r("GET", "/knowledge-bases/:kb/activity", "kb", "manage", ""),
	r("PUT", "/knowledge-bases/:kb/pin", "kb", "", ""),
	r("GET", "/knowledge-bases/:kb/hybrid-search", "kb", "", ""),
	r("POST", "/knowledge-bases/:kb/hybrid-search", "kb", "", ""),
	r("GET", "/knowledge-bases/:kb/knowledge", "kb", "", ""),
	r("DELETE", "/knowledge-bases/:kb/knowledge", "kb", "manage", ""),
	r("GET", "/knowledge-bases/:kb/knowledge/folders", "kb", "", ""),
	r("PUT", "/knowledge-bases/:kb/knowledge/folders", "kb", "manage", ""),
	r("POST", "/knowledge-bases/:kb/knowledge/file", "upload", "document.create", "file fileName metadata enable_multimodel process_config tag_ids channel folder_path"),
	r("POST", "/knowledge-bases/:kb/knowledge/url", "kb", "document.create", "url file_name file_type enable_multimodel title tag_ids channel process_config"),
	r("POST", "/knowledge-bases/:kb/knowledge/manual", "kb", "document.create", "title content status tag_ids channel process_config"),
	r("GET", "/knowledge-bases/:kb/tags", "kb", "", ""),
	r("POST", "/knowledge-bases/:kb/tags", "kb", "manage", ""),
	r("PUT", "/knowledge-bases/:kb/tags/:tag", "kb", "manage", ""),
	r("DELETE", "/knowledge-bases/:kb/tags/:tag", "kb", "manage", ""),
	r("GET", "/knowledge-bases/:kb/files", "file", "", ""),
	r("GET", "/knowledge-bases/:kb/faq/entries", "kb", "", ""),
	r("GET", "/knowledge-bases/:kb/faq/entries/export", "kb", "chunk.export", ""),
	r("GET", "/knowledge-bases/:kb/faq/entries/:entry", "kb", "", ""),
	r("POST", "/knowledge-bases/:kb/faq/search", "kb", "", ""),
	r("POST", "/knowledge-bases/:kb/faq/entries", "faq", "document.create document.update chunk.update", "entries mode knowledge_id dry_run"),
	r("POST", "/knowledge-bases/:kb/faq/entry", "faq", "document.create", "standard_question similar_questions negative_questions answers answer_strategy tag_id tag_name is_enabled is_recommended"),
	r("PUT", "/knowledge-bases/:kb/faq/entries/fields", "faq", "document.update chunk.update", "by_id by_tag exclude_ids"),
	r("PUT", "/knowledge-bases/:kb/faq/entries/tags", "faq", "document.update chunk.update", "updates"),
	r("PUT", "/knowledge-bases/:kb/faq/entries/:entry", "faq", "document.update chunk.update", "standard_question similar_questions negative_questions answers answer_strategy tag_id tag_name is_enabled is_recommended"),
	r("POST", "/knowledge-bases/:kb/faq/entries/:entry/similar-questions", "faq", "document.update chunk.update", "similar_questions"),
	r("DELETE", "/knowledge-bases/:kb/faq/entries", "faq", "document.delete", "ids"),
	r("PUT", "/knowledge-bases/:kb/faq/import/last-result/display", "kb", "manage", ""),
	r("GET", "/knowledge/batch", "batch-read", "", ""),
	r("GET", "/knowledge/search", "search", "", ""),
	r("GET", "/faq/import/progress/:task", "task", "", ""),
	r("PUT", "/knowledge/tags", "tags", "document.update", "kb_id updates"),
	r("POST", "/knowledge/batch-delete", "batch", "document.delete", "kb_id ids"),
	r("POST", "/knowledge/batch-reparse", "batch", "document.update chunk.update chunk.delete", "kb_id ids process_config"),
	r("POST", "/knowledge/folder", "batch", "document.update", "kb_id knowledge_ids folder_path"),
	r("GET", "/knowledge/:knowledge", "knowledge", "", ""),
	r("DELETE", "/knowledge/:knowledge", "knowledge", "document.delete", ""),
	r("PUT", "/knowledge/:knowledge", "knowledge", "document.update", "title description custom_metadata"),
	r("PUT", "/knowledge/manual/:knowledge", "knowledge", "document.update chunk.update chunk.delete", "title content status tag_ids channel process_config"),
	r("GET", "/knowledge/:knowledge/download", "knowledge", "document.download", ""),
	r("GET", "/knowledge/:knowledge/preview", "knowledge", "document.download", ""),
	r("GET", "/knowledge/:knowledge/stages", "knowledge", "", ""),
	r("GET", "/knowledge/:knowledge/spans", "knowledge", "", ""),
	r("POST", "/knowledge/:knowledge/reparse", "knowledge", "document.update chunk.update chunk.delete", "process_config"),
	r("POST", "/knowledge/:knowledge/cancel-parse", "knowledge", "document.update chunk.update chunk.delete", ""),
	r("POST", "/knowledge/:knowledge/regenerate-summary", "knowledge", "document.update", ""),
	r("PUT", "/knowledge/image/:knowledge/:chunk", "chunk", "chunk.update", "image_info"),
	r("GET", "/chunks/:knowledge", "knowledge", "", ""),
	r("DELETE", "/chunks/:knowledge", "knowledge", "chunk.delete", ""),
	r("GET", "/chunks/by-id/:chunk", "chunk", "", ""),
	r("PUT", "/chunks/:knowledge/:chunk", "chunk", "chunk.update", "content is_enabled expected_revision"),
	r("DELETE", "/chunks/:knowledge/:chunk", "chunk", "chunk.delete", ""),
	r("GET", "/chunks/:knowledge/:chunk/revisions", "chunk", "", ""),
	r("POST", "/chunks/:knowledge/:chunk/revert", "chunk", "chunk.update", "revision expected_revision"),
	r("PUT", "/chunks/by-id/:chunk/questions", "chunk", "chunk.update", "question question_id"),
	r("DELETE", "/chunks/by-id/:chunk/questions", "chunk", "chunk.update", "question_id"),
	r("POST", "/chunks/by-id/:chunk/questions/regenerate", "chunk", "chunk.update", ""),
	r("GET", "/initialization/config/:kb", "kb", "manage", ""),
	r("PUT", "/initialization/config/:kb", "kb", "manage", ""),
	r("POST", "/initialization/initialize/:kb", "kb", "manage", ""),
	r("GET", "/knowledgebase/:kb/wiki/pages", "wiki", "", ""),
	r("GET", "/knowledgebase/:kb/wiki/pages/*slug", "wiki", "", ""),
	r("POST", "/knowledgebase/:kb/wiki/pages", "wiki", "document.create", "slug title page_type status content summary aliases folder_id source_refs chunk_refs"),
	r("PUT", "/knowledgebase/:kb/wiki/pages/*slug", "wiki", "document.update", "title content summary page_type status aliases version"),
	r("DELETE", "/knowledgebase/:kb/wiki/pages/*slug", "wiki", "document.delete", ""),
	r("GET", "/knowledgebase/:kb/wiki/revisions/*slug", "wiki", "", ""),
	r("POST", "/knowledgebase/:kb/wiki/revert", "wiki", "document.update", "slug version"),
	r("PUT", "/knowledgebase/:kb/wiki/move-page", "wiki", "document.update", "slug folder_id"),
	r("GET", "/knowledgebase/:kb/wiki/folders", "wiki", "", ""),
	r("POST", "/knowledgebase/:kb/wiki/folders", "wiki", "manage", ""),
	r("PUT", "/knowledgebase/:kb/wiki/folders/:folder", "wiki", "manage", ""),
	r("DELETE", "/knowledgebase/:kb/wiki/folders/:folder", "wiki", "manage", ""),
	r("GET", "/knowledgebase/:kb/wiki/index", "wiki", "", ""),
	r("GET", "/knowledgebase/:kb/wiki/graph", "wiki", "", ""),
	r("GET", "/knowledgebase/:kb/wiki/stats", "wiki", "", ""),
	r("GET", "/knowledgebase/:kb/wiki/search", "wiki", "", ""),
	r("GET", "/knowledgebase/:kb/wiki/lint", "wiki", "", ""),
	r("GET", "/knowledgebase/:kb/wiki/issues", "wiki", "", ""),
	r("POST", "/knowledge-search", "retrieval", "", ""),
	r("POST", "/knowledge-chat/:session", "retrieval", "", ""),
	r("GET", "/shared-knowledge-bases", "empty", "", ""),
	r("GET", "/organizations/:org/shared-knowledge-bases", "empty", "", ""),
	r("GET", "/organizations/:org/shares", "empty", "", ""),
	r("GET", "/organizations/:org/agent-shares", "empty", "", ""),
	r("GET", "/organizations/:org/shared-agents", "empty", "", ""),
	r("GET", "/shared-agents", "empty", "", ""),
	r("POST", "/mindcreek/kb-permissions/:kb/export-chunks", "export", "chunk.export", "knowledge_id knowledge_ids"),
	r("GET", "/mindcreek/kb-content/:kb/chunks/export", "export", "chunk.export", ""),
}

func match(pattern, path string) (map[string]string, bool) {
	a, b := strings.Split(strings.Trim(pattern, "/"), "/"), strings.Split(strings.Trim(path, "/"), "/")
	wildcard := strings.HasPrefix(a[len(a)-1], "*")
	if (!wildcard && len(a) != len(b)) || (wildcard && len(b) < len(a)) {
		return nil, false
	}
	params := map[string]string{}
	for i, part := range a {
		if strings.HasPrefix(part, "*") {
			for _, piece := range b[i:] {
				if !validID(piece) {
					return nil, false
				}
			}
			params[part[1:]] = strings.Join(b[i:], "/")
			return params, true
		}
		if strings.HasPrefix(part, ":") {
			if !validID(b[i]) {
				return nil, false
			}
			params[part[1:]] = b[i]
		} else if part != b[i] {
			return nil, false
		}
	}
	return params, true
}

func classify(method, path string) (route, map[string]string, bool) {
	path = strings.TrimPrefix(path, "/api/v1")
	if path == "/shared-agents/disabled" {
		return route{kind: "disabled"}, nil, true
	}
	if strings.HasPrefix(path, "/agents/") && strings.Contains(path, "/shares") {
		return route{kind: "disabled"}, nil, true
	}
	// Static names are never interpreted as resource identifiers.
	for _, p := range []string{"/knowledge-bases/copy", "/knowledge-bases/copy/progress/", "/knowledge/move", "/evaluation"} {
		if path == p || strings.HasPrefix(path, strings.TrimSuffix(p, "/")+"/") {
			return route{kind: "disabled"}, nil, true
		}
	}
	for _, rt := range routes {
		if rt.method == method {
			if params, ok := match(rt.pattern, path); ok {
				return rt, params, true
			}
		}
	}
	for _, p := range []string{"/knowledge-bases", "/knowledge/", "/chunks", "/knowledgebase", "/faq/", "/initialization/config", "/initialization/initialize", "/shared-knowledge-bases"} {
		if path == p || strings.HasPrefix(path, strings.TrimSuffix(p, "/")+"/") {
			return route{kind: "disabled"}, nil, true
		}
	}
	return route{}, nil, false
}

// Classified reports whether a route belongs to this mandatory boundary.
func Classified(method, path string) bool { _, _, ok := classify(method, path); return ok }
func writeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}
