# Entra ID Authentication Design

**Date:** 2026-04-11
**Status:** Design approved, ready for implementation

## Problem

The Mailgun Mock API is currently usable as a local dev tool, but there is no safe way to deploy a shared instance. The UI surface (`/mock/*`, `/mock/ws`, and the embedded Vue SPA) has no authentication at all, and the Mailgun-compatible API surface (`/v3/*`, `/v4/*`, etc.) defaults to a `full` auth mode that accepts **any** non-empty Basic Auth password. A deployed instance would leak all stored test data to anyone who finds the URL.

The goal: make it safe to deploy this mock to a shared URL, gated by Entra ID so only authorized users in our tenant can read test data, while keeping the Mailgun API surface compatible with existing Mailgun SDK clients (no OAuth for machine clients) and keeping local dev friction-free (no mandatory tenant setup for contributors).

## Non-goals

- Group-based authorization (deferred — the JWT will carry group claims, so this is a small follow-up).
- Protecting sensitive production data. This is a mock; the security model is "control who reaches the UI," not "defend against DB compromise."
- Full automated E2E coverage of the MSAL redirect flow. Manual verification is sufficient for a dev tool.
- Multi-tenant Entra support from day one. Single-tenant is the target; the code shouldn't preclude multi-tenant later.

## Architecture overview

The mock keeps its current single-process shape (Go binary serving the Mailgun-compat API, embedded Vue SPA, and WebSocket), and gains **two independent, opt-in auth layers**:

```
                                 ┌──────────────────────────────┐
                                 │  mailgun-mock binary (Go)    │
                                 │                              │
  Browser (Vue SPA)              │  ┌────────────────────────┐  │
  — MSAL.js ─── Bearer JWT ────► │  │ EntraRequired          │  │
  — WS ?access_token=… ────────► │  │   /mock/* + /mock/ws   │  │
                                 │  └──────────┬─────────────┘  │
                                 │             ▼                │
                                 │  /mock/auth-config (public)  │
                                 │  /mock/api-keys (Entra-gated)│
                                 │  existing /mock/* handlers   │
                                 │                              │
                                 │  ┌────────────────────────┐  │
  Browser → /v3/*, /v4/*         │  │ APIAuth (dual-auth)    │  │
  — Bearer JWT ────────────────► │  │   /v3/*, /v4/*, etc.   │  │
                                 │  │                        │  │
  Test app (Mailgun SDK)         │  │   Bearer? → Entra      │  │
  — Basic api:<key> ───────────► │  │   Basic?  → api_keys   │  │
                                 │  └──────────┬─────────────┘  │
                                 │             ▼                │
                                 │  api_keys table (plaintext)  │
                                 │  existing Mailgun handlers   │
                                 └──────────────────────────────┘
```

### Key principles

1. **Two surfaces, but the auth mechanisms overlap** on the Mailgun-compat surface via dual-auth. The Vue SPA calls `/v3/*` and `/v4/*` extensively (domain picker, routes CRUD, templates, suppressions, etc.), so an Entra-only-on-`/mock/*` split is not enough. The Mailgun-compat middleware accepts **either** a valid Entra JWT (browser) **or** a valid managed API key (SDK clients).
2. **Both layers are opt-in** via `AUTH_MODE`. Local dev runs with `AUTH_MODE=disabled` and nothing changes for contributors. Deployed instances flip it on.
3. **Single source of truth: backend env vars.** The Vue bundle is config-free at build time and fetches its Entra settings from `/mock/auth-config` at startup. One binary, many deployments.
4. **Bootstrap order resolves itself.** Entra-authenticated UI users access the UI freely → mint API keys from the UI → distribute those keys to test apps. No chicken-and-egg.
5. **No new external services.** `github.com/coreos/go-oidc/v3` on the backend, `@azure/msal-browser` in the SPA, Microsoft's public JWKS/OIDC discovery endpoints.

## What the Vue app calls (and why dual-auth is required)

The Vue SPA does **not** only hit `/mock/*`. It uses Mailgun-compat endpoints as the canonical source of truth for domains, routes, templates, mailing lists, and suppressions — instead of duplicating CRUD into a parallel `/mock/*` admin API. Breakdown from `web/src/pages/*.vue`:

**`/mock/*` (control plane):**
- `DashboardPage` → `/mock/dashboard`
- `MessagesPage` → `/mock/messages`, `/mock/messages/{id}`, `/mock/messages/clear`
- `SettingsPage` → `/mock/config`, `/mock/reset/...`
- `WebhooksPage` (deliveries tab) → `/mock/webhooks/deliveries`, `/mock/webhooks/trigger`
- `SimulateInboundPage` → `/mock/inbound/{domain}`
- `TriggerEventsPage` → `/mock/events/{domain}/...`
- `useWebSocket` → `/mock/ws`

