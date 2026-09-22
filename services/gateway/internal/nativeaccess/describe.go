package nativeaccess

import (
	"context"
	"net/http"
	"net/url"
)

func (s *ScopeService) Describe(ctx context.Context, ids []string, h http.Header) ([]map[string]any, error) {
	items := []map[string]any{}
	for _, id := range ids {
		value, err := s.data(ctx, "/api/v1/knowledge-bases/"+url.PathEscape(id), nil, h)
		if err != nil {
			return nil, err
		}
		kb, ok := value.(map[string]any)
		if !ok {
			return nil, &Error{502, "upstream.invalid_response"}
		}
		item := map[string]any{"id": id}
		for _, key := range []string{"name", "description", "type", "capabilities"} {
			if value, ok := kb[key]; ok {
				item[key] = value
			}
		}
		items = append(items, item)
	}
	return items, nil
}
