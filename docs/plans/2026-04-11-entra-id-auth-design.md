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

---

# Implementation Plan

Ordered, discrete tasks. Each task ends with a verification step. Backend tasks come first (Tasks 1–11) so the SPA has a working `/mock/auth-config` endpoint to develop against; frontend tasks (Tasks 12–20) follow; docs and E2E close out (Tasks 21–23).

### Task 1: Extend `config.Config` with Entra fields

**Files to modify:** `internal/config/config.go`
**Files to create:** `internal/config/config_test.go`

**Pattern reference:** `internal/config/config.go` — already uses a simple `getEnv(key, fallback)` helper. Extend the existing style; don't introduce a new config library.

**Details:**
- Add fields to `Config`: `AuthMode` (default `"disabled"`), `EntraTenantID`, `EntraClientID`, `EntraAPIScope`, `EntraRedirectURI`.
- Extend `Load()` to read `AUTH_MODE`, `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_API_SCOPE`, `ENTRA_REDIRECT_URI` via `getEnv`.
- Add a new method `(c *Config) Validate() error` that returns a descriptive error if `AuthMode == "entra"` and any `Entra*` field is empty. In `disabled` mode, always returns nil.
- Tests cover: disabled mode ignores Entra vars; entra mode with all vars set returns nil; entra mode missing any one var returns a specific error naming the missing var.

**Checklist:**
- [ ] Add new `Config` fields
- [ ] Extend `Load()` with new env vars
- [ ] Add `Validate()` method
- [ ] Write `config_test.go` with disabled-mode, full-entra, and missing-var cases
- [ ] `go test ./internal/config/...` passes

---

### Task 2: Create `internal/auth` package with `Validator`

**Files to create:** `internal/auth/validator.go`, `internal/auth/validator_test.go`
**Files to modify:** `go.mod`, `go.sum`

**Pattern reference:** None in the repo — this is a new package. For test style, mirror `internal/middleware/middleware_test.go` (helpers + subtests, `httptest.NewServer` for fakes).

**Details:**
- Add dependency: `go get github.com/coreos/go-oidc/v3@latest`.
- Define `Claims` struct with `OID`, `Email` (from `preferred_username`), `Name`.
- Define `Validator` struct wrapping `*oidc.IDTokenVerifier`.
- `NewValidator(ctx context.Context, tenantID, expectedAud string) (*Validator, error)` constructs issuer URL (`https://login.microsoftonline.com/<tenant>/v2.0`), calls `oidc.NewProvider`, and builds a verifier with `ClientID: expectedAud`.
- `Validate(ctx, raw string) (*Claims, error)` calls `verifier.Verify` and extracts claims.
- Tests: run an `httptest.NewServer` that serves a fake `.well-known/openid-configuration` and JWKS using an RSA test key (`crypto/rsa` + `jwt.SigningMethodRS256`). Use `github.com/golang-jwt/jwt/v5` (transitive via go-oidc) to **sign test tokens**. Cases: valid token → claims extracted; expired token → error; wrong audience → error; wrong issuer → error; malformed → error.
- **Note on gotcha:** test tokens should use issuer `<httptest-server-url>/v2.0` to match what `NewValidator` builds — parameterize via an internal `newValidatorForIssuer` helper the test can call.

**Checklist:**
- [ ] `go get github.com/coreos/go-oidc/v3@latest`
- [ ] Write `validator.go` with `Claims`, `Validator`, `NewValidator`, `Validate`
- [ ] Expose an unexported `newValidatorForIssuer(ctx, issuerURL, aud)` used by tests only
- [ ] Write `validator_test.go` with the five cases above
- [ ] `go test ./internal/auth/...` passes
- [ ] `go build ./...` still compiles

---

### Task 3: Add startup validation + context in `cmd/server/main.go`

**Files to modify:** `cmd/server/main.go`

**Pattern reference:** Current `main.go` is 27 lines — just extend it.

