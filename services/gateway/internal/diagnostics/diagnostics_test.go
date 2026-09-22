package diagnostics

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTransportErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		kind string
	}{
		{"cancel", context.Canceled, "context_canceled"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"unknown CA", x509.UnknownAuthorityError{}, "tls_unknown_authority"},
		{"hostname", x509.HostnameError{}, "tls_hostname_mismatch"},
		{"expired", x509.CertificateInvalidError{Reason: x509.Expired}, "tls_certificate_time_invalid"},
		{"wrapped TLS", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "tls_unknown_authority"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &url.Error{Op: "Get", URL: "https://identity.test/userinfo?access_token=SECRET", Err: tc.err}
			if got := ErrorKind(err); got != tc.kind {
				t.Fatalf("kind = %q, want %q", got, tc.kind)
			}
		})
	}
}

func TestTLSFailureAndTrustSuccessAreDiagnosableWithoutSecrets(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"RESPONSE_SECRET"}`)
	}))
	defer server.Close()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	var output bytes.Buffer
	ctx := WithLogger(WithRequestID(WithFlow(context.Background(), "instance", "flow"), "request"), log.New(&output, "", 0))
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/private-path?access_token=QUERY_SECRET", nil)
	request.Header.Set("Authorization", "Bearer HEADER_SECRET")
	_, err := HTTP(ctx, &http.Client{}, request, "corporate_userinfo")
	if err == nil || ErrorKind(err) != "tls_unknown_authority" {
		t.Fatalf("expected unknown CA, got kind %s", ErrorKind(err))
	}
	response, err := HTTP(ctx, server.Client(), request, "corporate_userinfo")
	if err != nil || response.StatusCode != 200 {
		t.Fatal("trusted TLS request did not succeed")
	}
	defer response.Body.Close()
	for _, required := range []string{`"error_kind":"tls_unknown_authority"`, `"status":200`, `"flow_id":"flow"`, `"request_id":"request"`} {
		if !strings.Contains(output.String(), required) {
			t.Fatalf("diagnostic omitted %s", required)
		}
	}
	for _, forbidden := range []string{"QUERY_SECRET", "HEADER_SECRET", "RESPONSE_SECRET", "private-path", "access_token="} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("diagnostics exposed %s", forbidden)
		}
	}
}

func TestCanceledRequestRemainsCanceledAndDoesNotCallServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	ctx = WithLogger(ctx, log.New(&output, "", 0))
	request, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:1", nil)
	_, err := HTTP(ctx, &http.Client{}, request, "upstream_auth_me")
	if !errors.Is(err, context.Canceled) || !strings.Contains(output.String(), `"context_state":"context_canceled"`) {
		t.Fatal("cancellation semantics or context diagnostic changed")
	}
}

func TestPathLabelsNeverContainResourceOrQueryValues(t *testing.T) {
	for _, path := range []string{"/api/v1/knowledge/secret", "/r/private-token", "/api/v1/auth/me?token=secret"} {
		if strings.Contains(PathLabel(path), "secret") || strings.Contains(PathLabel(path), "token") {
			t.Fatal("sensitive path was exposed")
		}
	}
	if PathLabel("/api/v1/auth/me") != "/api/v1/auth/me" {
		t.Fatal("fixed authentication path missing")
	}
}
