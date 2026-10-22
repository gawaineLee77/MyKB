package nativeaccess

import (
	"net/http"
	"strings"
)

// Never let a deployment default turn authenticated resource handles into
// reusable public storage URLs. All resource reads remain behind the gateway.
func enforcePrivateResources(r *http.Request) error {
	q := r.URL.Query()
	q.Set("resource_urls", "handle")
	r.URL.RawQuery = q.Encode()
	r.RequestURI = r.URL.RequestURI()
	return nil
}

func checkResourceInputs(body map[string]any) error {
	if value, present := body["images"]; present && value != nil {
		images, ok := value.([]any)
		if !ok {
			return &Error{400, "request.invalid_images"}
		}
		for _, item := range images {
			image, ok := item.(map[string]any)
			if !ok {
				return &Error{400, "request.invalid_images"}
			}
			data, _ := image["data"].(string)
			// URL-only images can cause the VLM to read another KB's resource.
			// Accept fresh inline bytes; native code validates/decodes the image.
			if !strings.HasPrefix(data, "data:image/") || !strings.Contains(data, ";base64,") {
				return &Error{403, "kb_acl.resource_binding_required"}
			}
			for key, value := range image {
				if key != "data" && value != "" && value != nil {
					return &Error{403, "kb_acl.resource_binding_required"}
				}
			}
		}
	}
	// attachment_uploads contains fresh base64 bytes. attachment_ids are read
	// by the pinned upstream's temporaryDocuments.Get(tenant, session, id),
	// after CheckSession has established ownership of that session.
	return nil
}

func privateResourceField(key string) bool {
	switch strings.ToLower(key) {
	case "file_path", "file_url", "download_url", "preview_url", "presigned_url", "signed_url", "storage_url", "original_url", "physical_path", "failed_entries_url":
		return true
	}
	return false
}

func containsStorageLocator(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"local://", "minio://", "s3://", "oss://", "cos://", "tos://", "storage://", "file://", "x-amz-signature=", "x-oss-signature=", "q-signature=", "x-tos-signature=", "ossaccesskeyid="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// Defense in depth for stored message/reference metadata. resource:// handles
// remain usable only through an independently authorized gateway resource route.
func scrubPrivateResources(value any) {
	switch v := value.(type) {
	case []any:
		for i, item := range v {
			if text, ok := item.(string); ok && containsStorageLocator(text) {
				v[i] = ""
			} else {
				scrubPrivateResources(item)
			}
		}
	case map[string]any:
		for key, item := range v {
			if privateResourceField(key) {
				delete(v, key)
				continue
			}
			if text, ok := item.(string); ok && containsStorageLocator(text) {
				delete(v, key)
				continue
			}
			scrubPrivateResources(item)
		}
	}
}
