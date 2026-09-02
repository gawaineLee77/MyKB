package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
)

// OAuth2Provider adapts a corporate Authorization Code provider that exposes
// identity only through a bearer-protected UserInfo endpoint. It never sends
// corporate access or refresh tokens to WeKnora or the browser.
type OAuth2Provider struct {
	settings config.IdentityConfig
	client   *http.Client
}

func NewOAuth2Provider(settings config.IdentityConfig, client *http.Client) (*OAuth2Provider, error) {
	if !settings.Enabled || settings.AuthorizationURL == nil || settings.TokenURL == nil || settings.UserInfoURL == nil ||
		settings.Issuer == "" || settings.CorporateRedirectURI == "" || settings.ClientID == "" || settings.ClientSecret == "" ||
		(settings.ClientAuthMethod != "client_secret_post" && settings.ClientAuthMethod != "client_secret_basic") ||
		(settings.AuthorizationMethod != http.MethodGet && settings.AuthorizationMethod != http.MethodPost) ||
		(settings.TokenRequestFormat != "form" && settings.TokenRequestFormat != "json") ||
		(settings.UserInfoTokenTransport != "bearer" && settings.UserInfoTokenTransport != "query") ||
		settings.AuthorizationGrant == "" || settings.SubjectClaim == "" ||
		(settings.SubjectTenantScoped && settings.TenantClaim == "") {
		return nil, fmt.Errorf("corporate OAuth2 settings are incomplete")
	}
	if client == nil {
		client = corporateHTTPClient()
	}
	return &OAuth2Provider{settings: settings, client: client}, nil
}

func (p *OAuth2Provider) AuthorizationRequest(_ context.Context, state, _ string, challenge string) (AuthorizationRequest, error) {
	target := *p.settings.AuthorizationURL
	query := target.Query()
	query.Set("response_type", "code")
	query.Set("client_id", p.settings.ClientID)
	query.Set("redirect_uri", p.settings.CorporateRedirectURI)
	if p.settings.AuthorizationDisplay != "" {
		query.Set("display", p.settings.AuthorizationDisplay)
	}
	if p.settings.StateRequired {
		query.Set("state", state)
	}
	if len(p.settings.Scopes) > 0 {
		query.Set("scope", strings.Join(p.settings.Scopes, " "))
	}
	if p.settings.PKCEEnabled {
		if challenge == "" {
			return AuthorizationRequest{}, fmt.Errorf("corporate OAuth2 PKCE challenge is missing")
		}
		query.Set("code_challenge", challenge)
		query.Set("code_challenge_method", "S256")
	}
	if p.settings.AuthorizationMethod == http.MethodPost {
		target.RawQuery = ""
		return AuthorizationRequest{Method: http.MethodPost, URL: target.String(), Form: query}, nil
	}
	target.RawQuery = query.Encode()
	return AuthorizationRequest{Method: http.MethodGet, URL: target.String()}, nil
}

