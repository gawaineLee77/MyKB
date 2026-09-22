package identity

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/url"
	"os"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/diagnostics"
)

func (b *Broker) logConfiguration(ctx context.Context) {
	redirect, _ := url.Parse(b.settings.CorporateRedirectURI)
	fields := map[string]any{
		"protocol": b.settings.Protocol, "authorization_method": b.settings.AuthorizationMethod,
		"token_request_format": b.settings.TokenRequestFormat, "client_auth_method": b.settings.ClientAuthMethod,
		"state_required": b.settings.StateRequired, "pkce_enabled": b.settings.PKCEEnabled,
		"userinfo_token_transport": b.settings.UserInfoTokenTransport,
		"subject_tenant_scoped":    b.settings.SubjectTenantScoped,
		"subject_claim":            b.settings.SubjectClaim, "tenant_claim": b.settings.TenantClaim,
		"username_claim": b.settings.UsernameClaim, "email_claim": b.settings.EmailClaim,
		"authorization_origin": diagnostics.Origin(b.settings.AuthorizationURL),
		"token_origin":         diagnostics.Origin(b.settings.TokenURL),
		"userinfo_origin":      diagnostics.Origin(b.settings.UserInfoURL),
		"redirect_origin":      diagnostics.Origin(redirect),
		"transaction_storage":  "process_memory", "transaction_ttl_seconds": int(transactionTTL.Seconds()),
		"cookie_path": "/api/v1/mindcreek/oidc/callback", "cookie_secure": b.settings.ExternalOrigin.Scheme == "https",
	}
	if redirect != nil {
		switch redirect.Path {
		case "", "/":
			fields["redirect_target"] = "origin_root"
		case "/api/v1/mindcreek/oidc/callback":
			fields["redirect_target"] = "broker_callback"
		default:
			fields["redirect_target"] = "custom_path"
		}
	}
	file := os.Getenv("SSL_CERT_FILE")
	fields["ca_bundle_configured"] = file != ""
	if file != "" {
		data, err := os.ReadFile(file)
		fields["ca_bundle_readable"] = err == nil
		if err == nil {
			sum := sha256.Sum256(data)
			fields["ca_bundle_sha256"] = hex.EncodeToString(sum[:])
			count := 0
			for len(data) > 0 {
				var block *pem.Block
				block, data = pem.Decode(data)
				if block == nil {
					break
				}
				if block.Type == "CERTIFICATE" {
					if _, err := x509.ParseCertificate(block.Bytes); err == nil {
						count++
					}
				}
			}
			fields["ca_certificate_count"] = count
		}
	}
	diagnostics.Event(diagnostics.WithFlow(ctx, b.instanceID, ""), "identity_configuration", fields)
}
