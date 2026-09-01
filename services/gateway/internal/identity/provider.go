package identity

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
)

type Provider interface {
	AuthorizationRequest(context.Context, string, string, string) (AuthorizationRequest, error)
	Authenticate(context.Context, string, string, string) (Claims, error)
	EndSessionURL() string
}

type AuthorizationRequest struct {
	Method string
	URL    string
	Form   url.Values
}

type providerMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type CorporateProvider struct {
	settings config.IdentityConfig
	client   *http.Client
	mu       sync.Mutex
	metadata *providerMetadata
}

// NewProvider selects the corporate-facing protocol adapter. MindCreek still
// exposes its private OIDC broker to WeKnora regardless of this selection.
func NewProvider(settings config.IdentityConfig, client *http.Client) (Provider, error) {
	switch settings.Protocol {
	case config.IdentityProtocolOAuth2:
		return NewOAuth2Provider(settings, client)
	case config.IdentityProtocolOIDC, "":
		return NewCorporateProvider(settings, client)
	default:
		return nil, fmt.Errorf("unsupported corporate identity protocol")
	}
}

func NewCorporateProvider(settings config.IdentityConfig, client *http.Client) (*CorporateProvider, error) {
	if !settings.Enabled || settings.DiscoveryURL == nil {
		return nil, fmt.Errorf("corporate identity settings are incomplete")
	}
	if client == nil {
		client = corporateHTTPClient()
	}
	return &CorporateProvider{settings: settings, client: client}, nil
}

func corporateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("corporate identity redirects are disabled")
		},
	}
}

func (p *CorporateProvider) AuthorizationRequest(ctx context.Context, state, nonce, challenge string) (AuthorizationRequest, error) {
	metadata, err := p.loadMetadata(ctx)
	if err != nil {
		return AuthorizationRequest{}, err
	}
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.settings.ClientID},
		"redirect_uri":          {p.settings.CallbackURL},
		"scope":                 {strings.Join(p.settings.Scopes, " ")},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	target, err := url.Parse(metadata.AuthorizationEndpoint)
	if err != nil {
		return AuthorizationRequest{}, fmt.Errorf("invalid corporate authorization endpoint")
	}
	target.RawQuery = query.Encode()
	return AuthorizationRequest{Method: http.MethodGet, URL: target.String()}, nil
}

func (p *CorporateProvider) Authenticate(ctx context.Context, code, verifier, nonce string) (Claims, error) {
	metadata, err := p.loadMetadata(ctx)
	if err != nil {
		return Claims{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.settings.CallbackURL},
		"client_id":     {p.settings.ClientID},
		"client_secret": {p.settings.ClientSecret},
		"code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Claims{}, fmt.Errorf("create corporate token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return Claims{}, fmt.Errorf("exchange corporate authorization code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Claims{}, fmt.Errorf("corporate token endpoint returned status %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
	}
	if err := decodeLimitedJSON(response.Body, &token); err != nil || token.IDToken == "" {
		return Claims{}, fmt.Errorf("corporate token response is invalid")
	}
	verified, err := p.verifyIDToken(ctx, metadata, token.IDToken, nonce, time.Now().UTC())
	if err != nil {
		return Claims{}, err
	}
	if metadata.UserInfoEndpoint != "" && token.AccessToken != "" {
		userInfo, err := p.userInfo(ctx, metadata.UserInfoEndpoint, token.AccessToken)
		if err != nil {
			return Claims{}, err
		}
		if claimString(userInfo, "sub") != claimString(verified, "sub") {
			return Claims{}, fmt.Errorf("corporate userinfo subject mismatch")
		}
		for key, value := range userInfo {
			if key != "iss" && key != "aud" && key != "exp" && key != "iat" && key != "nonce" && key != "sub" {
				verified[key] = value
			}
		}
	}
	claims := Claims{
		Issuer:         p.settings.Issuer,
		Subject:        claimString(verified, "sub"),
		CorporateEmail: claimString(verified, p.settings.EmailClaim),
		Username:       claimString(verified, p.settings.UsernameClaim),
		DisplayName:    claimString(verified, p.settings.DisplayNameClaim),
		Groups:         claimStrings(verified[p.settings.GroupClaim]),
	}
	if claims.Username == "" {
		claims.Username = strings.Split(claims.CorporateEmail, "@")[0]
	}
	normalized := normalizeClaims(claims)
	if err := validateClaims(normalized); err != nil {
		return Claims{}, fmt.Errorf("corporate identity is missing required claims")
	}
	if !groupsAllowed(normalized.Groups, p.settings.RequiredGroups) {
		return normalized, ErrGroupDenied
	}
	return normalized, nil
}

func (p *CorporateProvider) EndSessionURL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.metadata == nil {
		return ""
	}
	return p.metadata.EndSessionEndpoint
}

var ErrGroupDenied = errors.New("corporate identity is not in an approved group")
var ErrEmployeeTypeDenied = errors.New("corporate identity has an unapproved employee type")