**`/v4/*` and `/v3/*` (Mailgun-compat):**
- `/v4/domains` — used as a domain picker by `DomainsPage`, `WebhooksPage`, `SimulateInboundPage`, `EventsPage`, `TemplatesPage`, `TriggerEventsPage`, `SettingsPage`, `SuppressionsPage`.
- `DomainsPage` — full CRUD via `/v4/domains`, `/v3/domains/{name}/tracking`, `/connection`, etc.
- `RoutesPage` — `/v3/routes` full CRUD.
- `WebhooksPage` (registration tab) — `/v3/domains/{name}/webhooks`.
- `EventsPage` — `/v3/{domain}/events`.
- `MailingListsPage` — `/v3/lists/...`.
- `SuppressionsPage` — `/v3/{domain}/bounces|complaints|unsubscribes|whitelists`.
- `TemplatesPage` — `/v3/{domain}/templates/...`.

The dual-auth on `APIAuth` is what lets the SPA keep these call patterns unchanged while still locking down the Mailgun-compat surface against unauthenticated access in deployed mode.

## Configuration model

Backend env vars (read by `internal/config/config.go`):

| Var | Values | Purpose |
|---|---|---|
| `AUTH_MODE` | `disabled` (default) \| `entra` | Master switch for both auth layers |
| `ENTRA_TENANT_ID` | GUID | Tenant directory ID |
| `ENTRA_CLIENT_ID` | GUID | App registration client ID |
| `ENTRA_API_SCOPE` | string, e.g. `access_as_user` | API scope name; expected audience is derived as `api://<client-id>` |
| `ENTRA_REDIRECT_URI` | URL | Public URL of this deployment |

In `disabled` mode, all `ENTRA_*` vars are ignored and everything behaves as it does today. In `entra` mode, any missing `ENTRA_*` var causes the server to fail fast at startup.

**Bootstrap endpoint:** unauthenticated `GET /mock/auth-config` returns the public subset for the SPA:

```json
{
  "enabled": true,
  "tenantId": "...",
  "clientId": "...",
  "scopes": ["api://<client-id>/access_as_user"],
  "redirectUri": "https://mock.example.com"
}
```

When `enabled: false`, the SPA skips MSAL initialization entirely and mounts as it does today.

## Backend changes (Go)

### New package: `internal/auth`

Owns Entra JWT validation. Uses `github.com/coreos/go-oidc/v3` for OIDC discovery and token verification. Single exported `Validator` struct:

```go
package auth

import (
    "context"
    "fmt"
    "github.com/coreos/go-oidc/v3/oidc"
)

type Claims struct {
    OID   string `json:"oid"`
    Email string `json:"preferred_username"`
    Name  string `json:"name"`
}

type Validator struct {
    verifier *oidc.IDTokenVerifier
}

func NewValidator(ctx context.Context, tenantID, expectedAud string) (*Validator, error) {
    issuer := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)
    provider, err := oidc.NewProvider(ctx, issuer)
    if err != nil {
        return nil, err
    }
    return &Validator{
        verifier: provider.Verifier(&oidc.Config{ClientID: expectedAud}),
    }, nil
}

func (v *Validator) Validate(ctx context.Context, raw string) (*Claims, error) {
    tok, err := v.verifier.Verify(ctx, raw)
    if err != nil {
        return nil, err
    }
    var c Claims
    if err := tok.Claims(&c); err != nil {
        return nil, err
    }
    return &c, nil
}
```

`expectedAud` is `"api://" + clientID`, not the bare client ID — Azure access tokens have `aud = api://<client-id>`.

### Entra-specific gotcha: token version

Azure AD issues two access token versions. The v1.0 token has `iss = https://sts.windows.net/<tenant>/`, the v2.0 token has `iss = https://login.microsoftonline.com/<tenant>/v2.0`. go-oidc's discovery returns the v2.0 issuer, so **the app registration manifest must set `accessTokenAcceptedVersion: 2`** or validation will fail with an issuer mismatch.

### `internal/config/config.go` — new fields

```go
type Config struct {
    Port             string
    DatabaseURL      string
    DBDriver         string
    AuthMode         string // "disabled" | "entra"
    EntraTenantID    string
    EntraClientID    string
    EntraAPIScope    string
    EntraRedirectURI string
}
```

Load from env vars with `disabled` default. Add a `Validate()` method that returns an error if `AuthMode == "entra"` and any `ENTRA_*` var is empty. Call from `cmd/server/main.go` at startup.

