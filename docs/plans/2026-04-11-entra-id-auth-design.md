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
                                 │  managed_api_keys (plaintext)│
                                 │  existing Mailgun handlers   │
                                 └──────────────────────────────┘
```

### Key principles

1. **Two surfaces, but the auth mechanisms overlap** on the Mailgun-compat surface via dual-auth. The Vue SPA calls `/v3/*` and `/v4/*` extensively (domain picker, routes CRUD, templates, suppressions, etc.), so an Entra-only-on-`/mock/*` split is not enough. The Mailgun-compat middleware accepts **either** a valid Entra JWT (browser) **or** a valid managed API key (SDK clients).
2. **One switch, not two.** `AUTH_MODE=entra` is the *single* master switch for a locked-down deployment. When it's on, the Basic-auth path on the Mailgun-compat surface is structurally forced to validate against the managed-keys table — `mock.MockConfig.Authentication.AuthMode` is ignored on that path. This is deliberate: an operator who enables Entra should not have to also remember to flip a second, unrelated toggle in the runtime mock config to actually protect `/v3/*`. The old `full` / `single` / `accept_any` modes in mock config remain the governing behavior in `disabled` mode only.
3. **Both layers are opt-in** via `AUTH_MODE`. Local dev runs with `AUTH_MODE=disabled` and nothing changes for contributors. Deployed instances flip it on.
4. **Single source of truth: backend env vars.** The Vue bundle is config-free at build time and fetches its Entra settings from `/mock/auth-config` at startup. One binary, many deployments.
5. **Bootstrap order resolves itself.** Entra-authenticated UI users access the UI freely → mint API keys from the UI → distribute those keys to test apps. No chicken-and-egg.
6. **Same-origin by construction.** The Go binary embeds the Vue SPA via `//go:embed` and serves it from the same process as the API (`internal/server/server.go:32-34, 445`). Browsers load `https://mock.example.com/` and fetch `/v4/domains` from the *same* origin and port. `just dev-ui` uses a Vite server-side proxy (`web/vite.config.ts:18-26`) so even during frontend HMR the browser never crosses an origin. The Mailgun SDK calls the mock from server-side test code, not from a browser, so browser CORS is not in the threat model. **Therefore we do not need CORS middleware at all** — the existing `cors.Handler` block in `server.go:40-46` is dead weight (and has a latent spec bug: `AllowedOrigins: ["*"]` with `AllowCredentials: true` is rejected by browsers). This plan removes it; see Task 11.
7. **No new external services.** `github.com/coreos/go-oidc/v3` on the backend, `@azure/msal-browser` in the SPA, Microsoft's public JWKS/OIDC discovery endpoints.

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

**Bootstrap endpoint:** unauthenticated `GET /mock/auth-config` returns one of two shapes:

```json
// Disabled mode
{ "enabled": false }
```

```json
// Entra mode — all fields required when enabled: true
{
  "enabled": true,
  "tenantId": "...",
  "clientId": "...",
  "scopes": ["api://<client-id>/access_as_user"],
  "redirectUri": "https://mock.example.com"
}
```

The SPA models this as a TypeScript discriminated union (`{enabled: false} | {enabled: true; tenantId: string; ...}`) so the type checker enforces "if enabled, the config fields are guaranteed present" at compile time — no optional-field undefined derefs in `initMsal`. See Task 12. When `enabled: false`, the SPA skips MSAL initialization entirely and mounts as it does today.

## Backend changes (Go)

### New package: `internal/auth`

Owns Entra JWT validation. Uses `github.com/coreos/go-oidc/v3` for OIDC discovery and token verification. Single exported `Validator` struct:

```go
package auth

import (
    "context"
    "errors"
    "fmt"
    "net"
    "net/url"
    "strings"
    "github.com/coreos/go-oidc/v3/oidc"
)

type Claims struct {
    OID   string `json:"oid"`
    Email string `json:"preferred_username"`
    Name  string `json:"name"`
    Scope string `json:"scp"` // space-delimited list of granted scopes
}

// ErrProviderUnavailable signals that token validation could not complete
// because the identity provider (JWKS / discovery) was unreachable. Callers
// should surface this as 503, not 401 — the problem is "our side," not the
// caller's credentials. See H6 in the review.
var ErrProviderUnavailable = errors.New("auth: identity provider unavailable")

type Validator struct {
    verifier      *oidc.IDTokenVerifier
    requiredScope string
}

func NewValidator(ctx context.Context, tenantID, expectedAud, requiredScope string) (*Validator, error) {
    issuer := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)
    provider, err := oidc.NewProvider(ctx, issuer)
    if err != nil {
        return nil, err
    }
    return &Validator{
        verifier:      provider.Verifier(&oidc.Config{ClientID: expectedAud}),
        requiredScope: requiredScope,
    }, nil
}

func (v *Validator) Validate(ctx context.Context, raw string) (*Claims, error) {
    tok, err := v.verifier.Verify(ctx, raw)
    if err != nil {
        // Distinguish "can't reach JWKS" (our problem, 503) from "token is bad"
        // (caller's problem, 401). go-oidc surfaces JWKS fetch failures as
        // *url.Error / net.Error types; everything else (wrong sig, wrong
        // issuer, expired, malformed) is a token-content problem.
        var urlErr *url.Error
        var netErr net.Error
        if errors.As(err, &urlErr) || errors.As(err, &netErr) {
            return nil, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
        }
        return nil, err
    }
    var c Claims
    if err := tok.Claims(&c); err != nil {
        return nil, err
    }
    // Signature/issuer/audience are verified above. We additionally require
    // that the token carries the scope this app exposes — otherwise any token
    // minted against api://<client-id> (including for a different scope, or
    // an on-behalf-of exchange) would be accepted.
    if v.requiredScope != "" {
        granted := strings.Fields(c.Scope)
        found := false
        for _, s := range granted {
            if s == v.requiredScope {
                found = true
                break
            }
        }
        if !found {
            return nil, fmt.Errorf("token missing required scope %q", v.requiredScope)
        }
    }
    return &c, nil
}
```

Callers in `internal/middleware/middleware.go` check for `ErrProviderUnavailable` with `errors.Is(err, auth.ErrProviderUnavailable)` and return 503 with `WWW-Authenticate: Bearer error="temporarily_unavailable"`. All other validation errors return 401.

`expectedAud` is `"api://" + clientID`, not the bare client ID — Azure access tokens have `aud = api://<client-id>`. `requiredScope` is the bare scope name (e.g. `access_as_user`), matched against the `scp` claim — not the fully-qualified `api://<client-id>/access_as_user` form, which is only used client-side when requesting the token.

### Entra-specific gotchas

**Token version.** Azure AD issues two access token versions. The v1.0 token has `iss = https://sts.windows.net/<tenant>/`, the v2.0 token has `iss = https://login.microsoftonline.com/<tenant>/v2.0`. go-oidc's discovery returns the v2.0 issuer, so **the app registration manifest must set `accessTokenAcceptedVersion: 2`** or validation will fail with an issuer mismatch.

**`scp` claim form.** The SPA requests the token via MSAL with a fully-qualified scope (`api://<client-id>/access_as_user`), but the `scp` claim in the resulting JWT only contains the bare scope name (`access_as_user`). `NewValidator` and `ENTRA_API_SCOPE` use the bare form; only `ENTRA_API_SCOPE` as consumed by the SPA-facing `/mock/auth-config` response is concatenated into the fully-qualified form.

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

1. **Rename `BasicAuth` → `APIAuth`** and extend with a Bearer path AND an entra-mode override on the Basic path. All 401/503 responses carry an RFC 7235-compliant `WWW-Authenticate` header (H4), and JWKS fetch failures translate to 503 with a `temporarily_unavailable` error code (H6):

    ```go
    const (
        wwwAuthBearer = `Bearer realm="mailgun-mock-api"`
        wwwAuthBasic  = `Basic realm="mailgun-mock-api"`
    )

    func unauthorized(w http.ResponseWriter, challenge, msg string) {
        w.Header().Set("WWW-Authenticate", challenge)
        response.RespondError(w, http.StatusUnauthorized, msg)
    }

    func providerUnavailable(w http.ResponseWriter) {
        w.Header().Set("WWW-Authenticate", `Bearer error="temporarily_unavailable"`)
        response.RespondError(w, http.StatusServiceUnavailable, "Auth provider unavailable")
    }

    func APIAuth(configPtr *mock.MockConfig, db *gorm.DB, v *auth.Validator) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
            return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                // Bearer path (dual-auth for the SPA)
                if bearer := extractBearer(r); bearer != "" {
                    if v == nil {
                        unauthorized(w, wwwAuthBasic, "Forbidden")
                        return
                    }
                    if _, err := v.Validate(r.Context(), bearer); err != nil {
                        if errors.Is(err, auth.ErrProviderUnavailable) {
                            providerUnavailable(w)
                            return
                        }
                        unauthorized(w, wwwAuthBearer, "Invalid token")
                        return
                    }
                    next.ServeHTTP(w, r)
                    return
                }
                // Basic path.
                //
                // IMPORTANT: when entra is enabled (v != nil), the Basic path
                // is STRUCTURALLY forced to use managed-keys DB lookup,
                // regardless of configPtr.Authentication.AuthMode. This is the
                // anti-footgun: an operator who flips AUTH_MODE=entra must NOT
                // have to also remember to flip mock config to managed_keys
                // to actually lock down /v3/* — otherwise a stale "full" mock
                // config (the default) would leave the Mailgun-compat surface
                // open to any non-empty password.
                if v != nil {
                    username, password, ok := r.BasicAuth()
                    if !ok || username != "api" || password == "" {
                        unauthorized(w, wwwAuthBasic, "Forbidden")
                        return
                    }
                    var key apikey.ManagedAPIKey
                    if err := db.Where("key_value = ?", password).First(&key).Error; err != nil {
                        unauthorized(w, wwwAuthBasic, "Forbidden")
                        return
                    }
                    next.ServeHTTP(w, r)
                    return
                }
                // Disabled mode: existing switch on configPtr.Authentication.AuthMode
                // (full / single / accept_any, plus the new optional managed_keys
                // case for local testing of the managed-keys lookup without enabling Entra).
                // All 401 responses in the switch use unauthorized(w, wwwAuthBasic, ...).
                // ... existing switch body, adapted ...
            })
        }
    }
    ```

    A Bearer token that fails validation does **not** fall through to Basic Auth — that's a fail-fast to avoid ambiguous errors. Similarly, in entra mode the Basic path does **not** consult `mock.MockConfig.Authentication.AuthMode` at all; the runtime config cannot weaken the deployment's auth posture.

    `EntraRequired` (for `/mock/*`) uses the same `unauthorized` / `providerUnavailable` helpers, with `wwwAuthBearer` as the challenge.

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
                    unauthorized(w, wwwAuthBearer, "Unauthenticated")
                    return
                }
                if _, err := v.Validate(r.Context(), token); err != nil {
                    if errors.Is(err, auth.ErrProviderUnavailable) {
                        providerUnavailable(w)
                        return
                    }
                    unauthorized(w, wwwAuthBearer, "Invalid token")
                    return
                }
                next.ServeHTTP(w, r)
            })
        }
    }
    ```

3. **WebSocket log scrubbing.** A small wrapper around the chi logger for the `/mock/ws` route that strips `?access_token=...` from the logged URL so tokens don't land in access logs.

### `internal/apikey/` — extend

Add `ManagedAPIKey` model (lives alongside the existing Mailgun-compat `APIKey`, in the same package but in a new file `managed.go`):

```go
type ManagedAPIKey struct {
    database.BaseModel          // string UUID ID + CreatedAt/UpdatedAt/DeletedAt
    Name     string `gorm:"not null" json:"name"`
    KeyValue string `gorm:"uniqueIndex;not null" json:"key_value"`
    Prefix   string `gorm:"not null" json:"prefix"` // "mock_" + first 8 chars of the random suffix, for display in the UI list
}
```

`BaseModel` is the same string-UUID + soft-delete base used by every other model in the repo (`internal/database/database.go:15-28`), so listings and deletes stay consistent with the rest of the codebase. Key format: `mock_<base64url(32 random bytes)>`, with a `UNIQUE` constraint on `KeyValue`. New file `managed_handlers.go` for UI-facing CRUD:

- `GET /mock/api-keys` — list, returns full plaintext values
- `POST /mock/api-keys` — body `{name}`, server generates key, returns full value
- `DELETE /mock/api-keys/{id}` — hard delete

The existing `/v1/keys` Mailgun-compat handlers stay untouched as a pure mock. Clearly separate from the real key store.

**`managed_keys` mock config mode.** A new `managed_keys` value is added to the existing `full` / `single` / `accept_any` enum in `mock.MockConfig.Authentication.AuthMode`. When active, the APIAuth middleware validates the Basic Auth password against the `managed_api_keys` table.

This mode is **only consulted in `disabled` Entra mode** — it gives contributors a way to exercise the managed-keys lookup logic locally without needing a real Entra app registration, and lets the Task 6 unit tests run in isolation. In `entra` mode the Basic path unconditionally does managed-keys lookup, and `mock.MockConfig.Authentication.AuthMode` is ignored (see Key principle #2).

### `internal/server/server.go` — wiring

- Signature change: `func New(ctx context.Context, db *gorm.DB, cfg *config.Config) http.Handler`.
- In `New`: if `cfg.AuthMode == "entra"`, call `auth.NewValidator(ctx, cfg.EntraTenantID, "api://"+cfg.EntraClientID, cfg.EntraAPIScope)` (bare scope name — see the `scp` claim gotcha above). Fail fast on error. Otherwise `validator = nil`.
- Add `&apikey.ManagedAPIKey{}` to the existing `db.AutoMigrate(...)` block.
- **Delete the existing `cors.Handler(...)` block and its import.** Same-origin architecture (Key principle #6); `go mod tidy` drops `github.com/go-chi/cors`.
- Replace every `BasicAuth(h.Config())` call site (~30 of them) with `APIAuth(h.Config(), db, validator)`.
- Wrap the `/mock/*` route group with `EntraRequired(validator)` via `r.Use(...)` at the top of `r.Route("/mock", ...)`. Nil validator is a pass-through (disabled mode).
- **Move `/mock/health` and `/mock/auth-config` *out* of the `/mock` group** — register them at the root router *before* the group is defined, so they don't inherit `EntraRequired`. Otherwise health probes and the SPA's pre-login config fetch would be blocked.
- Register `GET|POST|DELETE /mock/api-keys` routes inside the Entra-protected `/mock` group.
- Wrap `/mock/ws` with the log-scrubbing middleware + `EntraRequired`, and when `validator != nil` arm a 30-minute `time.AfterFunc` after the handshake to bound the token-revocation window (H5; see Error handling).
- See Task 11 for the full checklist and the end-to-end `server_entra_test.go` that proves this wiring works.

## Frontend changes (Vue)

### New npm dependency

`@azure/msal-browser` (Microsoft's official SPA library). No community Vue wrapper — our usage is small enough that a thin composable is cleaner.

### New files

- **`web/src/auth/config.ts`** — `fetchAuthConfig(): Promise<AuthConfig>` helper that GETs `/mock/auth-config` once and returns the typed config.
- **`web/src/auth/msalInstance.ts`** — singleton `PublicClientApplication` factory constructed from the fetched config with `cache: { cacheLocation: "localStorage" }` (persistent across tab closes). Exports `msalInstance`, `getActiveAccount()`, `getAccessToken()`, `signIn()`, `signOut()`. `getAccessToken()` wraps `acquireTokenSilent` with an `acquireTokenRedirect` fallback for `InteractionRequiredAuthError`. `signOut()` calls `msal.logoutRedirect({ postLogoutRedirectUri: window.location.origin, account: getActiveAccount() })` — this terminates the user's Entra session (not just the local MSAL cache) and returns them to the SPA root, where `main.ts`'s bootstrap will re-run and kick them back into `loginRedirect` if auth is still enabled. Local-cache-only sign-out is deliberately NOT offered — it would create a user who appears signed out but whose next `acquireTokenSilent` call silently re-authenticates from the tenant session, which is confusing and defeats the point of the button.
- **`web/src/composables/useAuth.ts`** — reactive wrapper exposing `user`, `isAuthenticated`, `signIn`, `signOut` for components.
- **`web/src/pages/ApiKeysPage.vue`** — list/create/delete against `/mock/api-keys`. Same visual structure as existing pages: table, "New Key" modal with a name field, "Created" modal showing the plaintext key with copy-to-clipboard, delete confirmation. Empty-state copy: "No API keys yet. Test apps won't be able to call the Mailgun API surface until you create one."

### Modified files

- **`web/src/main.ts`** — becomes async. Flow:
  1. `const cfg = await fetchAuthConfig()`
  2. If `!cfg.enabled`: `startWebSocket()`, then mount (current behavior + explicit WS start).
  3. Otherwise: init MSAL with `cfg`, call `handleRedirectPromise()` to consume any pending redirect, check for an active account, call `loginRedirect({scopes})` if none, `startWebSocket()` once the active account is present, mount.
  - The explicit `startWebSocket()` call is load-bearing: the composable no longer self-initializes on import (see note below), because doing so would fire before auth is ready.
- **`web/src/api/client.ts`** — the `request<T>()` method gains a pre-flight step: if auth is enabled, call `getAccessToken()` and set `Authorization: Bearer <jwt>`. Applies uniformly to every URL — `/mock/*` and `/v3/*` alike. No per-page changes needed.
- **`web/src/composables/useWebSocket.ts`** — two changes:
  1. **Remove the module-level `connect()` call** at the bottom of the file. Today `connect()` fires as a side effect of importing the module, which happens during `App.vue`'s import chain *before* `main.ts` has had a chance to `await` the auth bootstrap. With Entra enabled, that early connect would hit `/mock/ws` with no token and fail. Replace the eager call with an exported `startWebSocket()` function that `main.ts` invokes at the end of `bootstrap()`, once auth is ready (or immediately, in disabled mode).
  2. Make `getWSUrl()` async: if auth is enabled, append `?access_token=<jwt>` to the WS URL before opening. The reconnection logic re-runs `getWSUrl()` each time, so token refresh on reconnect is automatic. On close with a 1008/4401 code, fetch a fresh token and reconnect.
- **`web/src/App.vue`** — sidebar header gains a small user block (name + sign-out button) shown only when auth is enabled. New nav link "API Keys" under the Config section.
- **`web/src/router/index.ts`** — register `/api-keys` route. Light navigation guard redirects to `signIn()` if auth is enabled and no active account (belt-and-braces; `main.ts` handles the common case).

**Per-page impact: zero.** Every existing page keeps calling `api.get("/v4/domains")` (etc.) exactly as today. The interceptor attaches the token.

## Data flows

### 1. Local dev (`AUTH_MODE=disabled`) — status quo

```
main.ts → GET /mock/auth-config → {enabled: false}
       → startWebSocket() → mount app
api.get("/v4/domains") → no Authorization header
APIAuth middleware → validator == nil → disabled-mode Basic arm
                   → mock config default "accept_any" → passes through
```

(The mock config default is `accept_any`, set in `internal/mock/handlers.go:82`. Any other mock-config mode would 401 a request with no `Authorization` header — so contributors running with `full` / `single` / `managed_keys` locally will see 401s from the SPA until they either change mode or send credentials.)

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

### 5. Test app calling `/v3/domain.com/messages.mime` (entra mode)

```
SDK sends: Authorization: Basic api:<managed-key>
APIAuth middleware: no Bearer → validator != nil (entra mode)
  → entra-mode Basic arm: DB lookup in managed_api_keys table (unconditional,
    independent of mock.MockConfig.Authentication.AuthMode)
  → found → mh.SendMIMEMessage runs
```

If the SDK presents an unknown password: 401, regardless of what the runtime mock config says. The mock config's `full` / `single` / `accept_any` / `managed_keys` values only take effect in `disabled` mode.

### 6. WebSocket

```
main.ts bootstrap() → (after auth is ready) → startWebSocket()
  → await getWSUrl() → acquireTokenSilent() → jwt
  → new WebSocket(`/mock/ws?access_token=${jwt}`)
EntraRequired → extract ?access_token → Validator.Validate → ok
hub.HandleWebSocket runs
```

Note the explicit `startWebSocket()` call from `main.ts`. The `useWebSocket` module deliberately does **not** connect on import — if it did, `App.vue`'s import chain would trigger the connect before `main.ts` awaits the auth bootstrap, and the WS would open with no token.

### 7. Token expiry mid-session

`acquireTokenSilent` refreshes transparently using MSAL's cached refresh token. If that fails, it throws `InteractionRequiredAuthError`, which the interceptor catches and calls `acquireTokenRedirect` → user briefly bounces through Microsoft and lands back on the page they were on.

## Error handling & edge cases

- **Startup validation.** In `entra` mode, `cmd/server/main.go` fails fast on missing `ENTRA_*` vars. In `disabled` mode bound to a non-loopback address, log a big warning: "Auth is disabled and server is listening on a public interface — test data is unprotected."
- **JWKS / discovery unreachable at startup.** `oidc.NewProvider` fails → server exits.
- **JWKS fetch failure during validation (H6).** The validator detects `*url.Error` / `net.Error` from `verifier.Verify` and wraps with `auth.ErrProviderUnavailable`. Middleware checks `errors.Is(err, auth.ErrProviderUnavailable)` and returns **503** `auth provider unavailable` with `WWW-Authenticate: Bearer error="temporarily_unavailable"`. All other validation errors return 401. This lets SDK clients and monitoring distinguish "bad token" (retry won't help) from "our problem" (retry will help).
- **Invalid tokens (H4 + H6).** Missing → 401 `unauthenticated` + `WWW-Authenticate: Bearer realm="mailgun-mock-api"`. Malformed/wrong sig/wrong iss/wrong aud/expired/missing-scope → 401 `invalid token` + same Bearer challenge. JWKS fetch failure → 503 (see above). Basic-arm 401s use `WWW-Authenticate: Basic realm="mailgun-mock-api"`.
- **Clock skew.** go-oidc tolerates small `iat`/`exp` drift internally. No action.
- **No API keys minted yet.** Fresh deployed instance has an empty `managed_api_keys` table. `ApiKeysPage` shows a one-time empty state explaining the consequence. Server logs key count at startup in deployed mode.
- **WebSocket token in query string leaking to logs.** Custom logging middleware for `/mock/ws` strips `?access_token=...` before logging. Unit tested.
- **WebSocket token expiry and revocation window (H5).** `EntraRequired` only validates on the initial handshake. A JWT is accepted for its full ~1h lifetime, and a user whose Entra account is revoked mid-session keeps streaming events until they next reconnect. Full mitigation (re-validating on a timer, maintaining an expiry tracker per connection) is heavyweight for a mock. Narrow fix: **the hub forcibly closes each WS connection 30 minutes after it opens** (`time.AfterFunc` armed at `HandleWebSocket` entry, writes a `CloseMessage(1001, "reauth")` and closes the underlying conn). The SPA's reconnect logic (Task 17's `scheduleReconnect`) then re-runs `getWSUrl()`, which re-acquires a fresh JWT via MSAL, and opens a new connection that goes through `EntraRequired.Validate` again. This bounds the "revoked user still has access" window to 30 minutes without server-side token tracking. This is a narrow-scope, deliberate trade-off — documented here so reviewers know it's not an oversight. Only armed when `v != nil` (entra mode); disabled-mode connections are untouched.
- **Key uniqueness.** `mock_<base64url(32 random bytes)>` with `UNIQUE` constraint and one retry on collision.
- **Vite dev proxy.** `web/vite.config.ts` already proxies `/mock` and the Mailgun-compat paths. Local Entra usage with `http://localhost:5173` as redirect URI is documented in `README.md`'s Authentication section (Task 21) — both `:5173` (Vite dev) and `:8025` (Go binary direct) should be added to the app registration so either workflow works. Not required for normal dev since the default `disabled` mode sidesteps Entra entirely.
- **Revocation.** DB-backed, no caching layer. `DELETE /mock/api-keys/{id}` takes effect on the next request.

## Testing strategy

### Unit tests (Go)

- **`internal/auth/validator_test.go` (new, Task 2).** `httptest.Server` serving a fake `.well-known/openid-configuration` + JWKS with a test RSA key; signed JWTs generated in-test via `golang-jwt/jwt/v5`. Cases: valid-with-scope / missing-scope / scope-among-several / expired / wrong-aud / wrong-iss / malformed / **JWKS-unreachable asserts `errors.Is(err, auth.ErrProviderUnavailable)`** (H6).
- **`internal/middleware/auth_test.go` (extend, Tasks 6 / 7 / 8).**
  - Task 6: `TestBasicAuth_ManagedKeys_Valid|Invalid|EmptyTable`.
  - Task 7 (APIAuth): Bearer-valid / Bearer-invalid-no-fallthrough / Bearer-nil-validator-401 / EntraBasic-valid-managed-key / EntraBasic-invalid-key-401 / **`TestAPIAuth_EntraBasic_IgnoresMockConfigFullMode` — the Critical-#2 regression test** / `Bearer_Invalid_SetsBearerChallenge` (H4) / `Basic_Invalid_SetsBasicChallenge` (H4) / `Bearer_ProviderUnavailable_503` (H6).
  - Task 8 (EntraRequired): disabled-passthrough / header-valid / header-invalid-Bearer-challenge / query-param-valid / query-param-invalid / no-token-401-Bearer-challenge / `ProviderUnavailable_503`.
- **`internal/middleware/ws_logging_test.go` (new, Task 9).** `WSLogScrubber_RedactsAccessToken|LeavesOtherParams|NoQueryParams`.
- **`internal/apikey/managed_handlers_test.go` (new, Task 5).** CRUD coverage for `/mock/api-keys`: empty list / create / create-empty-name-400 / list-after-create / delete / list-after-delete.
- **`internal/config/config_test.go` (new or extend, Task 1).** Missing `ENTRA_*` in `entra` mode → error; `disabled` mode with empty Entra vars → ok.
- **`internal/mock/auth_config_test.go` (new, Task 10).** `GetAuthConfig_Disabled` / `GetAuthConfig_Enabled`.

### Integration test (Go)

- **`internal/server/server_entra_test.go` (new, Task 11, H12).** One test boots the full `server.New` in entra mode pointed at a fake OIDC provider, and walks the 8-case scenario in Task 11 — proves the actual wiring works end-to-end. This is the only auth-enabled server-level test; unit tests above cover isolated pieces. Includes:
  - `/mock/health` → 200 unauthenticated (regression test for H10 health-placement fix)
  - `/mock/auth-config` → 200 unauthenticated
  - `/v4/domains` no auth → 401 + `WWW-Authenticate: Basic realm=...`
  - `/v4/domains` expired Bearer → 401 + `WWW-Authenticate: Bearer realm=...`
  - `/v4/domains` valid Bearer (correct aud + scp) → 200
  - `/v4/domains` Basic with unknown key → 401 (Critical-#2 regression at the wiring level)
  - Mint a key via `/mock/api-keys`, re-hit `/v4/domains` with Basic → 200
  - `/mock/dashboard` no token → 401 + `WWW-Authenticate: Bearer`

### Existing tests

All run with `AUTH_MODE=disabled` (the zero value). No existing test changes. This is the test for "we didn't break anything."

### E2E (Playwright)

- **`web/e2e/api-keys.spec.ts` (new, Task 23).** Five specs against a server with auth disabled: create a key, see it in the list, copy-to-clipboard modal shows plaintext, delete, list empty.
- **Full Entra redirect flow** — not automated. Documented as manual verification in `README.md` (Task 21) and `E2E_TESTING.md` (Task 22).

### Not tested (and why)

- The MSAL SPA flow itself — it's Microsoft's library; mocking the identity provider in a browser is more work than value.
- `/mock/auth-config` with `enabled: true` — covered by Task 10's `GetAuthConfig_Enabled` unit test and the Task 11 integration test.

## Documentation updates

### `README.md` — new "Authentication" section

Full content spec lives in **Task 21** (below). At a high level the section covers:
- Local dev default (auth off, nothing to configure).
- Enabling Entra ID for deployed instances: 7-step app registration walkthrough including SPA redirect URIs (both local ports for dev testing — H9), logout URL, exposed API scope, `accessTokenAcceptedVersion: 2`, and optional `groupMembershipClaims: "SecurityGroup"` for future group authz (H13).
- Env vars table.
- First-API-key walkthrough + how test apps use minted keys.
- Five troubleshooting entries (token version, redirect URI mismatch, missing API key, `invalid_scope`, 503 from JWKS egress).

### `E2E_TESTING.md`

New manual-verification section for the full Entra flow.

## Summary of decisions

1. **Entra ID (MSAL, self-hosted option C)** protects `/mock/*`, `/mock/ws`, and — via dual-auth — `/v3/*`, `/v4/*` etc. when called from the browser.
2. **Managed API keys** stored plaintext in DB, minted from the UI, used by test apps via Basic Auth. In entra mode this is the *only* accepted Basic-Auth credential — `mock.MockConfig.Authentication.AuthMode` is ignored.
3. **Single Entra app registration** with SPA platform + exposed API scope + logout redirect URI.
4. **Bootstrap config endpoint** (`/mock/auth-config`) keeps the SPA bundle config-free. Typed as a TypeScript discriminated union on the frontend.
5. **Both auth layers opt-in** via `AUTH_MODE` — zero impact on local dev and existing tests.
6. **`github.com/coreos/go-oidc/v3`** as the sole new Go dependency; **`@azure/msal-browser`** as the sole new npm dependency. Task 11 also **removes** the unused `github.com/go-chi/cors` dependency — the architecture is same-origin by construction, so CORS headers serve no purpose.
7. **`scp` claim validated** against `ENTRA_API_SCOPE` (bare name). Tokens minted for other scopes against the same audience are rejected.
8. **WebSocket token in query string** (option A) with log scrubbing middleware. Server forcibly closes entra-mode WS connections every 30 minutes to bound the token-revocation window.
9. **Sign-out uses `logoutRedirect`** — local-cache-only sign-out is not offered, to avoid the "signed out but silently re-authenticates" UX trap.
10. **401 responses carry `WWW-Authenticate`** with the appropriate challenge scheme (Bearer on JWT/Entra paths, Basic on managed-key paths). JWKS fetch failures translate to 503 + `temporarily_unavailable` so SDK clients and monitoring can distinguish retriable from non-retriable failures.
11. **No CORS middleware** — the embedded SPA and the API share one origin; the Vite dev proxy keeps local dev same-origin too. Mailgun SDKs are server-side and never cross a browser origin.
12. **No group filtering** for now; deferred. Documentation tells operators how to enable group claims in the app registration so the future follow-up is a pure code change.

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
- [x] Add new `Config` fields
- [x] Extend `Load()` with new env vars
- [x] Add `Validate()` method (includes unknown AuthMode rejection)
- [x] Write `config_test.go` with disabled-mode, full-entra, missing-var, and unknown-AuthMode cases
- [x] `go test ./internal/config/...` passes

---

### Task 2: Create `internal/auth` package with `Validator`

**Files to create:** `internal/auth/validator.go`, `internal/auth/validator_test.go`
**Files to modify:** `go.mod`, `go.sum`

**Pattern reference:** None in the repo — this is a new package. For test style, mirror `internal/middleware/middleware_test.go` (helpers + subtests, `httptest.NewServer` for fakes).

**Details:**
- Add dependency: `go get github.com/coreos/go-oidc/v3@latest`.
- Define `Claims` struct with `OID`, `Email` (from `preferred_username`), `Name`, `Scope` (from `scp`).
- Define `Validator` struct wrapping `*oidc.IDTokenVerifier` and a `requiredScope` string.
- Export sentinel error `ErrProviderUnavailable` (see H6) for callers to distinguish JWKS/network failures from token-content failures.
- `NewValidator(ctx context.Context, tenantID, expectedAud, requiredScope string) (*Validator, error)` constructs issuer URL (`https://login.microsoftonline.com/<tenant>/v2.0`), calls `oidc.NewProvider`, and builds a verifier with `ClientID: expectedAud`.
- `Validate(ctx, raw string) (*Claims, error)` calls `verifier.Verify`, extracts claims, AND verifies the `scp` claim contains `requiredScope` (space-delimited list — use `strings.Fields` and an exact match). An empty `requiredScope` disables the check (used by tests that want to isolate audience/issuer verification from scope handling). When `verifier.Verify` returns a `*url.Error` / `net.Error`, wrap with `ErrProviderUnavailable` via `fmt.Errorf("%w: ...", auth.ErrProviderUnavailable, err)`.
- Tests: run an `httptest.NewServer` that serves a fake `.well-known/openid-configuration` and JWKS using an RSA test key (`crypto/rsa` + `jwt.SigningMethodRS256`). Use `github.com/golang-jwt/jwt/v5` to sign test tokens — **add it as a direct dependency** (`go get github.com/golang-jwt/jwt/v5`) rather than relying on the transitive pull via go-oidc. Cases:
  - valid token with required scope → claims extracted, Scope field populated
  - valid token missing required scope → error containing "missing required scope"
  - valid token with required scope among several → claims extracted
  - expired token → error
  - wrong audience → error
  - wrong issuer → error
  - malformed token → error
  - **JWKS unreachable (H6)**: stop the httptest server before calling `Validate` (or use a guaranteed-closed port), assert the returned error satisfies `errors.Is(err, auth.ErrProviderUnavailable)`
- **Note on gotcha:** test tokens should use issuer `<httptest-server-url>/v2.0` to match what `NewValidator` builds — parameterize via an internal `newValidatorForIssuer` helper the test can call.

**Checklist:**
- [x] `go get github.com/coreos/go-oidc/v3@latest`
- [x] `go get github.com/golang-jwt/jwt/v5@latest` (direct test dep)
- [x] Export `ErrProviderUnavailable` sentinel
- [x] Write `validator.go` with `Claims` (incl. `Scope`), `Validator` (incl. `requiredScope`), `NewValidator`, `Validate` with scope check and JWKS-failure translation
- [x] Expose an unexported `newValidatorForIssuer(ctx, issuerURL, aud, requiredScope)` used by tests only
- [x] Write `validator_test.go` with the eight cases above (incl. `ErrProviderUnavailable` assertion)
- [x] `go test ./internal/auth/...` passes
- [x] `go build ./...` still compiles

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
- [x] Import `context`
- [x] Create and cancel top-level context
- [x] Call `cfg.Validate()` with fatal on error
- [x] Log "auth disabled" warning in disabled mode
- [x] Update call site for `server.New` (compiles against Task 11's new signature; do this task AFTER Task 11 or use a temporary `_ = ctx` to keep things compiling in between)
- [x] `just build` succeeds

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
- [x] Create `internal/apikey/managed.go` with the model + key-generator helper
- [x] Add `&apikey.ManagedAPIKey{}` to `AutoMigrate` call in `server.go`
- [x] `go build ./...` succeeds
- [x] Run server locally with `just dev` — confirm new table is created (log check or sqlite inspection)

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
- [x] Create `managed_handlers.go` with `ManagedHandlers`, `NewManagedHandlers`, `List`, `Create`, `Delete`
- [x] Write `managed_handlers_test.go` with the 6 cases above
- [x] `go test ./internal/apikey/...` passes

---

### Task 6: Add `managed_keys` mode to Basic-path auth (disabled-mode pathway)

**Files to modify:** `internal/middleware/middleware.go`, `internal/middleware/auth_test.go`

**Pattern reference:** Existing `BasicAuth` switch at `internal/middleware/middleware.go:85-118`. Existing tests in `auth_test.go`.

**Note on scope:** This task only adds the `managed_keys` *enum value* to the existing mock-config switch. This pathway is consulted only when entra is disabled — it exists so contributors can exercise the managed-keys lookup locally, and so Task 6's unit tests can isolate the DB lookup from JWT validation. Task 7 separately adds the entra-mode Basic-path override that bypasses this switch entirely.

**Details:**
- Add a `case "managed_keys":` branch to the switch. Extract `username, password, ok := r.BasicAuth()`; on mismatch of format → 401; otherwise `db.Where("key_value = ?", password).First(&apikey.ManagedAPIKey{})`; not-found → 401; found → pass through.
- This means the middleware now needs access to `*gorm.DB`. Update `BasicAuth` signature to `BasicAuth(cfg *mock.MockConfig, db *gorm.DB)` (still no Entra yet — that's Task 7).
- Update all call sites in `internal/server/server.go` to pass `db` (mechanical change).
- Extend `auth_test.go`: `TestBasicAuth_ManagedKeys_Valid`, `TestBasicAuth_ManagedKeys_Invalid`, `TestBasicAuth_ManagedKeys_EmptyTable`.

**Checklist:**
- [x] Add `managed_keys` case to `BasicAuth` switch
- [x] Change `BasicAuth` signature to accept `*gorm.DB`
- [x] Update all call sites in `server.go` (mechanical)
- [x] Extend `auth_test.go` with 3 new cases
- [x] `go test ./internal/middleware/...` passes
- [x] `go build ./...` passes (catches missed call sites)

---

### Task 7: Rename `BasicAuth` → `APIAuth` with dual-auth Bearer path + entra-mode Basic override

**Files to modify:** `internal/middleware/middleware.go`, `internal/middleware/auth_test.go`, `internal/server/server.go`

**Pattern reference:** The existing `BasicAuth` function this extends, plus the new `auth.Validator` from Task 2.

**Details:**
- Rename `BasicAuth` → `APIAuth`. Update all call sites in `server.go`.
- Extend signature: `APIAuth(cfg *mock.MockConfig, db *gorm.DB, v *auth.Validator) func(http.Handler) http.Handler`. A `nil` validator means Entra is disabled.
- Handler body has three sequential arms:
  1. **Bearer arm.** If `extractBearer(r) != ""`: require `v != nil` (else 401); call `v.Validate`; success → pass through; failure → 401, **no fall-through** (avoids ambiguous error cases).
  2. **Entra-mode Basic arm.** If no Bearer AND `v != nil`: do managed-keys DB lookup directly (same logic as Task 6's `managed_keys` case but hardcoded in this branch — do NOT consult `cfg.Authentication.AuthMode`). Not found → 401. This is the structural anti-footgun — in entra mode the runtime mock config cannot weaken the Basic-path check.
  3. **Disabled-mode Basic arm.** If no Bearer AND `v == nil`: existing switch on `cfg.Authentication.AuthMode` (now including Task 6's `managed_keys` case).
- Helper function `extractBearer(r *http.Request) string` (unexported; used by Task 8 too — put in the same file).
- Every 401 response MUST set `WWW-Authenticate` (H4):
  - Bearer-arm 401s: `WWW-Authenticate: Bearer realm="mailgun-mock-api"`.
  - Basic-arm 401s (both entra-mode and disabled-mode): `WWW-Authenticate: Basic realm="mailgun-mock-api"`.
- When `Validate` returns an error wrapping `auth.ErrProviderUnavailable` (H6), return 503 with `WWW-Authenticate: Bearer error="temporarily_unavailable"` — use `errors.Is(err, auth.ErrProviderUnavailable)` to detect.
- New tests:
  - `TestAPIAuth_Bearer_ValidToken`, `TestAPIAuth_Bearer_InvalidToken_NoFallthrough`, `TestAPIAuth_Bearer_DisabledValidator_401` (when v == nil and Bearer is present, return 401 — not fall through to Basic, since the caller is clearly trying to use Bearer and silently accepting anything would be a footgun of its own).
  - `TestAPIAuth_EntraBasic_ValidManagedKey`, `TestAPIAuth_EntraBasic_InvalidKey_401`, `TestAPIAuth_EntraBasic_IgnoresMockConfigFullMode` — the last test explicitly sets `cfg.Authentication.AuthMode = "full"` with a validator present and asserts that a request with a nonsense Basic password is still 401'd. This is the regression test for Critical #2.
  - `TestAPIAuth_Bearer_Invalid_SetsBearerChallenge` — 401 response carries `WWW-Authenticate: Bearer realm=...` (H4).
  - `TestAPIAuth_Basic_Invalid_SetsBasicChallenge` — 401 response carries `WWW-Authenticate: Basic realm=...` (H4).
  - `TestAPIAuth_Bearer_ProviderUnavailable_503` — construct a validator whose JWKS URL points at a stopped server; assert the response is 503 with `WWW-Authenticate: Bearer error="temporarily_unavailable"` (H6).
  - Use the `newValidatorForIssuer` helper from Task 2 + a fake JWKS httptest server.

**Checklist:**
- [x] Rename `BasicAuth` → `APIAuth` with new signature
- [x] Implement Bearer arm with fail-fast on invalid token
- [x] Implement entra-mode Basic arm (managed-keys lookup, bypassing mock config)
- [x] Preserve disabled-mode Basic arm behavior unchanged
- [x] Add `extractBearer`, `unauthorized`, `providerUnavailable` helpers
- [x] All 401s set `WWW-Authenticate` with the correct scheme (H4)
- [x] `ErrProviderUnavailable` maps to 503 + `Bearer error="temporarily_unavailable"` (H6)
- [x] Update all call sites in `server.go`
- [x] Add 9 new tests (6 original + 3 for H4/H6)
- [x] `go test ./internal/middleware/...` passes
- [x] `go build ./...` passes

---

### Task 8: Create `EntraRequired` middleware (REST + WS variants)

**Files to modify:** `internal/middleware/middleware.go`, `internal/middleware/auth_test.go`

**Pattern reference:** The newly-renamed `APIAuth` from Task 7 — same validator-or-nil pattern.

**Details:**
- `EntraRequired(v *auth.Validator) func(http.Handler) http.Handler` — one function, handles both REST and WS via dual token extraction.
- Logic: if `v == nil`, pass through (disabled mode). Otherwise extract token from `Authorization: Bearer <token>` OR from `?access_token=<token>` query param. No token → 401 with `WWW-Authenticate: Bearer realm=...` (H4). Validate → if `errors.Is(err, auth.ErrProviderUnavailable)` return 503 with `WWW-Authenticate: Bearer error="temporarily_unavailable"` (H6), else 401 with Bearer challenge. Success → pass through.
- Uses the same `unauthorized` / `providerUnavailable` helpers defined in Task 7.
- Tests:
  - `TestEntraRequired_DisabledPassthrough`
  - `TestEntraRequired_Header_Valid`
  - `TestEntraRequired_Header_Invalid_BearerChallenge` — asserts 401 + `WWW-Authenticate: Bearer realm=...`
  - `TestEntraRequired_QueryParam_Valid` (WS simulation)
  - `TestEntraRequired_QueryParam_Invalid` (WS simulation with bad token — 401 + Bearer challenge)
  - `TestEntraRequired_NoToken_401_BearerChallenge`
  - `TestEntraRequired_ProviderUnavailable_503` — validator pointed at a stopped JWKS server; assert 503 + `WWW-Authenticate: Bearer error="temporarily_unavailable"`

**Checklist:**
- [x] Implement `EntraRequired` with header + query-param extraction
- [x] All 401s set `WWW-Authenticate: Bearer realm=...`
- [x] `ErrProviderUnavailable` maps to 503
- [x] Add 7 new tests (plus 1 additional: header-takes-priority-over-query-param)
- [x] `go test ./internal/middleware/...` passes

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
- [x] Decide on implementation approach (per-route manual logging is simpler — prefer it)
- [x] Implement `WSLogScrubber`
- [x] Add 3 tests
- [x] `go test ./internal/middleware/...` passes

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
- [x] Thread `*config.Config` into `mock.Handlers` (signature change)
- [x] Add `GetAuthConfig` handler
- [x] Add tests
- [x] `go test ./internal/mock/...` passes
- [x] `go build ./...` passes

---

### Task 11: Wire auth into `server.New()` (+ remove unused CORS, move `/mock/health`)

**Files to modify:** `internal/server/server.go`, `cmd/server/main.go`, `go.mod`, `go.sum`

**Pattern reference:** Existing route-wrapping style in `server.go:118-442`.

**Details:**

This task is larger than the others because it's the point where auth becomes real in the running server, and where two pre-existing cleanups the plan touches anyway are resolved.

*Signature + validator construction*
- Change signature: `func New(ctx context.Context, db *gorm.DB, cfg *config.Config) http.Handler`.
- At the top of `New`, if `cfg.AuthMode == "entra"`, construct `validator, err := auth.NewValidator(ctx, cfg.EntraTenantID, "api://"+cfg.EntraClientID, cfg.EntraAPIScope)`; `log.Fatalf` on error. Otherwise `validator = nil`. Note the third argument is the bare scope name from config (e.g. `access_as_user`), not the fully-qualified `api://<client-id>/access_as_user` form — the `scp` claim only carries the bare name.
- Update `mock.NewHandlers` call to pass `cfg` (from Task 10).

*Remove the CORS middleware (H7)*
- The embedded SPA and the API are served by the same Go process on the same port (`//go:embed all:static` + `spaHandler()` + `r.Handle("/*", spaHandler())`). `just dev-ui` uses a Vite server-side proxy. Browsers never cross an origin; Mailgun SDKs call server-to-server. **There is no cross-origin caller in the threat model.**
- Delete the `cors.Handler(cors.Options{...})` block at `server.go:40-46` entirely.
- Remove the `"github.com/go-chi/cors"` import.
- `go mod tidy` to drop the dependency from `go.mod` / `go.sum`.
- This also resolves the pre-existing `AllowedOrigins: ["*"]` + `AllowCredentials: true` spec violation by deletion.

*Route-level auth wiring*
- Replace every `appMiddleware.BasicAuth(h.Config())` call with `appMiddleware.APIAuth(h.Config(), db, validator)`. (There are ~30 of these — careful find/replace.)
- Wrap all `/mock/*` routes with `EntraRequired(validator)`: at the top of the `r.Route("/mock", ...)` block (currently `server.go:412`), add `r.Use(appMiddleware.EntraRequired(validator))`.
- Register the new `/mock/api-keys` routes *inside* the Entra-protected `/mock` group: `r.Get("/api-keys", mkh.List); r.Post("/api-keys", mkh.Create); r.Delete("/api-keys/{id}", mkh.Delete)`.
- Apply `WSLogScrubber` + `EntraRequired` to the `/mock/ws` route specifically.

*Move `/mock/health` and `/mock/auth-config` out of the Entra-protected group (H10)*
- Today `/mock/health` is registered *inside* `r.Route("/mock", ...)` at `server.go:414`. Once `r.Use(EntraRequired)` is added to that group, `/mock/health` inherits it — which breaks load-balancer health probes and Kubernetes liveness checks.
- Remove `r.Get("/health", mock.HealthHandler)` from the `/mock` group.
- Register both public endpoints at the root router *before* the `/mock` group is defined:
  ```go
  r.Get("/mock/health", mock.HealthHandler)
  r.Get("/mock/auth-config", h.GetAuthConfig)
  r.Route("/mock", func(r chi.Router) {
      r.Use(appMiddleware.EntraRequired(validator))
      // ... rest of /mock routes, minus /health ...
  })
  ```
- Verify by diff that `/health` no longer appears inside the `/mock` group block.

*Arm the 30-minute WS reauth timer (H5)*
- In the WebSocket hub's `HandleWebSocket` method (or a thin wrapper registered on the `/mock/ws` route when `validator != nil`), after the connection is upgraded, arm `time.AfterFunc(30*time.Minute, func() { conn.WriteMessage(websocket.CloseMessage, ...); conn.Close() })`. Cancel the timer on normal close. See the Error handling section for the rationale.
- Only armed when `validator != nil` (disabled-mode connections keep current behavior).
- If the existing hub does not expose a per-connection hook, add a minimal middleware at the route level that wraps `hub.HandleWebSocket` and arms the timer after the handshake.

*Main.go*
- Update `cmd/server/main.go` to call `server.New(ctx, db, cfg)`.

*Entra-mode end-to-end integration test (H12)*
New file: `internal/server/server_entra_test.go`. One test function that boots `server.New` in entra mode pointed at a fake OIDC provider (httptest server reusing Task 2's RSA test keys + a fake JWKS), then asserts:
1. `GET /mock/health` → 200 (unauthenticated — the group exception works).
2. `GET /mock/auth-config` → 200 `{"enabled": true, ...}` (unauthenticated).
3. `GET /v4/domains` with no auth → 401 + `WWW-Authenticate: Basic realm=...`.
4. `GET /v4/domains` with an expired Bearer → 401 + `WWW-Authenticate: Bearer realm=...`.
5. `GET /v4/domains` with a valid Bearer (signed by the fake JWKS, correct aud, correct scp) → 200.
6. `GET /v4/domains` with Basic `api:<unknown-key>` → 401. **Regression test for Critical #2 at the wiring level** — even though the mock config defaults to `full` (accept any non-empty password), entra mode must reject unknown Basic passwords.
7. `POST /mock/api-keys` with valid Bearer creates a key; then `GET /v4/domains` with Basic `api:<that-key>` → 200.
8. `GET /mock/dashboard` with no token → 401 + `WWW-Authenticate: Bearer`.

This is the only server-level auth-enabled test; unit tests in Task 2 / Task 7 / Task 8 still cover the isolated pieces, but this test is what proves the wiring itself works.

**Checklist:**
- [ ] Change `server.New` signature
- [ ] Construct validator at startup in entra mode
- [ ] **Delete `cors.Handler` block and import; run `go mod tidy`**
- [ ] Replace all `BasicAuth` call sites with `APIAuth`
- [ ] Wrap `/mock/*` group with `EntraRequired`
- [ ] **Move `/mock/health` out of the `/mock` group (register at root)**
- [ ] Register `/mock/auth-config` at root (not inside the `/mock` group)
- [ ] Wire `/mock/api-keys` CRUD routes inside the Entra-protected group
- [ ] Apply WS scrubber + `EntraRequired` to `/mock/ws`
- [ ] Arm 30-minute WS reauth timer when `validator != nil`
- [ ] Update `main.go` call site
- [ ] Write `server_entra_test.go` with the 8 cases above
- [ ] `just build` succeeds (catches missed call sites and the CORS removal)
- [ ] `go test ./...` passes — both the existing disabled-mode tests and the new entra-mode test
- [ ] Manual smoke: `just dev`, `curl localhost:8025/mock/auth-config` → `{"enabled":false}`
- [ ] Manual smoke: `just dev`, `curl -i localhost:8025/mock/health` → 200 (confirms the routing move)

---

### Task 12: Add `@azure/msal-browser` dependency + `auth/config.ts`

**Files to modify:** `web/package.json`, `web/package-lock.json`
**Files to create:** `web/src/auth/config.ts`

**Pattern reference:** `web/src/api/client.ts` for fetch-based helper style.

**Details:**
- `cd web && npm install @azure/msal-browser`.
- Create `web/src/auth/config.ts`:
  ```ts
  // Discriminated union: if enabled is true, all other fields are guaranteed present.
  // This forecloses a class of "undefined deref in initMsal" bugs at the type level.
  export type AuthConfig =
    | { enabled: false }
    | {
        enabled: true;
        tenantId: string;
        clientId: string;
        scopes: string[];
        redirectUri: string;
      };

  export async function fetchAuthConfig(): Promise<AuthConfig> {
    const res = await fetch("/mock/auth-config", { headers: { Accept: "application/json" } });
    if (!res.ok) throw new Error(`auth-config fetch failed: ${res.status}`);
    return res.json();
  }
  ```
- Callers use `if (cfg.enabled) { cfg.tenantId /* known string */ }` narrowing — no `?.` or `!` needed.
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
- `signIn()` calls `msalInstance.loginRedirect({scopes})`.
- **`signOut()` (H8)** calls `msalInstance.logoutRedirect({ postLogoutRedirectUri: window.location.origin, account: getActiveAccount() })`. This terminates the Entra tenant session — not just the local MSAL cache — and redirects the browser back to the SPA root. `main.ts`'s bootstrap re-runs on arrival and `loginRedirect` fires again because there is no longer an active account. **Do not** offer a local-cache-only sign-out variant: it would create a "signed out" user whose next silent token acquisition succeeds, which is confusing.
- Requires `postLogoutRedirectUri` (the SPA root) to be registered in the Entra app registration under the SPA platform. Task 21 documents this.
- MSAL config uses `cacheLocation: "localStorage"`.
- No unit tests — MSAL is Microsoft's library; we trust it. Integration verification in Task 15's manual smoke.

**Checklist:**
- [ ] Create `msalInstance.ts` with the functions above
- [ ] `signOut` uses `logoutRedirect` with `postLogoutRedirectUri`
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

### Task 17: Defer WebSocket connect + thread token into URL

**Files to modify:** `web/src/composables/useWebSocket.ts`, `web/src/main.ts`

**Pattern reference:** Current `getWSUrl` helper and module-level `connect()` call at the bottom of `useWebSocket.ts`. This task rewrites both.

**Details:**

There are two coupled changes. Do them together — doing either alone leaves the WS broken in one of the two auth modes.

1. **Defer the initial connect.** The current file ends with:

   ```ts
   // Initialize connection on first import
   connect();
   ```

   This runs as soon as anything imports the module. `web/src/App.vue:3` imports `useWebSocket` at the top of its `<script setup>`, so the connect fires during `main.ts`'s `import App from "./App.vue"` — synchronously, before any `await` in `bootstrap()`. With Entra enabled, that means the WS opens with no token and is immediately 401'd.

   Fix: **delete the module-level `connect()` call** and instead export a `startWebSocket()` function:

   ```ts
   export function startWebSocket() {
     connect();
   }
   ```

   The `useWebSocket()` composable itself becomes a pure consumer — it reads the reactive `connected` state and lets callers subscribe to messages, but never initiates the connection. Consumer components (`App.vue`, `DashboardPage.vue`, `EventsPage.vue`, `MessagesPage.vue`) do not change — they continue to call `useWebSocket()` as before.

2. **Thread the token through `getWSUrl`.** Make `getWSUrl()` async:
   - `const token = await getAccessToken();` (returns `null` in disabled mode).
   - If token present, append `?access_token=${encodeURIComponent(token)}`.
   - Otherwise return the URL unchanged (preserves disabled-mode behavior).
   - Update `connect()` to `await getWSUrl()` before constructing `new WebSocket(url)`. `connect` becomes async.
   - `scheduleReconnect` continues to work — it calls `connect()` which re-runs `getWSUrl()`, so a fresh token is acquired on each reconnect. This covers the "token expired mid-session, WS dropped, reconnect" path automatically.

3. **Call `startWebSocket()` from `main.ts`.** At the end of `bootstrap()`, after auth is ready (in both branches — disabled and entra), call `startWebSocket()` before `createApp(...).mount(...)`. Task 15's bootstrap flow already has the branching; add `startWebSocket()` to both arms.

**Checklist:**
- [ ] Delete the module-level `connect()` call in `useWebSocket.ts`
- [ ] Export `startWebSocket()` wrapping `connect()`
- [ ] Make `getWSUrl` async; await `getAccessToken`; append `?access_token=...` when present
- [ ] Make `connect()` async; await `getWSUrl()` before opening the socket
- [ ] Verify reconnection path still works (re-runs `getWSUrl` → fresh token)
- [ ] Call `startWebSocket()` from both arms of `main.ts`'s `bootstrap()`
- [ ] `npm run lint` passes
- [ ] `npm run build` succeeds
- [ ] Manual smoke (disabled mode): `just dev`, open browser, confirm "Connected" indicator appears and the WS URL in DevTools has no `access_token` param
- [ ] Manual smoke (entra mode, once Task 21 is done): confirm the WS URL in DevTools has a `?access_token=<jwt>` param and "Connected" appears only after sign-in completes

---

### Task 18: Add sign-in / user display / sign-out to `App.vue`

**Files to modify:** `web/src/App.vue`

**Pattern reference:** Current sidebar header at `App.vue:11-22`.

**Details:**
- Import `useAuth`. Get `user`, `isAuthenticated`, `signOut`.
- In the `sidebar-header` block, under the connection status div, add `v-if="isAuthenticated"` block with `{{ user?.name }}` and a "Sign out" button (`@click="signOut"`). When disabled mode, `isAuthenticated` is false and this block never renders.
- Styling: match existing sidebar typography; small text, muted color. Keep it minimal.
- **Sign-out behavior (H8).** `signOut` resolves to Task 13's `logoutRedirect`-backed function. Clicking it triggers a full-page navigation to Entra's logout endpoint, which invalidates the tenant session and then redirects the browser to `window.location.origin`. `main.ts`'s bootstrap re-runs on arrival, and because the Entra session is gone, `loginRedirect` fires and the user lands on the Microsoft sign-in page. This is intentional — "sign out" must mean "gone until you re-authenticate," not "local cache cleared while tenant still trusts you."

**Checklist:**
- [ ] Import and use `useAuth`
- [ ] Add conditional user block in sidebar header
- [ ] Verify click-to-signout triggers Entra logout (not just local cache clear) — manual smoke once Task 21 lands
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
    - Numbered list for the Entra app registration:
      1. Create app registration in Azure portal.
      2. Add the **SPA platform** and configure redirect URIs:
         - For a deployed instance, add your public URL (matching `ENTRA_REDIRECT_URI`).
         - **(H9) For local Entra testing**, add *both* `http://localhost:5173` (Vite dev server via `just dev-ui`) and `http://localhost:8025` (Go binary direct via `just dev` / `just run`). The SPA is served from a different port depending on which command you use; listing both lets contributors test the Entra flow under either workflow without switching app registrations.
      3. Under the SPA platform, also register a **logout URL** (`postLogoutRedirectUri`) matching the SPA root — Task 13's `signOut` calls `logoutRedirect` which requires this. For deployed instances this is the same as `ENTRA_REDIRECT_URI`; for local it's the Vite and/or Go URL you added in step 2.
      4. Expose an API with scope `access_as_user` (or whatever you plan to set `ENTRA_API_SCOPE` to — the scope name must match).
      5. **Set `accessTokenAcceptedVersion: 2` in the app manifest** — called out prominently. Without this, issued tokens use the v1 issuer and validation fails.
      6. **(H13) Optional: group-based authorization.** If you plan to add group filtering later, also set `"groupMembershipClaims": "SecurityGroup"` in the app manifest and re-consent. Without this, JWTs will not carry a `groups` claim regardless of code changes. This plan does not use group claims yet (see Non-goals), but this step makes the future work a one-line code change.
      7. Copy Tenant ID and Client ID.
    - Env vars table: `AUTH_MODE`, `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_API_SCOPE`, `ENTRA_REDIRECT_URI` with descriptions.
    - First-run: "After deploying, sign in to the UI, navigate to Config → API Keys, create your first key. Give the key to your test apps — they use it as the Basic Auth password (`api:<key>`), exactly like a real Mailgun key."
  - **Troubleshooting.** Subsections:
    - "Test apps get 401s" → check API key is created, check Basic Auth format.
    - "Issuer mismatch during token validation" → `accessTokenAcceptedVersion` is not set to 2.
    - "Redirect loop on sign-in" → redirect URI in Entra app registration doesn't match `ENTRA_REDIRECT_URI`, or (for local dev) you're hitting a port that isn't in the SPA redirect URI list.
    - "Token valid but 401 with `invalid_scope`" → the user's token doesn't carry the `access_as_user` scope. Re-consent, or confirm the token is requested with the right scope.
    - "`503 Service Unavailable` on requests" → the server couldn't reach Microsoft's JWKS endpoint. Check egress firewall rules for `login.microsoftonline.com`.