func (p *CorporateProvider) loadMetadata(ctx context.Context) (*providerMetadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.metadata != nil {
		copy := *p.metadata
		return &copy, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.settings.DiscoveryURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load corporate OIDC discovery: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("corporate OIDC discovery returned status %d", response.StatusCode)
	}
	var metadata providerMetadata
	if err := decodeLimitedJSON(response.Body, &metadata); err != nil {
		return nil, fmt.Errorf("decode corporate OIDC discovery: %w", err)
	}
	if strings.TrimRight(metadata.Issuer, "/") != p.settings.Issuer {
		return nil, fmt.Errorf("corporate OIDC discovery issuer mismatch")
	}
	for name, endpoint := range map[string]string{
		"authorization_endpoint": metadata.AuthorizationEndpoint,
		"token_endpoint":         metadata.TokenEndpoint,
		"jwks_uri":               metadata.JWKSURI,
	} {
		if err := validateProviderURL(name, endpoint, p.settings.AllowInsecureHTTP); err != nil {
			return nil, err
		}
	}
	if metadata.UserInfoEndpoint != "" {
		if err := validateProviderURL("userinfo_endpoint", metadata.UserInfoEndpoint, p.settings.AllowInsecureHTTP); err != nil {
			return nil, err
		}
	}
	if metadata.EndSessionEndpoint != "" {
		if err := validateProviderURL("end_session_endpoint", metadata.EndSessionEndpoint, p.settings.AllowInsecureHTTP); err != nil {
			return nil, err
		}
	}
	p.metadata = &metadata
	copy := metadata
	return &copy, nil
}

func (p *CorporateProvider) userInfo(ctx context.Context, endpoint, accessToken string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load corporate userinfo: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("corporate userinfo returned status %d", response.StatusCode)
	}
	var claims map[string]any
	if err := decodeLimitedJSON(response.Body, &claims); err != nil {
		return nil, fmt.Errorf("decode corporate userinfo: %w", err)
	}
	return claims, nil
}

func (p *CorporateProvider) verifyIDToken(ctx context.Context, metadata *providerMetadata, raw, nonce string, now time.Time) (map[string]any, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("corporate ID token is malformed")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeJWTPart(parts[0], &header); err != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return nil, fmt.Errorf("corporate ID token uses an unsupported signature")
	}
	key, err := p.loadRSAKey(ctx, metadata.JWKSURI, header.KeyID)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("corporate ID token signature is malformed")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return nil, fmt.Errorf("corporate ID token signature is invalid")
	}
	var claims map[string]any
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("corporate ID token claims are invalid")
	}
	if subtle.ConstantTimeCompare([]byte(claimString(claims, "iss")), []byte(p.settings.Issuer)) != 1 ||
		!audienceContains(claims["aud"], p.settings.ClientID) ||
		subtle.ConstantTimeCompare([]byte(claimString(claims, "nonce")), []byte(nonce)) != 1 ||
		claimString(claims, "sub") == "" {
		return nil, fmt.Errorf("corporate ID token issuer, audience, nonce, or subject is invalid")
	}
	if audiences := claimStrings(claims["aud"]); len(audiences) > 1 &&
		subtle.ConstantTimeCompare([]byte(claimString(claims, "azp")), []byte(p.settings.ClientID)) != 1 {
		return nil, fmt.Errorf("corporate ID token authorized party is invalid")
	}
	expires, ok := numericDate(claims["exp"])
	if !ok || !expires.After(now.Add(-30*time.Second)) {
		return nil, fmt.Errorf("corporate ID token is expired")
	}
	if issued, ok := numericDate(claims["iat"]); ok && issued.After(now.Add(2*time.Minute)) {
		return nil, fmt.Errorf("corporate ID token issue time is invalid")
	}
	return claims, nil
}

func (p *CorporateProvider) loadRSAKey(ctx context.Context, endpoint, keyID string) (*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load corporate signing keys: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("corporate JWKS returned status %d", response.StatusCode)
	}
	var document struct {
		Keys []struct {
			KeyType   string `json:"kty"`
			KeyID     string `json:"kid"`
			Use       string `json:"use"`
			Algorithm string `json:"alg"`
			Modulus   string `json:"n"`
			Exponent  string `json:"e"`
		} `json:"keys"`
	}
	if err := decodeLimitedJSON(response.Body, &document); err != nil {
		return nil, fmt.Errorf("decode corporate JWKS: %w", err)
	}
	for _, candidate := range document.Keys {
		if candidate.KeyID != keyID || candidate.KeyType != "RSA" || (candidate.Algorithm != "" && candidate.Algorithm != "RS256") {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(candidate.Modulus)
		if err != nil || len(modulus) < 256 {
			continue
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(candidate.Exponent)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			continue
		}
		exponent := 0
		for _, value := range exponentBytes {
			exponent = exponent<<8 | int(value)
		}
		if exponent < 3 {
			continue
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}, nil
	}
	return nil, fmt.Errorf("corporate signing key is unavailable")
}

func decodeJWTPart(raw string, destination any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) > 64<<10 {
		return fmt.Errorf("invalid JWT section")
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.UseNumber()
	return decoder.Decode(destination)
}

func decodeLimitedJSON(source io.Reader, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(source, 1<<20))
	decoder.UseNumber()
	return decoder.Decode(destination)
}

func numericDate(value any) (time.Time, bool) {
	var seconds int64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return time.Time{}, false
		}
		seconds = parsed
	case float64:
		seconds = int64(typed)
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		seconds = parsed
	default:
		return time.Time{}, false
	}
	return time.Unix(seconds, 0).UTC(), true
}

func claimString(claims map[string]any, name string) string {
	value, _ := claims[name].(string)
	return strings.TrimSpace(value)
}

func claimStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == ' ' })
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	case []string:
		return typed
	default:
		return nil
	}
}

func audienceContains(value any, clientID string) bool {
	for _, candidate := range claimStrings(value) {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(clientID)) == 1 {
			return true
		}
	}
	return false
}

func groupsAllowed(groups []string, required map[string]bool) bool {
	if len(required) == 0 {
		return true
	}
	for _, group := range groups {
		if required[strings.ToLower(strings.TrimSpace(group))] {
			return true
		}
	}
	return false
}

func validateProviderURL(name, raw string, allowHTTP bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http")) {
		return fmt.Errorf("corporate OIDC %s is not an approved URL", name)
	}
	return nil
}