### `internal/middleware/middleware.go` — three changes

1. **Rename `BasicAuth` → `APIAuth`** and extend with a Bearer path:

    ```go
    func APIAuth(configPtr *mock.MockConfig, v *auth.Validator) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                // Bearer path (dual-auth for the SPA)
                if bearer := extractBearer(r); bearer != "" {
                    if v == nil {
                        response.RespondError(w, http.StatusUnauthorized, "Forbidden")
                        return
                    }
                    if _, err := v.Validate(r.Context(), bearer); err != nil {
                        response.RespondError(w, http.StatusUnauthorized, "Invalid token")
                        return
                    }
                    next.ServeHTTP(w, r)
                    return
                }
                // Basic path (existing behavior, extended to support managed_keys mode)
                // ... existing switch on configPtr.Authentication.AuthMode ...
                // New case "managed_keys": DB lookup in api_keys table
            })
        }
    }
    ```

    A Bearer token that fails validation does **not** fall through to Basic Auth — that's a fail-fast to avoid ambiguous errors.

2. **New `EntraRequired` middleware** for `/mock/*` and `/mock/ws`:

    ```go
    func EntraRequired(v *auth.Validator) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if v == nil { // AUTH_MODE=disabled
                    next.ServeHTTP(w, r)
                    return
                }
                token := extractBearer(r)
                if token == "" {
                    token = r.URL.Query().Get("access_token") // WebSocket path
                }
                if token == "" {
                    response.RespondError(w, http.StatusUnauthorized, "Unauthenticated")
                    return
                }
                if _, err := v.Validate(r.Context(), token); err != nil {
                    response.RespondError(w, http.StatusUnauthorized, "Invalid token")
                    return
                }
                next.ServeHTTP(w, r)
            })
        }
    }
    ```

3. **WebSocket log scrubbing.** A small wrapper around the chi logger for the `/mock/ws` route that strips `?access_token=...` from the logged URL so tokens don't land in access logs.

### `internal/apikey/` — extend

Add `ManagedAPIKey` model (lives alongside the existing Mailgun-compat `APIKey`):

```go
type ManagedAPIKey struct {
    ID        uint   `gorm:"primaryKey"`
    Name      string `gorm:"not null"`
    KeyValue  string `gorm:"uniqueIndex;not null"`
    Prefix    string `gorm:"not null"` // first 8 chars for display
    CreatedAt time.Time
}
```

Key format: `mock_<base64url(32 random bytes)>`, with a `UNIQUE` constraint and a single retry on collision. New file `managed_handlers.go` for UI-facing CRUD:

- `GET /mock/api-keys` — list, returns full plaintext values
- `POST /mock/api-keys` — body `{name}`, server generates key, returns full value
- `DELETE /mock/api-keys/{id}` — hard delete

The existing `/v1/keys` Mailgun-compat handlers stay untouched as a pure mock. Clearly separate from the real key store.

**New auth mode:** `managed_keys` added to the existing `full` / `single` / `accept_any` enum in `mock.MockConfig.Authentication.AuthMode`. When active, Basic Auth validates the password against the `managed_api_keys` table.

### `internal/server/server.go` — wiring

- In `New(db)`: if `cfg.AuthMode == "entra"`, call `auth.NewValidator(ctx, cfg.EntraTenantID, "api://"+cfg.EntraClientID)`. Fail fast on error.
- Add `&apikey.ManagedAPIKey{}` to the existing `db.AutoMigrate(...)` block.
- Wrap all `/mock/*` route registrations with `EntraRequired(validator)` (nil validator == disabled, pass-through).
- Replace `BasicAuth(h.Config())` with `APIAuth(h.Config(), validator)` in every route group that currently uses it.
- Register the new unauthenticated `GET /mock/auth-config` route (not wrapped in `EntraRequired`, since the SPA needs it before signing in).
- Register the new `GET|POST|DELETE /mock/api-keys` routes wrapped in `EntraRequired`.
- Wrap `/mock/ws` with the log-scrubbing middleware + `EntraRequired`.

## Frontend changes (Vue)

### New npm dependency

