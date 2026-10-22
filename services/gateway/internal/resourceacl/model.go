// Package resourceacl reads the pinned WeKnora v0.8.0 resource registry and
// immutable processing provenance. It never infers ownership from editable
// document bodies, chunk image_info, or attachment lifecycle bindings.
package resourceacl

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrInvalidReference    = errors.New("resource_acl.invalid_reference")
	ErrNotFound            = errors.New("resource_acl.not_found")
	ErrUnavailable         = errors.New("resource_acl.unavailable")
	ErrFingerprintMismatch = errors.New("resource_acl.fingerprint_mismatch")
	ErrInvalidApproval     = errors.New("resource_acl.invalid_approval")
)

const PinnedUpstream = "v0.8.0/1edcd54b43606d9079bb36650efe3f68707a79ea"

type Owner struct {
	KBID        string `json:"knowledge_base_id"`
	KnowledgeID string `json:"knowledge_id"`
	Provenance  string `json:"provenance"`
}

type Resource struct {
	ID            string  `json:"resource_id"`
	Reference     string  `json:"reference"`
	Fingerprint   string  `json:"fingerprint"`
	TenantID      uint64  `json:"tenant_id"`
	Owners        []Owner `json:"owners"`
	SessionScoped bool    `json:"session_scoped,omitempty"`
	PhysicalPath  string  `json:"-"`
}

type AuditItem struct {
	Resource
	Reason string `json:"reason,omitempty"`
}

// Verified and Unverified count the Scanned sample, not unscanned objects.
// Total is the registered active image count before the bounded scan.
type AuditReport struct {
	TenantID   uint64      `json:"tenant_id"`
	Total      int         `json:"total"`
	Scanned    int         `json:"scanned"`
	Verified   int         `json:"verified"`
	Unverified int         `json:"unverified"`
	Truncated  bool        `json:"truncated"`
	NextCursor string      `json:"next_cursor,omitempty"`
	Items      []AuditItem `json:"items"`
}

var handlePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
var backendPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func fingerprint(tenant uint64, id, physical string) string {
	return digest("mindcreek-resource-v1\x00" + strconv.FormatUint(tenant, 10) + "\x00" + id + "\x00" + physical)
}

func handleOf(reference string) (string, bool) {
	handle, ok := strings.CutPrefix(reference, "resource://")
	return handle, ok && handlePattern.MatchString(handle)
}

// validPhysical accepts only the canonical exports paths the pinned native KB
// proxy serves. No decoding, path cleaning, or basename matching is performed:
// aliases and traversal must never acquire another object's provenance.
func validPhysical(value string, tenant uint64) bool {
	if tenant == 0 || value == "" || len(value) > 4096 || strings.ContainsAny(value, "\\%?#") ||
		strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	inner := value
	if rest, ok := strings.CutPrefix(inner, "storage://"); ok {
		backend, provider, found := strings.Cut(rest, "/")
		if !found || !backendPattern.MatchString(backend) {
			return false
		}
		inner = provider
	}
	scheme, rest, found := strings.Cut(inner, "://")
	if !found {
		return false
	}
	switch scheme {
	case "local", "minio", "cos", "tos", "s3", "oss", "ks3", "obs":
	default:
		return false
	}
	parts := strings.Split(rest, "/")
	tenantPart := strconv.FormatUint(tenant, 10)
	firstNumber := ""
	exports := false
	for i, segment := range parts {
		if segment == "" || segment == "." || segment == ".." || strings.Contains(segment, ":") {
			return false
		}
		if _, err := strconv.ParseUint(segment, 10, 64); err == nil && firstNumber == "" {
			firstNumber = segment
		}
		if segment == tenantPart && ((i+2 < len(parts) && parts[i+1] == "exports") ||
			(i > 0 && i+1 < len(parts) && parts[i-1] == "exports")) {
			exports = true
		}
	}
	if firstNumber != tenantPart || !exports {
		return false
	}
	switch strings.ToLower(path.Ext(parts[len(parts)-1])) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".tif", ".tiff", ".avif":
		return true
	default:
		return false
	}
}