**Details:**
- Create `ctx, cancel := context.WithCancel(context.Background()); defer cancel()`.
- After `config.Load()`, call `cfg.Validate()` and `log.Fatalf` on error.
- If `cfg.AuthMode == "disabled"` and the listen address is non-loopback, log a warning (simple heuristic: `cfg.Port` is set and env var `BIND_ADDR` is non-empty and not `127.0.0.1`/`localhost`; or just always log the warning in disabled mode as a soft nudge — simpler, take this path).
- Change the call to `server.New(ctx, db, cfg)` (signature change covered in Task 11).
- No tests needed for `main.go` directly; behavior is covered by `config_test.go` + `server_test.go`.

**Checklist:**
- [ ] Import `context`
- [ ] Create and cancel top-level context
- [ ] Call `cfg.Validate()` with fatal on error
- [ ] Log "auth disabled" warning in disabled mode
- [ ] Update call site for `server.New` (compiles against Task 11's new signature; do this task AFTER Task 11 or use a temporary `_ = ctx` to keep things compiling in between)
- [ ] `just build` succeeds

---

### Task 4: Add `ManagedAPIKey` model + migration

**Files to create:** `internal/apikey/managed.go`
**Files to modify:** `internal/server/server.go`

**Pattern reference:** `internal/apikey/apikey.go:17-30` — model uses `database.BaseModel` (string UUID ID + timestamps + soft delete). Copy this style.

**Details:**
- New file `internal/apikey/managed.go` in the existing `apikey` package (keeps managed keys and Mailgun-compat keys co-located but clearly distinct by file).
- Model:
  ```go
  type ManagedAPIKey struct {
      database.BaseModel
      Name     string `gorm:"not null" json:"name"`
      KeyValue string `gorm:"uniqueIndex;not null" json:"key_value"`
      Prefix   string `gorm:"not null" json:"prefix"`
  }
  ```
- Helper `generateManagedKeyValue() (value, prefix string, err error)` → `mock_<base64url(32 random bytes)>`, prefix is `value[:13]` (`"mock_" + 8 chars`).
- Register in the `db.AutoMigrate(...)` call at `internal/server/server.go:49` by adding `&apikey.ManagedAPIKey{}` to the existing list.

**Checklist:**
- [ ] Create `internal/apikey/managed.go` with the model + key-generator helper
- [ ] Add `&apikey.ManagedAPIKey{}` to `AutoMigrate` call in `server.go`
- [ ] `go build ./...` succeeds
- [ ] Run server locally with `just dev` — confirm new table is created (log check or sqlite inspection)

---

### Task 5: Implement managed API key CRUD handlers

**Files to create:** `internal/apikey/managed_handlers.go`, `internal/apikey/managed_handlers_test.go`

**Pattern reference:** `internal/apikey/apikey.go:117-298` — `Handlers` struct + `NewHandlers(db)` + method handlers using `response.RespondJSON` / `response.RespondError`.

**Details:**
- `ManagedHandlers` struct holding `db *gorm.DB`, constructor `NewManagedHandlers(db) *ManagedHandlers`.
- Three handler methods:
  - `List(w, r)` — `db.Order("created_at DESC").Find(&keys)`, return as JSON array of `{id, name, key_value, prefix, created_at}`.
  - `Create(w, r)` — decode `{name string}` via `request.DecodeJSON`, validate non-empty, call `generateManagedKeyValue()`, insert (one retry on `UNIQUE` violation), return the full object with 201.
  - `Delete(w, r)` — read `{id}` from chi URL param, `db.Unscoped().Delete(&ManagedAPIKey{}, "id = ?", id)`, 204.
- Tests (copy structure from `internal/middleware/middleware_test.go`): in-memory SQLite, migrate `ManagedAPIKey`, httptest server, cases: empty list → `[]`; create → returns value with `mock_` prefix; create with empty name → 400; list after create → 1 item; delete by id → 204; list after delete → `[]`.
- **Not** wired into routes yet — that's Task 11.

**Checklist:**
- [ ] Create `managed_handlers.go` with `ManagedHandlers`, `NewManagedHandlers`, `List`, `Create`, `Delete`
- [ ] Write `managed_handlers_test.go` with the 6 cases above
- [ ] `go test ./internal/apikey/...` passes

---

### Task 6: Add `managed_keys` mode to Basic-path auth

**Files to modify:** `internal/middleware/middleware.go`, `internal/middleware/auth_test.go`

**Pattern reference:** Existing `BasicAuth` switch at `internal/middleware/middleware.go:85-118`. Existing tests in `auth_test.go`.

**Details:**
- Add a `case "managed_keys":` branch to the switch. Extract `username, password, ok := r.BasicAuth()`; on mismatch of format → 401; otherwise `db.Where("key_value = ?", password).First(&apikey.ManagedAPIKey{})`; not-found → 401; found → pass through.
- This means the middleware now needs access to `*gorm.DB`. Update `BasicAuth` signature to `BasicAuth(cfg *mock.MockConfig, db *gorm.DB)` (still no Entra yet — that's Task 7).
- Update all call sites in `internal/server/server.go` to pass `db` (mechanical change).
- Extend `auth_test.go`: `TestBasicAuth_ManagedKeys_Valid`, `TestBasicAuth_ManagedKeys_Invalid`, `TestBasicAuth_ManagedKeys_EmptyTable`.

**Checklist:**
- [ ] Add `managed_keys` case to `BasicAuth` switch
- [ ] Change `BasicAuth` signature to accept `*gorm.DB`
- [ ] Update all call sites in `server.go` (mechanical)
- [ ] Extend `auth_test.go` with 3 new cases
- [ ] `go test ./internal/middleware/...` passes
- [ ] `go build ./...` passes (catches missed call sites)

---

### Task 7: Rename `BasicAuth` → `APIAuth` with dual-auth Bearer path

**Files to modify:** `internal/middleware/middleware.go`, `internal/middleware/auth_test.go`, `internal/server/server.go`

**Pattern reference:** The existing `BasicAuth` function this extends, plus the new `auth.Validator` from Task 2.

**Details:**
- Rename `BasicAuth` → `APIAuth`. Update all call sites in `server.go`.
- Extend signature: `APIAuth(cfg *mock.MockConfig, db *gorm.DB, v *auth.Validator) func(http.Handler) http.Handler`. A `nil` validator means Entra is disabled — skip Bearer path entirely.
- At the top of the handler body, **before** the existing Basic switch: check for `Authorization: Bearer <token>`. If present and `v != nil`, call `v.Validate(r.Context(), token)`; on success, pass through; on failure, **return 401 directly — do NOT fall through to Basic** (avoids ambiguous error cases).
- Helper function `extractBearer(r *http.Request) string` (exported lowercase since used by Task 8 too — put in the same file).
- New tests: `TestAPIAuth_Bearer_ValidToken`, `TestAPIAuth_Bearer_InvalidToken_NoFallthrough`, `TestAPIAuth_Bearer_DisabledValidator_FallthroughToBasic`. Use the `newValidatorForIssuer` helper from Task 2 + a fake JWKS httptest server.

**Checklist:**
- [ ] Rename `BasicAuth` → `APIAuth` with new signature
- [ ] Implement Bearer path with fail-fast on invalid token
- [ ] Add `extractBearer` helper
- [ ] Update all call sites in `server.go`
- [ ] Add 3 new tests
- [ ] `go test ./internal/middleware/...` passes
- [ ] `go build ./...` passes

---

### Task 8: Create `EntraRequired` middleware (REST + WS variants)

**Files to modify:** `internal/middleware/middleware.go`, `internal/middleware/auth_test.go`

**Pattern reference:** The newly-renamed `APIAuth` from Task 7 — same validator-or-nil pattern.

**Details:**
- `EntraRequired(v *auth.Validator) func(http.Handler) http.Handler` — one function, handles both REST and WS via dual token extraction.
- Logic: if `v == nil`, pass through (disabled mode). Otherwise extract token from `Authorization: Bearer <token>` OR from `?access_token=<token>` query param. No token → 401. Validate → 401 on failure, pass through on success.
- Tests: `TestEntraRequired_DisabledPassthrough`, `TestEntraRequired_Header_Valid`, `TestEntraRequired_Header_Invalid`, `TestEntraRequired_QueryParam_Valid` (WS simulation), `TestEntraRequired_NoToken_401`.

**Checklist:**
- [ ] Implement `EntraRequired` with header + query-param extraction
- [ ] Add 5 new tests
- [ ] `go test ./internal/middleware/...` passes

---

### Task 9: Create WebSocket log-scrubbing middleware

**Files to create:** `internal/middleware/ws_logging.go`, `internal/middleware/ws_logging_test.go`

**Pattern reference:** chi's `middleware.Logger` — we're wrapping it, not replacing it.

**Details:**
- `WSLogScrubber() func(http.Handler) http.Handler` — wraps the request so that by the time `middleware.Logger` sees it, the `?access_token=...` query param is replaced with `?access_token=REDACTED`.
- Implementation: clone `r.URL.Query()`, if `access_token` exists replace with `"REDACTED"`, build a new URL, shallow-clone the request with the scrubbed URL, and pass that to `next.ServeHTTP`. **Only touches what the logger reads; the real request handlers still see the original query params via `r.URL.RawQuery`.**
- Actually simpler: since chi's logger reads `r.URL.String()`, just mutate a copy. Use `*r` to clone the Request, set `.URL = cloned` with the scrubbed query, call `next.ServeHTTP(w, &scrubbed)`. **BUT** the downstream `EntraRequired` middleware needs the real token. Solution: reverse the ordering — `WSLogScrubber` runs AFTER `EntraRequired` (which validates the real token), THEN scrubs before the logger middleware logs. chi's middleware ordering handles this if we register the scrubber between the logger and `EntraRequired`.
- **Cleaner alternative:** make `WSLogScrubber` a per-route wrapper that, before calling `next`, logs manually with scrubbed URL and skips chi's built-in logger for `/mock/ws`. Simpler than middleware reordering.
- Tests: `TestWSLogScrubber_RedactsAccessToken`, `TestWSLogScrubber_LeavesOtherParams`, `TestWSLogScrubber_NoQueryParams`.

**Checklist:**
- [ ] Decide on implementation approach (per-route manual logging is simpler — prefer it)
- [ ] Implement `WSLogScrubber`
- [ ] Add 3 tests
- [ ] `go test ./internal/middleware/...` passes

---

### Task 10: Add `/mock/auth-config` endpoint

**Files to modify:** `internal/mock/handlers.go` (or create `internal/mock/auth_config.go`)

**Pattern reference:** Existing `GetConfig` handler in `internal/mock/handlers.go` — returns JSON from the in-memory mock config. Mirror the style.

**Details:**
- Add handler method on `*Handlers`: `GetAuthConfig(w http.ResponseWriter, r *http.Request)`.
- Handler needs access to `*config.Config` (the Go-level config, not the mock config). Thread this through `mock.NewHandlers(db, cfg)` — signature change, update call site in `server.go`.
- Response shape (when enabled):
  ```json
  {
    "enabled": true,
    "tenantId": "...",
    "clientId": "...",
    "scopes": ["api://<client-id>/access_as_user"],
    "redirectUri": "..."
  }
  ```
- When disabled: `{"enabled": false}`.
- **No** secrets returned — this endpoint is unauthenticated. Only public OIDC config.
- Test: `TestGetAuthConfig_Disabled`, `TestGetAuthConfig_Enabled` (write a test file if one doesn't exist for `mock`).

**Checklist:**
- [ ] Thread `*config.Config` into `mock.Handlers` (signature change)
- [ ] Add `GetAuthConfig` handler
- [ ] Add tests
- [ ] `go test ./internal/mock/...` passes
- [ ] `go build ./...` passes

---

### Task 11: Wire auth into `server.New()`

**Files to modify:** `internal/server/server.go`, `cmd/server/main.go`

**Pattern reference:** Existing route-wrapping style in `server.go:118-442`.

**Details:**
- Change signature: `func New(ctx context.Context, db *gorm.DB, cfg *config.Config) http.Handler`.
- At the top of `New`, if `cfg.AuthMode == "entra"`, construct `validator, err := auth.NewValidator(ctx, cfg.EntraTenantID, "api://"+cfg.EntraClientID)`; `log.Fatalf` on error. Otherwise `validator = nil`.
- Update `mock.NewHandlers` call to pass `cfg` (from Task 10).
- Replace every `appMiddleware.BasicAuth(h.Config())` call with `appMiddleware.APIAuth(h.Config(), db, validator)`. (There are ~30 of these — use a careful find/replace.)
- Wrap all `/mock/*` routes with `EntraRequired(validator)`. The `/mock` route group starts at line 412 — add `r.Use(appMiddleware.EntraRequired(validator))` at the top of the `r.Route("/mock", ...)` block. **Exception:** `/mock/auth-config` and `/mock/health` must NOT be wrapped. Place these two routes OUTSIDE the `r.Route("/mock", ...)` block, or use a nested `r.Group` that does not inherit `EntraRequired`.
- Register the new `/mock/auth-config` route (unauthenticated): `r.Get("/mock/auth-config", h.GetAuthConfig)`.
- Register the new `/mock/api-keys` routes inside the Entra-protected `/mock` group: `r.Get("/api-keys", mkh.List); r.Post("/api-keys", mkh.Create); r.Delete("/api-keys/{id}", mkh.Delete)`.
- Apply `WSLogScrubber` + `EntraRequired` to the `/mock/ws` route specifically.
- Update `cmd/server/main.go` to call `server.New(ctx, db, cfg)`.

**Checklist:**
- [ ] Change `server.New` signature
- [ ] Construct validator at startup in entra mode
- [ ] Replace all `BasicAuth` call sites with `APIAuth`
- [ ] Wrap `/mock/*` group with `EntraRequired`
- [ ] Place `/mock/auth-config` and `/mock/health` outside the Entra-protected group
- [ ] Wire `/mock/api-keys` CRUD routes
- [ ] Apply WS scrubber to `/mock/ws`
- [ ] Update `main.go` call site
- [ ] `just build` succeeds
- [ ] `go test ./...` passes (all existing tests must still pass with `AUTH_MODE=disabled` default)
- [ ] Manual smoke: `just dev`, `curl localhost:8025/mock/auth-config` → `{"enabled":false}`

---

### Task 12: Add `@azure/msal-browser` dependency + `auth/config.ts`

**Files to modify:** `web/package.json`, `web/package-lock.json`
**Files to create:** `web/src/auth/config.ts`

**Pattern reference:** `web/src/api/client.ts` for fetch-based helper style.

**Details:**
- `cd web && npm install @azure/msal-browser`.
- Create `web/src/auth/config.ts`:
  ```ts
  export interface AuthConfig {
    enabled: boolean;
    tenantId?: string;
    clientId?: string;
    scopes?: string[];
    redirectUri?: string;
  }
  export async function fetchAuthConfig(): Promise<AuthConfig> {
    const res = await fetch("/mock/auth-config", { headers: { Accept: "application/json" } });
    if (!res.ok) throw new Error(`auth-config fetch failed: ${res.status}`);
    return res.json();
  }
  ```
- No tests — this is a trivial fetch wrapper; covered end-to-end by Task 23.

**Checklist:**
- [ ] `npm install @azure/msal-browser` in `web/`
- [ ] Create `web/src/auth/config.ts`
- [ ] `npm run lint` in `web/` passes
- [ ] `npm run build` in `web/` succeeds

---

### Task 13: Create MSAL singleton wrapper `auth/msalInstance.ts`

**Files to create:** `web/src/auth/msalInstance.ts`

**Pattern reference:** `web/src/api/client.ts` — singleton export pattern.

**Details:**
- Export `initMsal(config: AuthConfig): Promise<PublicClientApplication | null>` — returns `null` if `config.enabled === false`.
- Export `msalInstance: PublicClientApplication | null` (module-level, set by `initMsal`).
- Export `getActiveAccount()`, `getAccessToken(): Promise<string | null>`, `signIn()`, `signOut()`.
- `getAccessToken()` wraps `acquireTokenSilent({scopes, account: getActiveAccount()})` with a `.catch` for `InteractionRequiredAuthError` → `acquireTokenRedirect(...)`.
- MSAL config uses `cacheLocation: "localStorage"`.
- No unit tests — MSAL is Microsoft's library; we trust it. Integration verification in Task 15's manual smoke.

**Checklist:**
- [ ] Create `msalInstance.ts` with the functions above
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds

---

### Task 14: Create `useAuth` composable

**Files to create:** `web/src/composables/useAuth.ts`

**Pattern reference:** `web/src/composables/useWebSocket.ts` — reactive composable structure.

**Details:**
- Expose reactive `user` (name/email/oid), `isAuthenticated`, and functions `signIn`, `signOut`.
- Initial values derived from `getActiveAccount()`. Listen to MSAL's `addEventCallback` for `LOGIN_SUCCESS` / `LOGOUT_SUCCESS` to keep `user` in sync.
- Gracefully handles `msalInstance === null` (disabled mode): returns static `{ isAuthenticated: computed(() => false), user: computed(() => null), signIn: noop, signOut: noop }`.

**Checklist:**
- [ ] Create `useAuth.ts`
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds

---

### Task 15: Refactor `main.ts` to async bootstrap

**Files to modify:** `web/src/main.ts`

**Pattern reference:** Current `main.ts` is 5 lines.

**Details:**
- Wrap current mount logic in an async `bootstrap()` function.
- Flow: `const cfg = await fetchAuthConfig();` → if `!cfg.enabled`, mount immediately → else `await initMsal(cfg); await msalInstance.handleRedirectPromise();` → if no active account, `await signIn()` (redirect, won't return) → mount.
- Call `bootstrap().catch(err => { console.error(err); document.body.innerText = "Failed to load app: " + err.message; })`.
- **Manual smoke at end of this task:** run `just dev` with `AUTH_MODE=disabled`, confirm the UI loads exactly as before. (Entra-on smoke waits for Task 21 doc task since it requires env vars + app registration.)

**Checklist:**
- [ ] Refactor `main.ts` to async bootstrap
- [ ] `npm run build` succeeds
- [ ] Manual smoke: `just dev` (auth disabled) — UI loads and dashboard renders

---

### Task 16: Add auth interceptor to `api/client.ts`

**Files to modify:** `web/src/api/client.ts`

**Pattern reference:** The current `request<T>` method at `web/src/api/client.ts:13-51`.

**Details:**
- Import `getAccessToken` from `@/auth/msalInstance`.
- In `request<T>()`, after the existing header assembly (line 24) and before `fetch()` (line 26), add: `const token = await getAccessToken(); if (token) headers["Authorization"] = `Bearer ${token}`;`.
- Because `getAccessToken` returns `null` in disabled mode, auth-off requests send no `Authorization` header and existing Basic-auth behavior on the backend (`full` mode) is preserved.
- No new tests — covered end-to-end by Task 23 and the existing Playwright suite (run in auth-off mode).

**Checklist:**
- [ ] Add pre-flight token fetch to `request<T>()`
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds
- [ ] Manual smoke: `just dev`, click through several pages, confirm API calls still succeed (network tab shows no Authorization header in disabled mode)

---

### Task 17: Thread token into WebSocket URL

**Files to modify:** `web/src/composables/useWebSocket.ts`

**Pattern reference:** Current `getWSUrl` helper in the same file.

**Details:**
- Make `getWSUrl()` async: await `getAccessToken()`; if token present, append `?access_token=${encodeURIComponent(token)}`. Otherwise return unchanged.
- Update the call site that constructs `new WebSocket(url)` to `await getWSUrl()` first.
- Reconnection logic re-runs `getWSUrl()` (which re-acquires a fresh token) each time — no additional changes needed for token refresh on long-lived connections.

**Checklist:**
- [ ] Make `getWSUrl` async
- [ ] Await it at connect and reconnect sites
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds
- [ ] Manual smoke: `just dev`, open browser, confirm "Connected" indicator (WS still works in disabled mode)

---

### Task 18: Add sign-in / user display / sign-out to `App.vue`

**Files to modify:** `web/src/App.vue`

**Pattern reference:** Current sidebar header at `App.vue:11-22`.

**Details:**
- Import `useAuth`. Get `user`, `isAuthenticated`, `signOut`.
- In the `sidebar-header` block, under the connection status div, add `v-if="isAuthenticated"` block with `{{ user?.name }}` and a "Sign out" button (`@click="signOut"`). When disabled mode, `isAuthenticated` is false and this block never renders.
- Styling: match existing sidebar typography; small text, muted color. Keep it minimal.

**Checklist:**
- [ ] Import and use `useAuth`
- [ ] Add conditional user block in sidebar header
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds

---

### Task 19: Create `ApiKeysPage.vue`

**Files to create:** `web/src/pages/ApiKeysPage.vue`

**Pattern reference:** `web/src/pages/MailingListsPage.vue` (CRUD page structure), plus the "detail panel with close" pattern at lines 387-487 for the "Created key" display.

**Details:**
- Top-level `<script setup lang="ts">` with refs: `keys`, `loading`, `error`, `creating`, `newKeyName`, `justCreatedKey` (the plaintext value shown once after creation — stays visible until dismissed).
- Functions: `fetchKeys()` → `api.get<ManagedKey[]>("/mock/api-keys")`, `createKey()` → `api.post("/mock/api-keys", {name})` + set `justCreatedKey`, `deleteKey(id)` → `window.confirm(...)` + `api.del` + refresh.
- Template: h1 "API Keys" + description paragraph explaining what they're for (test apps calling `/v3/*` etc.). Empty state: "No API keys yet. Test apps won't be able to call the Mailgun API surface until you create one." Input field + "Create Key" button. Table of existing keys with columns `Name`, `Prefix`, `Created`, `Actions` (delete button). "Just created" panel shows the full plaintext key with a copy-to-clipboard button — dismissible.
- Keep styling consistent with other pages; copy class names from `MailingListsPage.vue`.

**Checklist:**
- [ ] Create `ApiKeysPage.vue` with script setup + template + styles
- [ ] Empty state copy matches design spec
- [ ] "Just created" panel with copy-to-clipboard
- [ ] Delete confirmation via `window.confirm`
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds

---

### Task 20: Register `/api-keys` route + nav link

**Files to modify:** `web/src/router/index.ts`, `web/src/App.vue`

**Pattern reference:** Existing route declarations in `router/index.ts` and nav links in `App.vue:95-103`.

**Details:**
- Import `ApiKeysPage` in `router/index.ts`, add `{ path: "/api-keys", name: "ApiKeys", component: ApiKeysPage }`.
- In `App.vue`, under the "Config" section header (after Settings), add `<router-link to="/api-keys" class="nav-item">API Keys</router-link>`.
- No router guards — we intentionally scoped out the belt-and-braces guard since `main.ts` already handles the redirect flow (YAGNI).

**Checklist:**
- [ ] Register route
- [ ] Add nav link
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds
- [ ] Manual smoke: `just dev`, click "API Keys" in sidebar, page loads with empty state

---

### Task 21: Update `README.md` with Authentication section

**Files to modify:** `README.md`

**Pattern reference:** Existing README structure (sections with level-2 headings).

**Details:**
- New top-level section "Authentication" (before or after "Commands"). Subsections:
  - **Local development (default).** "Auth is disabled by default. `just dev` works without any Entra ID setup."
  - **Enabling Entra ID for deployed instances.**
    - Numbered list for the Entra app registration: create app registration in Azure portal → add SPA platform with redirect URI matching `ENTRA_REDIRECT_URI` → expose an API with scope `access_as_user` → **set `accessTokenAcceptedVersion: 2` in the app manifest** (called out prominently) → copy Tenant ID and Client ID.
    - Env vars table: `AUTH_MODE`, `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_API_SCOPE`, `ENTRA_REDIRECT_URI` with descriptions.
    - First-run: "After deploying, sign in to the UI, navigate to Config → API Keys, create your first key. Give the key to your test apps — they use it as the Basic Auth password (`api:<key>`), exactly like a real Mailgun key."
  - **Troubleshooting.** Three subsections:
    - "Test apps get 401s" → check API key is created, check Basic Auth format.
    - "Issuer mismatch during token validation" → `accessTokenAcceptedVersion` is not set to 2.
    - "Redirect loop on sign-in" → redirect URI in Entra app registration doesn't match `ENTRA_REDIRECT_URI`.

**Checklist:**
- [ ] Write Authentication section in README
- [ ] Include app registration steps
- [ ] Include env var table
- [ ] Include first-API-key walkthrough
- [ ] Include three troubleshooting items
- [ ] Verify Markdown renders correctly (visual check in editor preview)

---

### Task 22: Update `E2E_TESTING.md` with manual Entra verification

**Files to modify:** `E2E_TESTING.md`

**Pattern reference:** Current file structure.

**Details:**
- New section "Manual verification: Entra ID flow" at the end.
- Steps: set env vars, start server, open browser to redirect URI, confirm redirect to `login.microsoftonline.com`, sign in, confirm redirect back, confirm dashboard renders, open network tab and confirm requests carry `Authorization: Bearer`, navigate to API Keys, create a key, copy the value, run `curl -u api:<key> http://localhost:8025/v4/domains` and confirm 200.

**Checklist:**
- [ ] Add manual-verification section
- [ ] List the numbered verification steps
- [ ] Include the curl smoke test

---

### Task 23: Add Playwright E2E spec for API Keys page

**Files to create:** `web/e2e/api-keys.spec.ts`

**Pattern reference:** `web/e2e/mailing-lists.spec.ts` — custom `test` from `./fixtures`, `page.goto`, role/placeholder selectors, `expect(...).toBeVisible()`, dialog acceptance for confirms.

**Details:**
- Runs against auth-disabled mode (the default for e2e — same setup as other specs).
- 5 specs:
  1. Empty state visible on first load.
  2. Create a key with name "ci-runner" → appears in the list with `mock_` prefix.
  3. Just-created panel shows the full key value and copy button.
  4. Delete a key with confirmation → disappears from the list.
  5. Create two keys, confirm both appear in the list in creation order.
- Use `test.beforeEach` to clear the `managed_api_keys` table via the existing reset helpers in `fixtures.ts` (or add one if needed — but prefer using the API helper to DELETE all existing keys via the new endpoint).

**Checklist:**
- [ ] Create `api-keys.spec.ts` with 5 specs
- [ ] Ensure beforeEach clears state
- [ ] `npx playwright test api-keys.spec.ts` passes
- [ ] Full suite still passes: `just lint && go test ./... && cd web && npx playwright test`

---

### Progress Tracking

| Task | Description | Status |
|------|-------------|--------|
| 1  | Extend `config.Config` with Entra fields + `Validate()` | Not Started |
| 2  | Create `internal/auth` package with `Validator` | Not Started |
| 3  | Add startup validation + context in `cmd/server/main.go` | Not Started |
| 4  | Add `ManagedAPIKey` model + migration | Not Started |
| 5  | Implement managed API key CRUD handlers | Not Started |
| 6  | Add `managed_keys` mode to Basic-path auth | Not Started |
| 7  | Rename `BasicAuth` → `APIAuth` with dual-auth Bearer path | Not Started |
| 8  | Create `EntraRequired` middleware | Not Started |
| 9  | Create WebSocket log-scrubbing middleware | Not Started |
| 10 | Add `/mock/auth-config` endpoint | Not Started |
| 11 | Wire auth into `server.New()` | Not Started |
| 12 | Add `@azure/msal-browser` dependency + `auth/config.ts` | Not Started |
| 13 | Create MSAL singleton wrapper `auth/msalInstance.ts` | Not Started |
| 14 | Create `useAuth` composable | Not Started |
| 15 | Refactor `main.ts` to async bootstrap | Not Started |
| 16 | Add auth interceptor to `api/client.ts` | Not Started |
| 17 | Thread token into WebSocket URL | Not Started |
| 18 | Add sign-in / user display to `App.vue` | Not Started |
| 19 | Create `ApiKeysPage.vue` | Not Started |
| 20 | Register `/api-keys` route + nav link | Not Started |
| 21 | Update `README.md` with Authentication section | Not Started |
| 22 | Update `E2E_TESTING.md` with manual verification | Not Started |
| 23 | Add Playwright E2E spec for API Keys page | Not Started |
