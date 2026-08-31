# Phase 5 Corporate Identity Provider

## Contract

MindCreek Gate B uses a product-owned OIDC broker between WeKnora and the corporate identity provider. The corporate provider must support Authorization Code Flow, PKCE `S256`, OIDC discovery, RS256 ID tokens, and stable `iss` and `sub` claims. The required scopes default to `openid profile email`; configure the provider to return the selected username, email, and optional group claim.

Register this exact corporate callback URI:

```text
https://mindcreek.example/api/v1/mindcreek/oidc/callback
```

Do not register `/api/v1/auth/oidc/callback` with the corporate provider. That is the private broker-to-WeKnora callback.

## Provisioning and authorization

The broker validates signature, issuer, audience, expiry, nonce, user-info subject, and the optional any-of group allow-list before provisioning. It maps the immutable `issuer + subject` pair to a pairwise broker subject and a stable internal `@identity.invalid` email. Corporate email changes therefore do not create another MindCreek account. The real corporate email and current groups remain in the product-owned identity record.

On first successful login, WeKnora creates the local user and personal workspace. Further workspace creation is disabled by default. Optional `MINDCREEK_IDENTITY_REQUIRED_GROUPS` controls organization eligibility; a blank value admits every active identity from the configured issuer.

## Configuration

Set these values only in `.local/mindcreek.env` or an approved secret injector:

```dotenv
MINDCREEK_IDENTITY_ENABLED=true
MINDCREEK_EXTERNAL_ORIGIN=https://mindcreek.example
MINDCREEK_IDENTITY_ISSUER=https://identity.example
MINDCREEK_IDENTITY_CLIENT_ID=mindcreek
MINDCREEK_IDENTITY_CLIENT_SECRET=<corporate-client-secret>
MINDCREEK_BROKER_CLIENT_SECRET=<random-32-plus-character-secret>
MINDCREEK_IDENTITY_REQUIRED_GROUPS=knowledge-users
```

Use `MINDCREEK_IDENTITY_ALLOW_INSECURE_HTTP=true` only for isolated development. Production requires HTTPS and an exact external origin.

## Sessions, suspension, and logout

WeKnora issues the business access token; corporate tokens never reach the browser or WeKnora. Although the upstream callback also returns its normal refresh token for compatibility, Gate B rejects refresh at the gateway and requires corporate reauthentication after access-token expiry. The gateway binds the first local principal to the corporate record and checks every human bearer session. A suspended or unlinked identity is rejected even if a local token has not expired. A system administrator can call:

```text
POST /api/v1/mindcreek/identities/{pairwise-subject}/suspend
POST /api/v1/mindcreek/identities/{pairwise-subject}/activate
```

Both changes are audited. Normal logout revokes the local session and continues to the provider logout endpoint when advertised.

## Break-glass procedure

Maintain one local WeKnora system administrator whose password is stored outside the repository. Put its local user ID in `MINDCREEK_BREAK_GLASS_USER_IDS`. During an identity-provider outage, an authorized server operator may temporarily apply `deploy/phase5/compose.break-glass.yml`, which publishes the upstream app on host loopback only. Obtain a local token, perform the emergency action, then immediately remove the override and review the upstream and MindCreek audit records. Never expose the break-glass port to the LAN.

## Verification

Run `make phase5-gate-b` for the synthetic security suite. After configuring the real provider and starting Phase 5, run `make phase5-gate-b-probe`. Complete one browser login with a test employee, confirm first-login provisioning, suspend that identity, and verify its existing session is denied.
