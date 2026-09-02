# Phase 5 Corporate Identity Provider

## Plain OAuth 2.0 contract

MindCreek Gate B places a product-owned identity broker between WeKnora and the corporate provider. The corporate-facing default is OAuth 2.0 Authorization Code Flow. WeKnora continues to use the broker's private OIDC interface, so no upstream change is required.

The corporate provider exposes POST authorization, POST access-token, GET UserInfo, and GET refresh-token interfaces. MindCreek renders a CSP-restricted `application/x-www-form-urlencoded` browser form containing `client_id`, the external-origin `redirect_uri`, `response_type=code`, `scope=base.profile`, and `display=page`. After the provider returns `/?code=...`, a pre-SPA bridge forwards the code to the gateway. The gateway sends an `application/json` token request containing `client_id`, `client_secret`, the same `redirect_uri`, `grant_type=authorization_code`, and `code`, then calls UserInfo server-to-server with `access_token`, the returned scope (falling back to configured scope), and `client_id` query parameters. Refresh-token configuration is accepted but is not used by the initial login flow.

This provider does not currently return `state` or accept PKCE in the described token exchange. MindCreek therefore binds the one-time login transaction to a Secure, HttpOnly, SameSite callback cookie. This is weaker than provider-returned `state` plus PKCE; request both capabilities from the provider owner when possible. Query-string access tokens can also appear in corporate proxy/access logs, so those systems must redact query values. MindCreek does not include the outbound URL in its errors or audit events.

Register this exact corporate redirect URI:

```text
https://mindcreek.example
```

The provider returns the browser to `https://mindcreek.example/?code=...`. Do not register `/api/v1/mindcreek/oidc/callback`, which is the product-owned internal target of the root bridge, or `/api/v1/auth/oidc/callback`, which is the private broker-to-WeKnora callback.

## UserInfo mapping

The UserInfo response is a flat JSON object with `employeeType`, `globalUserID`, `tenantId`, `uid`, and `uuid`. MindCreek hashes `tenantId + globalUserID` into the stable corporate subject, uses `uid` as both username and display name, uses `uuid` only as a username fallback, and exposes `employeeType` as a prefixed broker group. It generates a stable `@identity.invalid` login alias because the corporate response has no email.

`globalUserID` and `tenantId` must be immutable and non-empty. Set `MINDCREEK_IDENTITY_SUBJECT_TENANT_SCOPED=false` only after the provider owner confirms `globalUserID` is globally unique, permanent, and never recycled. `employeeType` controls login eligibility only; it never grants a MindCreek role. A blank allow-list admits every successfully authenticated employee type.

`MINDCREEK_IDENTITY_USERINFO_DATA_PATH` must remain empty for this flat response. Field values may be JSON strings or numbers; `employeeType` may also be an array or a comma/space-separated string.

## Configuration

Set values only in `.local/mindcreek.env` or an approved secret injector:

```dotenv
MINDCREEK_IDENTITY_ENABLED=true
MINDCREEK_IDENTITY_PROTOCOL=oauth2
MINDCREEK_EXTERNAL_ORIGIN=https://mindcreek.example
MINDCREEK_IDENTITY_PROVIDER_NAME=Corporate account
MINDCREEK_IDENTITY_ISSUER=https://identity.example
MINDCREEK_IDENTITY_AUTHORIZATION_URL=https://identity.example/authorize
MINDCREEK_IDENTITY_TOKEN_URL=https://identity.example/accesstoken
MINDCREEK_IDENTITY_USERINFO_URL=https://identity.example/userinfo
MINDCREEK_IDENTITY_REFRESH_URL=https://identity.example/refreshtoken
MINDCREEK_IDENTITY_CLIENT_ID=mindcreek
MINDCREEK_IDENTITY_CLIENT_SECRET=<corporate-client-secret>
MINDCREEK_IDENTITY_CLIENT_AUTH_METHOD=client_secret_post
MINDCREEK_IDENTITY_AUTHORIZATION_METHOD=POST
MINDCREEK_IDENTITY_AUTHORIZATION_GRANT_TYPE=authorization_code
MINDCREEK_IDENTITY_AUTHORIZATION_DISPLAY=page
MINDCREEK_IDENTITY_TOKEN_REQUEST_FORMAT=json
MINDCREEK_IDENTITY_REDIRECT_URI=https://mindcreek.example
MINDCREEK_IDENTITY_PKCE_ENABLED=false
MINDCREEK_IDENTITY_STATE_REQUIRED=false
MINDCREEK_IDENTITY_USERINFO_TOKEN_TRANSPORT=query
MINDCREEK_IDENTITY_SCOPES=base.profile
MINDCREEK_IDENTITY_USERINFO_DATA_PATH=
MINDCREEK_IDENTITY_SUBJECT_CLAIM=globalUserID
MINDCREEK_IDENTITY_TENANT_CLAIM=tenantId
MINDCREEK_IDENTITY_SUBJECT_TENANT_SCOPED=true
MINDCREEK_IDENTITY_USERNAME_CLAIM=uid
MINDCREEK_IDENTITY_DISPLAY_NAME_CLAIM=uid
MINDCREEK_IDENTITY_UUID_CLAIM=uuid
MINDCREEK_IDENTITY_EMPLOYEE_TYPE_CLAIM=employeeType
MINDCREEK_IDENTITY_ALLOWED_EMPLOYEE_TYPES=
MINDCREEK_BROKER_CLIENT_SECRET=<random-32-plus-character-secret>
```

Use `MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP=true` only for isolated development. Production requires HTTPS and an exact external origin. Keep the environment file mode at `0600`.

The token JSON uses the exact field and value `"grant_type": "authorization_code"`. Authorization remains a browser form POST because an HTML navigation cannot issue a JSON POST while rendering the provider's login response.

## OIDC compatibility mode

An OIDC corporate provider remains supported. Set `MINDCREEK_IDENTITY_PROTOCOL=oidc`, configure the issuer/discovery URL and `openid` scope, and map `sub`, `preferred_username`, `email`, `name`, and optional `groups`. This mode uses redirect-based GET authorization, provider-returned state, PKCE, and RS256 signature, issuer, audience, expiry, nonce, and UserInfo-subject validation.

## Provisioning and session policy

On first successful login, WeKnora creates the local user and personal workspace. Further workspace creation is disabled by default. Corporate access and refresh tokens never reach the browser or WeKnora. MindCreek requires corporate reauthentication after the local access token expires. Suspension or unlinking is enforced at the gateway even for an otherwise unexpired local token.

Normal logout clears the local session. Plain OAuth 2.0 mode then returns to the MindCreek login page because the provider exposes no standardized logout endpoint. Retain the separately protected, loopback-only break-glass administrator described in `PHASE5_OPERATIONS.md`.

## Verification

Run `make phase5-gate-b`, start the configured deployment, and run `make phase5-gate-b-probe`. Then complete one browser login with a test employee, confirm first-login provisioning, test a denied `employeeType` when an allow-list is configured, suspend that identity, and verify its existing session is rejected.
