package identity

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gawaineLee77/MyKB/services/gateway/internal/config"
)

const (
	transactionTTL = 10 * time.Minute
	brokerCodeTTL  = 2 * time.Minute
	brokerTokenTTL = 2 * time.Minute
	loginCookie    = "mindcreek_sso_transaction"
	brokerKeyID    = "mindcreek-broker-1"
)

type authTransaction struct {
	CookieBinding    string
	CorporateNonce   string
	PKCEVerifier     string
	UpstreamState    string
	UpstreamRedirect string
	ExpiresAt        time.Time
}

type authorizationCode struct {
	Identity    Identity
	RedirectURI string
	ExpiresAt   time.Time
}

type accessGrant struct {
	Identity  Identity
	ExpiresAt time.Time
}

type Broker struct {
	settings   config.IdentityConfig
	provider   Provider
	store      Store
	signingKey *rsa.PrivateKey
	now        func() time.Time

	mu                 sync.Mutex
	transactions       map[string]authTransaction
	transactionCookies map[string]string
	codes              map[string]authorizationCode
	tokens             map[string]accessGrant
	handler            http.Handler
}

func NewBroker(settings config.IdentityConfig, provider Provider, store Store) (*Broker, error) {
	if !settings.Enabled || provider == nil || store == nil || settings.ExternalOrigin == nil ||
		settings.BrokerClientID == "" || len(settings.BrokerClientSecret) < 32 {
		return nil, fmt.Errorf("identity broker configuration is incomplete")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate identity broker signing key: %w", err)
	}
	broker := &Broker{
		settings: settings, provider: provider, store: store, signingKey: key,
		now: time.Now, transactions: make(map[string]authTransaction),
		transactionCookies: make(map[string]string),
		codes:              make(map[string]authorizationCode), tokens: make(map[string]accessGrant),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/.well-known/openid-configuration", broker.discovery)
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/status", broker.status)
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/authorize", broker.authorize)
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/callback", broker.callback)
	mux.HandleFunc("POST /api/v1/mindcreek/oidc/token", broker.token)
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/userinfo", broker.userInfo)
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/jwks", broker.jwks)
	mux.HandleFunc("GET /api/v1/mindcreek/oidc/logout", broker.logout)
	broker.handler = mux
	return broker, nil
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	b.handler.ServeHTTP(w, r)
}

func (b *Broker) discovery(w http.ResponseWriter, _ *http.Request) {
	writeBrokerJSON(w, http.StatusOK, map[string]any{
		"issuer":                                b.settings.BrokerIssuer,
		"authorization_endpoint":                b.settings.BrokerIssuer + "/authorize",
		"token_endpoint":                        b.settings.BrokerIssuer + "/token",
		"userinfo_endpoint":                     b.settings.BrokerIssuer + "/userinfo",
		"jwks_uri":                              b.settings.BrokerIssuer + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"pairwise"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "groups"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
	})
}

func (b *Broker) status(w http.ResponseWriter, _ *http.Request) {
	writeBrokerJSON(w, http.StatusOK, map[string]any{
		"enabled": true, "provider_display_name": b.settings.ProviderName,
		"corporate_protocol": b.settings.Protocol, "authorization_method": b.settings.AuthorizationMethod,
		"state_required": b.settings.StateRequired, "pkce_enabled": b.settings.PKCEEnabled,
		"userinfo_token_transport": b.settings.UserInfoTokenTransport, "registration": "closed",
	})
}

func (b *Broker) authorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("response_type") != "code" ||
		subtle.ConstantTimeCompare([]byte(query.Get("client_id")), []byte(b.settings.BrokerClientID)) != 1 ||
		subtle.ConstantTimeCompare([]byte(query.Get("redirect_uri")), []byte(b.settings.BrokerRedirectURI)) != 1 ||
		!scopeContains(query.Get("scope"), "openid") || len(query.Get("state")) < 16 || len(query.Get("state")) > 4096 {
		writeBrokerError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	corporateState, err := randomToken(32)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	cookieBinding, err := randomToken(32)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	corporateNonce, err := randomToken(32)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	verifier, err := randomToken(48)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	authorization, err := b.provider.AuthorizationRequest(r.Context(), corporateState, corporateNonce, challenge)
	if err != nil {
		writeBrokerError(w, http.StatusBadGateway, "identity_provider_unavailable")
		return
	}
	now := b.now().UTC()
	b.mu.Lock()
	b.cleanupLocked(now)
	stateHash := hashText(corporateState)
	b.transactions[stateHash] = authTransaction{
		CookieBinding: cookieBinding, CorporateNonce: corporateNonce, PKCEVerifier: verifier,
		UpstreamState: query.Get("state"), UpstreamRedirect: query.Get("redirect_uri"), ExpiresAt: now.Add(transactionTTL),
	}
	b.transactionCookies[hashText(cookieBinding)] = stateHash
	b.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: loginCookie, Value: cookieBinding, Path: "/api/v1/mindcreek/oidc/callback",
		MaxAge: int(transactionTTL.Seconds()), HttpOnly: true,
		Secure: b.settings.ExternalOrigin.Scheme == "https", SameSite: http.SameSiteLaxMode,
	})
	if authorization.Method == http.MethodPost {
		b.writeAuthorizationPost(w, authorization)
		return
	}
	http.Redirect(w, r, authorization.URL, http.StatusFound)
}

