# Mailgun Mock API

A mock Mailgun service for local development and testing. Accepts real Mailgun API calls, stores data for inspection, and simulates events — without sending real email.

Point your Mailgun client at this service instead of `api.mailgun.net` and everything Just Works, except no emails actually leave your machine.

## Why

- **Local development** — no Mailgun account needed, no accidental sends
- **CI/CD testing** — assert on email content, recipients, and events without network calls
- **Drop-in replacement** — uses the same API shape as real Mailgun, compatible with official SDKs

## Planned Features

| Area | Description |
|------|-------------|
| Messages | Accept messages via API, validate payload, store for inspection |
| Domains | Domain CRUD, controllable verification status, DNS records |
| Events & Logs | Generate realistic events for sent messages, event polling |
| Webhooks | Register webhooks, deliver event payloads, simulate events |
| Suppressions | Bounces, complaints, unsubscribes, allowlist — full CRUD |
| Templates | Template CRUD, versioning, variable rendering |
| Tags | Store tags on messages, return alongside stats |
| Mailing Lists | List and member CRUD, bulk operations |
| Routes | Inbound route management |
| Web UI | Inspect messages, view events, manage suppressions |

See [`implementation_plan/overview.md`](implementation_plan/overview.md) for the full breakdown.

## Development

Commands are run via [`just`](https://github.com/casey/just). Run `just` with no args to list all recipes.

### Testing

| Task | Command |
|---|---|
| Go tests (unit + integration) | `just test` |
| Integration tests only (with optional filter) | `just integration` / `just integration Credentials` |
| Playwright frontend e2e tests | `just test-e2e` |

`just test` runs everything under `./...`, which covers both unit tests in `internal/` and the integration suite in `tests/integration/`. `just test-e2e` builds the SPA, starts the server, and runs Playwright against it.

## Authentication

### Local development (default)

Auth is disabled by default. `just dev` works without any Entra ID setup.

### Enabling Entra ID for deployed instances

1. Create app registration in Azure portal.
2. Add the **SPA platform** and configure redirect URIs:
   - For a deployed instance, add your public URL (must match `ENTRA_REDIRECT_URI`).
   - For local Entra testing, add *both* `http://localhost:5173` (Vite dev server via `just dev-ui`) and `http://localhost:8025` (Go binary direct via `just dev` / `just run`). The SPA is served from a different port depending on which command you use.
3. Under the SPA platform, register a **logout URL** matching the SPA root (same as `ENTRA_REDIRECT_URI` for deployed instances; for local use the Vite and/or Go URLs from step 2).
4. Expose an API with scope `access_as_user` (or whatever you set `ENTRA_API_SCOPE` to — the scope name must match).
5. **Set `accessTokenAcceptedVersion: 2` in the app manifest.** Without this, tokens use the v1 issuer and validation fails.
6. Optional: for future group-based authorization, set `"groupMembershipClaims": "SecurityGroup"` in the app manifest and re-consent. Not used yet, but makes future work easier.
7. Copy Tenant ID and Client ID.

| Variable | Description |
|---|---|
| `AUTH_MODE` | `disabled` (default) or `entra` |
| `ENTRA_TENANT_ID` | Azure AD tenant (directory) ID |
| `ENTRA_CLIENT_ID` | App registration client ID |
| `ENTRA_API_SCOPE` | API scope name, e.g. `access_as_user` |
| `ENTRA_REDIRECT_URI` | Public URL of this deployment |

After deploying, sign in to the UI, navigate to Config → API Keys, and create your first key. Give the key to your test apps — they use it as the Basic Auth password (`api:<key>`), exactly like a real Mailgun key.

### Troubleshooting

**Test apps get 401s** — Check that an API key has been created in the UI. Verify the Basic Auth format is `api:<key>`.

**Issuer mismatch during token validation** — `accessTokenAcceptedVersion` is not set to `2` in the app registration manifest.

**Redirect loop on sign-in** — The redirect URI in the Entra app registration doesn't match `ENTRA_REDIRECT_URI`, or (for local dev) you're hitting a port that isn't in the SPA redirect URI list.

**Token valid but 401 with scope error** — The user's token doesn't carry the required scope. Re-consent, or confirm the token is requested with the right scope.

**503 Service Unavailable on requests** — The server couldn't reach Microsoft's JWKS endpoint. Check egress firewall rules for `login.microsoftonline.com`.

## License

MIT
