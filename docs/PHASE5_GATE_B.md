# Phase 5 Gate B Evidence

Gate B implements corporate SSO, just-in-time local account provisioning, closed registration, and immediate gateway enforcement of identity suspension.

## Delivered controls

- MindCreek-owned OIDC broker with corporate Authorization Code + PKCE `S256`.
- RS256 signature, exact issuer/audience, nonce, expiry, subject, and UserInfo subject validation.
- Stable `issuer + subject` mapping; corporate email changes cannot create a second account.
- Optional corporate group eligibility and first-login personal-workspace provisioning.
- Password login, self-registration, invitation registration, auto-setup, password changes, and new public invitations closed whenever corporate identity is enabled.
- SSO-only web entry with automatic redirect and a retry guard for callback failures.
- Human bearer sessions bound to an active corporate identity; unlinked and suspended sessions fail closed, and refresh requires corporate reauthentication.
- Audited system-administrator suspension/reactivation and a loopback-only break-glass deployment override.
- Local logout followed by corporate logout when the provider publishes an end-session endpoint.

## Automated evidence

The Gate B suite covers valid login, callback-cookie binding, state and authorization-code replay, PKCE propagation, wrong issuer, wrong audience, wrong nonce, expired token, missing subject, stable JIT mapping, suspension, closed routes, migration presence, secret-free configuration, frontend overlay assertions, and a clean upstream submodule.

```sh
make phase5-gate-b
```

The live probe deliberately stops before entering employee credentials:

```sh
make phase5-up
make phase5-gate-b-probe
```

Production activation still requires operator-supplied provider endpoints/client credentials, the exact HTTPS origin, provider-side callback registration, and a manual browser login/suspension exercise. No production credential is stored in Git.
