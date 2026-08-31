package observability

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecorderUsesBoundedLabelsAndRedactedLogs(t *testing.T) {
	var output bytes.Buffer
	recorder := NewRecorder(log.New(&output, "", 0))
	handler := recorder.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases/secret-id?prompt=private-text", strings.NewReader("private-body"))
	request.Header.Set("Authorization", "Bearer private-token")
	request.Header.Set("X-Request-ID", "request-safe")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	logs := output.String()
	for _, forbidden := range []string{"secret-id", "private-text", "private-body", "private-token"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("structured log disclosed %q: %s", forbidden, logs)
		}
	}
	for _, required := range []string{`"event":"http_request"`, `"request_id":"request-safe"`, `"route_class":"upstream_api"`} {
		if !strings.Contains(logs, required) {
			t.Fatalf("structured log omitted %s: %s", required, logs)
		}
	}

	metrics := httptest.NewRecorder()
	recorder.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/internal/metrics", nil))
	body := metrics.Body.String()
	for _, required := range []string{"mindcreek_gateway_http_requests_total", "mindcreek_gateway_security_denials_total{status=\"403\"} 1"} {
		if !strings.Contains(body, required) {
			t.Fatalf("metrics omitted %q: %s", required, body)
		}
	}
}