**Checklist:**
- [ ] Write Authentication section in README
- [ ] Include app registration steps (SPA redirect URIs, logout URL, scope, `accessTokenAcceptedVersion`, optional group claims)
- [ ] Document both `:5173` and `:8025` redirect URIs for local Entra testing (H9)
- [ ] Include env var table
- [ ] Include first-API-key walkthrough
- [ ] Include five troubleshooting items (incl. scope failure and 503)
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
| 1  | Extend `config.Config` with Entra fields + `Validate()` | Done |
| 2  | Create `internal/auth` package with `Validator` | Done |
| 3  | Add startup validation + context in `cmd/server/main.go` | Done |
| 4  | Add `ManagedAPIKey` model + migration | Done |
| 5  | Implement managed API key CRUD handlers | Done |
| 6  | Add `managed_keys` mode to Basic-path auth (disabled-mode pathway) | Done |
| 7  | Rename `BasicAuth` → `APIAuth` with dual-auth Bearer path + entra-mode Basic override | Done |
| 8  | Create `EntraRequired` middleware (REST + WS variants) | Done |
| 9  | Create WebSocket log-scrubbing middleware | Done |
| 10 | Add `/mock/auth-config` endpoint | Not Started |
| 11 | Wire auth into `server.New()` (+ remove unused CORS, move `/mock/health`) | Not Started |
| 12 | Add `@azure/msal-browser` dependency + `auth/config.ts` | Not Started |
| 13 | Create MSAL singleton wrapper `auth/msalInstance.ts` | Not Started |
| 14 | Create `useAuth` composable | Not Started |
| 15 | Refactor `main.ts` to async bootstrap | Not Started |
| 16 | Add auth interceptor to `api/client.ts` | Not Started |
| 17 | Defer WebSocket connect + thread token into URL | Not Started |
| 18 | Add sign-in / user display / sign-out to `App.vue` | Not Started |
| 19 | Create `ApiKeysPage.vue` | Not Started |
| 20 | Register `/api-keys` route + nav link | Not Started |
| 21 | Update `README.md` with Authentication section | Not Started |
| 22 | Update `E2E_TESTING.md` with manual verification | Not Started |
| 23 | Add Playwright E2E spec for API Keys page | Not Started |