func (b *Broker) callback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	cookie, cookieErr := r.Cookie(loginCookie)
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: "", Path: "/api/v1/mindcreek/oidc/callback", MaxAge: -1, HttpOnly: true})
	b.mu.Lock()
	stateHash := ""
	if state != "" {
		stateHash = hashText(state)
	} else if !b.settings.StateRequired && cookieErr == nil {
		stateHash = b.transactionCookies[hashText(cookie.Value)]
	}
	transaction, ok := b.transactions[stateHash]
	valid := ok && !b.now().UTC().After(transaction.ExpiresAt) && cookieErr == nil &&
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(transaction.CookieBinding)) == 1
	if valid {
		delete(b.transactions, stateHash)
		delete(b.transactionCookies, hashText(transaction.CookieBinding))
	}
	b.mu.Unlock()
	if !valid {
		writeBrokerError(w, http.StatusBadRequest, "invalid_state")
		return
	}
	if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
		b.redirectUpstreamError(w, r, transaction, "access_denied")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		b.redirectUpstreamError(w, r, transaction, "missing_code")
		return
	}
	claims, err := b.provider.Authenticate(r.Context(), code, transaction.PKCEVerifier, transaction.CorporateNonce)
	if err != nil {
		if claims.Issuer == "" || claims.Subject == "" {
			claims = Claims{Issuer: b.settings.Issuer, Subject: "unknown"}
		}
		code := "identity.validation_failed"
		if errorsIsEligibilityDenied(err) {
			if err == ErrEmployeeTypeDenied {
				code = "identity.employee_type_denied"
			} else {
				code = "identity.group_denied"
			}
			_, upstreamEmail := stableAliases(claims.Issuer, claims.Subject)
			if existing, lookupErr := b.store.GetByUpstreamEmail(r.Context(), upstreamEmail); lookupErr == nil {
				_, _ = b.store.SetStatus(r.Context(), existing.BrokerSubject, StatusSuspended, b.now().UTC())
			}
		}
		b.recordAudit(r.Context(), claims, "login", "denied", code, r)
		b.redirectUpstreamError(w, r, transaction, code)
		return
	}
	identity, err := b.store.Upsert(r.Context(), claims, b.now().UTC())
	if err != nil {
		b.recordAudit(r.Context(), claims, "login", "failure", "identity.provision_failed", r)
		b.redirectUpstreamError(w, r, transaction, "identity.provision_failed")
		return
	}
	if identity.Status != StatusActive {
		b.recordAudit(r.Context(), claims, "login", "denied", "identity.suspended", r)
		b.redirectUpstreamError(w, r, transaction, "identity.suspended")
		return
	}
	if err := b.recordAudit(r.Context(), claims, "login", "success", "", r); err != nil {
		b.redirectUpstreamError(w, r, transaction, "audit_unavailable")
		return
	}
	brokerCode, err := randomToken(32)
	if err != nil {
		b.redirectUpstreamError(w, r, transaction, "server_error")
		return
	}
	b.mu.Lock()
	b.codes[hashText(brokerCode)] = authorizationCode{Identity: identity, RedirectURI: transaction.UpstreamRedirect, ExpiresAt: b.now().UTC().Add(brokerCodeTTL)}
	b.mu.Unlock()
	target, _ := url.Parse(transaction.UpstreamRedirect)
	query := target.Query()
	query.Set("code", brokerCode)
	query.Set("state", transaction.UpstreamState)
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (b *Broker) token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "authorization_code" {
		writeBrokerError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID, clientSecret = r.Form.Get("client_id"), r.Form.Get("client_secret")
	}
	if subtle.ConstantTimeCompare([]byte(clientID), []byte(b.settings.BrokerClientID)) != 1 ||
		subtle.ConstantTimeCompare([]byte(clientSecret), []byte(b.settings.BrokerClientSecret)) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="mindcreek-oidc"`)
		writeBrokerError(w, http.StatusUnauthorized, "invalid_client")
		return
	}
	codeHash := hashText(r.Form.Get("code"))
	b.mu.Lock()
	grant, found := b.codes[codeHash]
	delete(b.codes, codeHash)
	b.mu.Unlock()
	if !found || b.now().UTC().After(grant.ExpiresAt) ||
		subtle.ConstantTimeCompare([]byte(r.Form.Get("redirect_uri")), []byte(grant.RedirectURI)) != 1 {
		writeBrokerError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	accessToken, err := randomToken(32)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	expiresAt := b.now().UTC().Add(brokerTokenTTL)
	b.mu.Lock()
	b.tokens[hashText(accessToken)] = accessGrant{Identity: grant.Identity, ExpiresAt: expiresAt}
	b.mu.Unlock()
	idToken, err := b.signIDToken(grant.Identity, expiresAt)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	writeBrokerJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken, "token_type": "Bearer", "expires_in": int(brokerTokenTTL.Seconds()), "id_token": idToken,
	})
}

func (b *Broker) userInfo(w http.ResponseWriter, r *http.Request) {
	authorization := strings.Fields(r.Header.Get("Authorization"))
	if len(authorization) != 2 || !strings.EqualFold(authorization[0], "Bearer") {
		writeBrokerError(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	b.mu.Lock()
	grant, found := b.tokens[hashText(authorization[1])]
	b.mu.Unlock()
	if !found || b.now().UTC().After(grant.ExpiresAt) {
		writeBrokerError(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	current, err := b.store.GetByUpstreamEmail(r.Context(), grant.Identity.UpstreamEmail)
	if err != nil || current.Status != StatusActive {
		writeBrokerError(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	grant.Identity = current
	writeBrokerJSON(w, http.StatusOK, map[string]any{
		"sub": grant.Identity.BrokerSubject, "email": grant.Identity.UpstreamEmail,
		"preferred_username": grant.Identity.Username, "name": grant.Identity.DisplayName,
		"groups": grant.Identity.Groups,
	})
}

func (b *Broker) jwks(w http.ResponseWriter, _ *http.Request) {
	public := b.signingKey.PublicKey
	exponent := big.NewInt(int64(public.E)).Bytes()
	writeBrokerJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": brokerKeyID,
		"n": base64.RawURLEncoding.EncodeToString(public.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(exponent),
	}}})
}

func (b *Broker) logout(w http.ResponseWriter, r *http.Request) {
	target := b.provider.EndSessionURL()
	if target == "" {
		target = b.settings.ExternalOrigin.String() + "/login"
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (b *Broker) signIDToken(identity Identity, expiresAt time.Time) (string, error) {
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": brokerKeyID})
	claims, _ := json.Marshal(map[string]any{
		"iss": b.settings.BrokerIssuer, "sub": identity.BrokerSubject, "aud": b.settings.BrokerClientID,
		"iat": b.now().UTC().Unix(), "exp": expiresAt.Unix(), "email": identity.UpstreamEmail,
		"preferred_username": identity.Username, "name": identity.DisplayName, "groups": identity.Groups,
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, b.signingKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (b *Broker) redirectUpstreamError(w http.ResponseWriter, r *http.Request, transaction authTransaction, code string) {
	target, _ := url.Parse(transaction.UpstreamRedirect)
	query := target.Query()
	query.Set("error", "access_denied")
	query.Set("error_description", code)
	query.Set("state", transaction.UpstreamState)
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (b *Broker) recordAudit(ctx context.Context, claims Claims, action, outcome, errorCode string, r *http.Request) error {
	id, err := randomUUID()
	if err != nil {
		return err
	}
	correlationID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if correlationID == "" {
		correlationID, _ = randomToken(16)
	}
	return b.store.RecordAudit(ctx, AuditEvent{
		ID: id, Issuer: claims.Issuer, Subject: claims.Subject, Action: action, Outcome: outcome,
		ErrorCode: errorCode, CorrelationID: correlationID, SourceIP: r.RemoteAddr, CreatedAt: b.now().UTC(),
	})
}

func (b *Broker) cleanupLocked(now time.Time) {
	for key, value := range b.transactions {
		if now.After(value.ExpiresAt) {
			delete(b.transactions, key)
			delete(b.transactionCookies, hashText(value.CookieBinding))
		}
	}
	for key, value := range b.codes {
		if now.After(value.ExpiresAt) {
			delete(b.codes, key)
		}
	}
	for key, value := range b.tokens {
		if now.After(value.ExpiresAt) {
			delete(b.tokens, key)
		}
	}
}

func (b *Broker) writeAuthorizationPost(w http.ResponseWriter, request AuthorizationRequest) {
	target, err := url.Parse(request.URL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeBrokerError(w, http.StatusBadGateway, "identity_provider_unavailable")
		return
	}
	nonce, err := randomToken(16)
	if err != nil {
		writeBrokerError(w, http.StatusInternalServerError, "server_error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action "+target.Scheme+"://"+target.Host+"; script-src 'nonce-"+nonce+"'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Corporate sign-in</title></head><body><form id=\"corporate-login\" method=\"post\" action=\"%s\">", html.EscapeString(request.URL))
	keys := make([]string, 0, len(request.Form))
	for key := range request.Form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range request.Form[key] {
			_, _ = fmt.Fprintf(w, "<input type=\"hidden\" name=\"%s\" value=\"%s\">", html.EscapeString(key), html.EscapeString(value))
		}
	}
	_, _ = fmt.Fprintf(w, "<noscript><button type=\"submit\">Continue to corporate sign-in</button></noscript></form><script nonce=\"%s\">document.getElementById('corporate-login').submit();</script></body></html>", html.EscapeString(nonce))
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func randomUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func scopeContains(raw, scope string) bool {
	for _, value := range strings.Fields(raw) {
		if value == scope {
			return true
		}
	}
	return false
}

func errorsIsEligibilityDenied(err error) bool {
	return err == ErrGroupDenied || err == ErrEmployeeTypeDenied
}

func writeBrokerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeBrokerError(w http.ResponseWriter, status int, code string) {
	writeBrokerJSON(w, status, map[string]string{"error": code})
}
