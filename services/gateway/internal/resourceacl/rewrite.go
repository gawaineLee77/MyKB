package resourceacl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

// Only locators are replaced. Editable content never becomes ownership
// evidence. The resulting opaque native handle still requires AuthorizeKB or
// AuthorizeMessage on every image read.
var locatorPattern = regexp.MustCompile(`(?:storage://[A-Za-z0-9_-]+/)?(?:local|minio|s3|oss|cos|tos|ks3|obs)://[^\s<>"'\x60)\]},]+`)

func (s *Service) RewriteResources(ctx context.Context, h http.Header, value any) error {
	cache := map[string]string{}
	var tenant uint64
	var err error
	var rewrite func(any) error
	rewriteText := func(text string) (string, error) {
		var lookupErr error
		updated := locatorPattern.ReplaceAllStringFunc(text, func(ref string) string {
			if resolved, ok := cache[ref]; ok {
				return resolved
			}
			if lookupErr != nil {
				return ""
			}
			if tenant == 0 {
				tenant, _, lookupErr = s.Access.ResourceIdentity(ctx, h)
				if lookupErr != nil {
					return ""
				}
			}
			resource, e := s.Catalog.Resolve(ctx, tenant, ref)
			if e != nil {
				if errors.Is(e, ErrNotFound) || errors.Is(e, ErrInvalidReference) {
					// Do not erase the entire paragraph/answer because one old
					// locator lacks a registered resource.
					cache[ref] = "mindcreek-unverified-image"
					return cache[ref]
				}
				lookupErr = fail(503, "resource_verification_unavailable")
				return ""
			}
			if resource.TenantID != tenant || !strings.HasPrefix(resource.Reference, "resource://") {
				lookupErr = fail(503, "resource_verification_unavailable")
				return ""
			}
			cache[ref] = resource.Reference
			return resource.Reference
		})
		return updated, lookupErr
	}
	rewrite = func(v any) error {
		switch object := v.(type) {
		case map[string]any:
			for key, child := range object {
				if text, ok := child.(string); ok {
					// Native image_info is itself serialized JSON. Parse it
					// before rewriting locators so escaping stays valid.
					if key == "image_info" {
						var nested any
						if json.Unmarshal([]byte(text), &nested) == nil {
							if err := rewrite(nested); err != nil {
								return err
							}
							raw, e := json.Marshal(nested)
							if e != nil {
								return e
							}
							object[key] = string(raw)
							continue
						}
					}
					object[key], err = rewriteText(text)
				} else {
					err = rewrite(child)
				}
				if err != nil {
					return err
				}
			}
		case []any:
			for i, child := range object {
				if text, ok := child.(string); ok {
					object[i], err = rewriteText(text)
				} else {
					err = rewrite(child)
				}
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	return rewrite(value)
}