func (p *OAuth2Provider) Authenticate(ctx context.Context, code, verifier, _ string) (Claims, error) {
	form := url.Values{
		"grant_type":   {p.settings.AuthorizationGrant},
		"code":         {code},
		"redirect_uri": {p.settings.CorporateRedirectURI},
	}
	if p.settings.PKCEEnabled {
		if verifier == "" {
			return Claims{}, fmt.Errorf("corporate OAuth2 PKCE verifier is missing")
		}
		form.Set("code_verifier", verifier)
	}
	if p.settings.ClientAuthMethod == "client_secret_post" {
		form.Set("client_id", p.settings.ClientID)
		form.Set("client_secret", p.settings.ClientSecret)
	}
	var body string
	contentType := "application/x-www-form-urlencoded"
	if p.settings.TokenRequestFormat == "json" {
		payload := make(map[string]string, len(form))
		for key := range form {
			payload[key] = form.Get(key)
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return Claims{}, fmt.Errorf("encode corporate OAuth2 token request: %w", err)
		}
		body = string(encoded)
		contentType = "application/json"
	} else {
		body = form.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.settings.TokenURL.String(), strings.NewReader(body))
	if err != nil {
		return Claims{}, fmt.Errorf("create corporate OAuth2 token request: %w", err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/json")
	if p.settings.ClientAuthMethod == "client_secret_basic" {
		request.SetBasicAuth(p.settings.ClientID, p.settings.ClientSecret)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return Claims{}, fmt.Errorf("exchange corporate OAuth2 authorization code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Claims{}, fmt.Errorf("corporate OAuth2 token endpoint returned status %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	if err := decodeLimitedJSON(response.Body, &token); err != nil {
		return Claims{}, fmt.Errorf("corporate OAuth2 token response is invalid")
	}
	accessToken := strings.TrimSpace(token.AccessToken)
	if accessToken == "" || len(accessToken) > 8192 ||
		(token.TokenType != "" && !strings.EqualFold(token.TokenType, "bearer")) {
		return Claims{}, fmt.Errorf("corporate OAuth2 token response is invalid")
	}
	scope := strings.TrimSpace(token.Scope)
	if scope == "" {
		scope = strings.Join(p.settings.Scopes, " ")
	}
	if len(scope) > 4096 {
		return Claims{}, fmt.Errorf("corporate OAuth2 token response is invalid")
	}
	return p.loadClaims(ctx, accessToken, scope)
}

func (p *OAuth2Provider) EndSessionURL() string { return "" }

func (p *OAuth2Provider) loadClaims(ctx context.Context, accessToken, scope string) (Claims, error) {
	target := *p.settings.UserInfoURL
	if p.settings.UserInfoTokenTransport == "query" {
		query := target.Query()
		query.Set("access_token", accessToken)
		query.Set("client_id", p.settings.ClientID)
		if scope != "" {
			query.Set("scope", scope)
		}
		target.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Claims{}, fmt.Errorf("create corporate OAuth2 UserInfo request: %w", err)
	}
	if p.settings.UserInfoTokenTransport == "bearer" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		if p.settings.UserInfoTokenTransport == "query" {
			return Claims{}, fmt.Errorf("load corporate OAuth2 UserInfo")
		}
		return Claims{}, fmt.Errorf("load corporate OAuth2 UserInfo: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Claims{}, fmt.Errorf("corporate OAuth2 UserInfo returned status %d", response.StatusCode)
	}
	var document map[string]any
	if err := decodeLimitedJSON(response.Body, &document); err != nil {
		return Claims{}, fmt.Errorf("decode corporate OAuth2 UserInfo: %w", err)
	}
	userInfo, err := objectAtPath(document, p.settings.UserInfoDataPath)
	if err != nil {
		return Claims{}, err
	}
	globalUserID := claimText(userInfo[p.settings.SubjectClaim])
	tenantID := claimText(userInfo[p.settings.TenantClaim])
	if globalUserID == "" || (p.settings.SubjectTenantScoped && tenantID == "") {
		return Claims{}, fmt.Errorf("corporate OAuth2 UserInfo is missing its stable identity")
	}
	subject := globalUserID
	if p.settings.SubjectTenantScoped {
		digest := sha256.Sum256([]byte(tenantID + "\x00" + globalUserID))
		subject = "tenant-user:" + hex.EncodeToString(digest[:])
	}
	username := claimText(userInfo[p.settings.UsernameClaim])
	if username == "" && p.settings.UUIDClaim != "" {
		username = claimText(userInfo[p.settings.UUIDClaim])
	}
	if username == "" {
		digest := sha256.Sum256([]byte(subject))
		username = "user-" + hex.EncodeToString(digest[:8])
	}
	displayName := claimText(userInfo[p.settings.DisplayNameClaim])
	if displayName == "" {
		displayName = username
	}
	employeeTypes := claimTexts(userInfo[p.settings.EmployeeTypeClaim])
	claims := Claims{
		Issuer:      p.settings.Issuer,
		Subject:     subject,
		Username:    username,
		DisplayName: displayName,
		Groups:      claimTexts(userInfo[p.settings.GroupClaim]),
	}
	if p.settings.EmailClaim != "" {
		claims.CorporateEmail = claimText(userInfo[p.settings.EmailClaim])
	}
	if claims.CorporateEmail == "" {
		_, claims.CorporateEmail = stableAliases(claims.Issuer, claims.Subject)
	}
	for _, employeeType := range employeeTypes {
		claims.Groups = append(claims.Groups, "employee-type:"+employeeType)
	}
	claims = normalizeClaims(claims)
	if err := validateClaims(claims); err != nil {
		return Claims{}, fmt.Errorf("corporate OAuth2 UserInfo is missing required mapped values")
	}
	if !valuesAllowed(employeeTypes, p.settings.AllowedEmployeeTypes) {
		return claims, ErrEmployeeTypeDenied
	}
	if !groupsAllowed(claims.Groups, p.settings.RequiredGroups) {
		return claims, ErrGroupDenied
	}
	return claims, nil
}

func objectAtPath(document map[string]any, path string) (map[string]any, error) {
	current := document
	for _, part := range strings.Split(strings.TrimSpace(path), ".") {
		if part == "" {
			continue
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("corporate OAuth2 UserInfo data path is unavailable")
		}
		current = next
	}
	return current, nil
}

func claimText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", typed), "0"), ".")
	default:
		return ""
	}
}

func claimTexts(value any) []string {
	switch typed := value.(type) {
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := claimText(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		text := claimText(value)
		if text == "" {
			return nil
		}
		return strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' })
	}
}

func valuesAllowed(values []string, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, value := range values {
		if allowed[strings.ToLower(strings.TrimSpace(value))] {
			return true
		}
	}
	return false
}