`@azure/msal-browser` (Microsoft's official SPA library). No community Vue wrapper — our usage is small enough that a thin composable is cleaner.

### New files

- **`web/src/auth/config.ts`** — `fetchAuthConfig(): Promise<AuthConfig>` helper that GETs `/mock/auth-config` once and returns the typed config.
- **`web/src/auth/msalInstance.ts`** — singleton `PublicClientApplication` factory constructed from the fetched config with `cache: { cacheLocation: "localStorage" }` (persistent across tab closes). Exports `msalInstance`, `getActiveAccount()`, `getAccessToken()`, `signIn()`, `signOut()`. `getAccessToken()` wraps `acquireTokenSilent` with an `acquireTokenRedirect` fallback for `InteractionRequiredAuthError`.
- **`web/src/composables/useAuth.ts`** — reactive wrapper exposing `user`, `isAuthenticated`, `signIn`, `signOut` for components.
- **`web/src/pages/ApiKeysPage.vue`** — list/create/delete against `/mock/api-keys`. Same visual structure as existing pages: table, "New Key" modal with a name field, "Created" modal showing the plaintext key with copy-to-clipboard, delete confirmation. Empty-state copy: "No API keys yet. Test apps won't be able to call the Mailgun API surface until you create one."

### Modified files

- **`web/src/main.ts`** — becomes async. Flow:
  1. `const cfg = await fetchAuthConfig()`
  2. If `!cfg.enabled`, mount immediately (current behavior).
  3. Otherwise: init MSAL with `cfg`, call `handleRedirectPromise()` to consume any pending redirect, check for an active account, call `loginRedirect({scopes})` if none, mount once signed in.
- **`web/src/api/client.ts`** — the `request<T>()` method gains a pre-flight step: if auth is enabled, call `getAccessToken()` and set `Authorization: Bearer <jwt>`. Applies uniformly to every URL — `/mock/*` and `/v3/*` alike. No per-page changes needed.
- **`web/src/composables/useWebSocket.ts`** — if auth enabled, append `?access_token=<jwt>` to the WS URL before opening. On close with a 1008/4401 code, fetch a fresh token and reconnect.
- **`web/src/App.vue`** — sidebar header gains a small user block (name + sign-out button) shown only when auth is enabled. New nav link "API Keys" under the Config section.
- **`web/src/router/index.ts`** — register `/api-keys` route. Light navigation guard redirects to `signIn()` if auth is enabled and no active account (belt-and-braces; `main.ts` handles the common case).

**Per-page impact: zero.** Every existing page keeps calling `api.get("/v4/domains")` (etc.) exactly as today. The interceptor attaches the token.

## Data flows

### 1. Local dev (`AUTH_MODE=disabled`) — status quo

```
main.ts → GET /mock/auth-config → {enabled: false}
       → mount app
api.get("/v4/domains") → no Authorization header
APIAuth middleware → "full" mode → passes through
```

### 2. First visit, auth enabled

```
main.ts → GET /mock/auth-config → {enabled: true, ...}
       → msal.initialize()
       → handleRedirectPromise() → no pending redirect
       → getActiveAccount() → null
       → loginRedirect({scopes: ["api://<client-id>/access_as_user"]})
       → (browser → login.microsoftonline.com)
       → user authenticates
       → (browser → ENTRA_REDIRECT_URI with tokens in URL hash)
main.ts runs again → handleRedirectPromise() consumes hash, caches account
       → getActiveAccount() → {username, name, oid}
       → mount app
```

### 3. Subsequent visit (cached account)

```
main.ts → handleRedirectPromise() → nothing
       → getActiveAccount() → found in localStorage
       → mount app immediately
```

### 4. SPA calling `/v4/domains` (the dual-auth path)

```
api.get("/v4/domains")
  → interceptor: msal.acquireTokenSilent({scopes, account}) → JWT
  → fetch("/v4/domains", {headers: {Authorization: "Bearer <jwt>"}})
Go: APIAuth middleware sees Bearer → Validator.Validate(jwt) → ok
  → dh.ListDomains runs
```

### 5. Test app calling `/v3/domain.com/messages.mime`

```
SDK sends: Authorization: Basic api:<managed-key>
APIAuth middleware: no Bearer → falls through to Basic path
  → managed_keys mode: DB lookup in managed_api_keys table
  → found → mh.SendMIMEMessage runs
```

### 6. WebSocket

```
useWebSocket → get token → new WebSocket(`/mock/ws?access_token=${jwt}`)
EntraRequired → extract ?access_token → Validator.Validate → ok
hub.HandleWebSocket runs
```

### 7. Token expiry mid-session

`acquireTokenSilent` refreshes transparently using MSAL's cached refresh token. If that fails, it throws `InteractionRequiredAuthError`, which the interceptor catches and calls `acquireTokenRedirect` → user briefly bounces through Microsoft and lands back on the page they were on.

## Error handling & edge cases

- **Startup validation.** In `entra` mode, `cmd/server/main.go` fails fast on missing `ENTRA_*` vars. In `disabled` mode bound to a non-loopback address, log a big warning: "Auth is disabled and server is listening on a public interface — test data is unprotected."
- **JWKS / discovery unreachable at startup.** `oidc.NewProvider` fails → server exits. Transient JWKS failures during validation return 503 (not 401) so clients can distinguish "bad token" from "our problem."
- **Invalid tokens.** Missing → 401 `unauthenticated`. Malformed/wrong sig/wrong iss/wrong aud/expired → 401 `invalid token`. JWKS fetch failure → 503 `auth provider unavailable`.
- **Clock skew.** go-oidc tolerates small `iat`/`exp` drift internally. No action.
- **No API keys minted yet.** Fresh deployed instance has an empty `managed_api_keys` table. `ApiKeysPage` shows a one-time empty state explaining the consequence. Server logs key count at startup in deployed mode.
- **WebSocket token in query string leaking to logs.** Custom logging middleware for `/mock/ws` strips `?access_token=...` before logging. Unit tested.
- **Key uniqueness.** `mock_<base64url(32 random bytes)>` with `UNIQUE` constraint and one retry on collision.
- **Vite dev proxy.** `web/vite.config.ts` already proxies `/mock` and the Mailgun-compat paths. Local Entra usage with `http://localhost:5173` as redirect URI is documented in `E2E_TESTING.md` but not required for normal dev.
- **Revocation.** DB-backed, no caching layer. `DELETE /mock/api-keys/{id}` takes effect on the next request.

## Testing strategy

### Unit tests (Go)

- **`internal/auth/validator_test.go` (new).** `httptest.Server` serving a fake `.well-known/openid-configuration` + JWKS with a test RSA key. Generate signed JWTs in the test for each case: valid / expired / wrong audience / wrong issuer / unsigned. Assert outcomes.
- **`internal/middleware/middleware_test.go` (extend).**
  - `TestAPIAuth_DualPath` — Bearer path valid/invalid/expired; Basic path valid managed key / unknown key / empty; mixed (invalid Bearer does not fall through).
  - `TestEntraRequired` — header path and `?access_token` path, REST and WS scenarios.
- **`internal/apikey/managed_handlers_test.go` (new).** CRUD coverage for `/mock/api-keys`.
- **`internal/config/config_test.go` (new or extend).** Missing `ENTRA_*` in `entra` mode → error; `disabled` mode with empty Entra vars → ok.

### Existing tests

All run with `AUTH_MODE=disabled` (the zero value). No existing test changes. This is the test for "we didn't break anything."

### E2E (Playwright)

- **`web/e2e/api-keys.spec.ts` (new).** Five specs against a server with auth disabled: create a key, see it in the list, copy-to-clipboard modal shows plaintext, delete, list empty.
- **Full Entra redirect flow** — not automated. Documented as manual verification in `E2E_TESTING.md`.

### Not tested (and why)

- The MSAL SPA flow itself — it's Microsoft's library; mocking the identity provider in a browser is more work than value.
- `/mock/auth-config` with `enabled: true` — covered implicitly by config unit tests.

## Documentation updates

### `README.md` — new "Authentication" section

- Local dev default: auth is off, nothing to configure.
- Enabling Entra ID for deployed instances:
  - Step-by-step app registration: set `accessTokenAcceptedVersion: 2`, configure SPA platform with redirect URI, expose API scope `access_as_user`.
  - Env vars to set: `AUTH_MODE=entra`, `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_API_SCOPE`, `ENTRA_REDIRECT_URI`.
  - How to mint the first API key from the UI after signing in.
  - How test apps use the minted keys (Basic Auth `api:<key>` — unchanged from today).
- Troubleshooting: token version mismatch, missing redirect URI, 401s on test apps when no keys exist.

### `E2E_TESTING.md`

New manual-verification section for the full Entra flow.

## Summary of decisions

1. **Entra ID (MSAL, self-hosted option C)** protects `/mock/*`, `/mock/ws`, and — via dual-auth — `/v3/*`, `/v4/*` etc. when called from the browser.
2. **Managed API keys** stored plaintext in DB, minted from the UI, used by test apps via Basic Auth.
3. **Single Entra app registration** with SPA platform + exposed API scope.
4. **Bootstrap config endpoint** (`/mock/auth-config`) keeps the SPA bundle config-free.
5. **Both auth layers opt-in** via `AUTH_MODE` — zero impact on local dev and existing tests.
6. **`github.com/coreos/go-oidc/v3`** as the sole new Go dependency; **`@azure/msal-browser`** as the sole new npm dependency.
7. **WebSocket token in query string** (option A) with log scrubbing middleware.
8. **No group filtering** for now; deferred.
