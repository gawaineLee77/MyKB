# Phase 5 Gate B Evidence

Gate B implements corporate SSO, just-in-time local account provisioning, closed registration, and immediate gateway enforcement of identity suspension.

## Delivered controls

- MindCreek-owned identity broker with corporate form-POST authorization, callback-cookie binding when provider state is absent, and configurable provider state/PKCE for future upgrades.
- Browser form-POST authorization with `response_type=code` and `display=page`, JSON access-token exchange, and server-side query-parameter UserInfo mapping for the provider's flat `employeeType`, `globalUserID`, `tenantId`, `uid`, and `uuid` object; form token requests, Bearer UserInfo, and `client_secret_basic` remain supported as configurable compatibility modes.
- Stable provider plus tenant/user mapping and generated internal email alias; absent corporate email cannot block first-login provisioning.
- Optional OIDC corporate mode with RS256 signature, exact issuer/audience, nonce, expiry, subject, and UserInfo subject validation.
- Optional employee-type/group eligibility and first-login personal-workspace provisioning.
- Password login, self-registration, invitation registration, auto-setup, password changes, and new public invitations closed whenever corporate identity is enabled.
- SSO-only web entry with automatic redirect, exact internal-to-public broker URL mapping, root `?code=...` callback forwarding, and a retry guard for callback failures.
- Human bearer sessions bound to an active corporate identity; unlinked and suspended sessions fail closed, and refresh requires corporate reauthentication.
- Audited system-administrator suspension/reactivation and a loopback-only break-glass deployment override.
- Local logout followed by corporate logout when the provider publishes an end-session endpoint.

## Automated evidence

The Gate B suite covers form-POST authorization, callbacks with and without provider state, query-token and Bearer UserInfo, OAuth token mapping, both client authentication methods, nested UserInfo, employee-type denial, callback-cookie binding, authorization-code replay, PKCE propagation, the OIDC token-security matrix, missing stable identity, JIT mapping, suspension, closed routes, migration presence, secret-free configuration, frontend assertions, and a clean upstream submodule.

```sh
make phase5-gate-b
```

The live probe deliberately stops before entering employee credentials:

```sh
make phase5-up
make phase5-gate-b-probe
```

Production activation still requires operator-supplied provider endpoints/client credentials, the exact HTTPS origin, provider-side callback registration, and a manual browser login/suspension exercise. No production credential is stored in Git.
