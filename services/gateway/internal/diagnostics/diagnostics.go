// Package diagnostics records authentication stages without credential values.
package diagnostics

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

type contextKey int

const (
	requestKey contextKey = iota
	flowKey
	instanceKey
	loggerKey
)

func NewID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(raw[:])
}

func WithRequestID(ctx context.Context, id string) context.Context {
	if len(id) > 128 || strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:-", r))
	}) >= 0 {
		id = "invalid"
	}
	return context.WithValue(ctx, requestKey, id)
}

func WithFlow(ctx context.Context, instance, flow string) context.Context {
	return context.WithValue(context.WithValue(ctx, instanceKey, instance), flowKey, flow)
}

func WithLogger(ctx context.Context, logger *log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// Event accepts fields constructed by product code, never raw headers, query,
// response bodies, claims or err.Error(). ErrorKind is safe for wrapped URL errors.
func Event(ctx context.Context, name string, fields map[string]any) {
	event := make(map[string]any, len(fields)+5)
	for key, value := range fields {
		event[key] = value
	}
	event["event"] = name
	event["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	for key, name := range map[contextKey]string{requestKey: "request_id", flowKey: "flow_id", instanceKey: "instance_id"} {
		if value, _ := ctx.Value(key).(string); value != "" {
			event[name] = value
		}
	}
	logger, _ := ctx.Value(loggerKey).(*log.Logger)
	if logger == nil {
		logger = log.Default()
	}
	encoded, err := json.Marshal(event)
	if err == nil {
		logger.Println(string(encoded))
	}
}

func ErrorKind(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var roots x509.SystemRootsError
	var tlsVerify *tls.CertificateVerificationError
	switch {
	case errors.As(err, &unknown):
		return "tls_unknown_authority"
	case errors.As(err, &hostname):
		return "tls_hostname_mismatch"
	case errors.As(err, &invalid):
		if invalid.Reason == x509.Expired {
			return "tls_certificate_time_invalid"
		}
		return "tls_certificate_invalid"
	case errors.As(err, &roots):
		return "tls_system_roots_unavailable"
	case errors.As(err, &tlsVerify):
		return "tls_verification_failed"
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns_failed"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "connection_reset"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "unexpected_eof"
	}
	return "operation_failed"
}

// Origin deliberately omits userinfo, path, query and fragment.
func Origin(target *url.URL) string {
	if target == nil {
		return ""
	}
	return target.Scheme + "://" + target.Host
}

func HTTP(ctx context.Context, client *http.Client, request *http.Request, stage string) (*http.Response, error) {
	started := time.Now()
	callID := NewID()
	fields := map[string]any{"stage": stage, "call_id": callID, "method": request.Method, "endpoint_origin": Origin(request.URL)}
	Event(ctx, "identity_http_started", fields)
	response, err := client.Do(request)
	fields["duration_ms"] = time.Since(started).Milliseconds()
	fields["error_kind"] = ErrorKind(err)
	fields["context_state"] = ErrorKind(ctx.Err())
	if response != nil {
		fields["status"] = response.StatusCode
	}
	Event(ctx, "identity_http_finished", fields)
	return response, err
}

// PathLabel exposes only fixed authentication paths, never resource identifiers.
func PathLabel(path string) string {
	switch path {
	case "/health", "/version", "/api/v1/auth/me", "/api/v1/auth/refresh", "/api/v1/auth/logout",
		"/api/v1/auth/oidc/config", "/api/v1/auth/oidc/login", "/api/v1/auth/oidc/callback",
		"/api/v1/mindcreek/auth/config", "/api/v1/mindcreek/onboarding", "/api/v1/mindcreek/installation",
		"/api/v1/mindcreek/admin/auth/login", "/api/v1/mindcreek/admin/auth/refresh",
		"/api/v1/mindcreek/admin/auth/logout", "/api/v1/mindcreek/admin/auth/change-password",
		"/api/v1/mindcreek/oidc/.well-known/openid-configuration", "/api/v1/mindcreek/oidc/authorize",
		"/api/v1/mindcreek/oidc/callback", "/api/v1/mindcreek/oidc/token", "/api/v1/mindcreek/oidc/userinfo",
		"/api/v1/mindcreek/oidc/jwks", "/api/v1/mindcreek/oidc/status", "/api/v1/mindcreek/oidc/logout", "/mcp":
		return path
	default:
		if strings.HasPrefix(path, "/api/") {
			return "/api/*"
		}
		return "/*"
	}
}
