# Credentials & Keys

Manage API keys, SMTP credentials, and IP allowlists. This is a stub area — the mock needs to accept authentication and provide CRUD for keys/credentials so apps that manage them programmatically don't break. RBAC enforcement is optional.

---

## Overview

Mailgun has three distinct credential/key systems:

1. **API Keys** (`/v1/keys`) — Account-level or domain-scoped keys used for REST API authentication via HTTP Basic Auth. Keys have roles (RBAC) that determine endpoint access.
2. **SMTP Credentials** (`/v3/domains/{domain}/credentials`) — Per-domain username/password pairs used for SMTP relay authentication. Separate from API keys.
3. **IP Allowlist** (`/v2/ip_whitelist`) — Account-level list of IP addresses permitted to access the Mailgun API. Security/access-control feature.

Additionally, there are **DKIM keys** (`/v1/dkim/keys`, `/v4/domains/{domain}/keys`) for email signing — these are production-only and documented in the scratchpad under Domains research. The mock should stub them if needed but they are out of scope for this plan.

### Authentication model

All Mailgun API requests use HTTP Basic Auth:
- **Username:** literal string `"api"`
- **Password:** the API key value
- **Header:** `Authorization: Basic base64("api:<API_KEY>")`

The mock should accept this format. By default, accept any non-empty key. Optionally support a configurable set of valid keys with roles.

### What the mock needs

- Accept HTTP Basic Auth on all endpoints (validate format, optionally validate key existence)
- SMTP Credential CRUD so apps that provision SMTP users during setup work correctly
- API Key CRUD so apps that manage keys programmatically get correct responses
- IP Allowlist CRUD (simple stub — accept calls and return stored data)
- Optionally enforce RBAC roles on API keys (can be a future enhancement)
- Password validation rules (5–32 characters for SMTP credentials)

---

## API Endpoints

### SMTP Credentials — Domain-Scoped

SMTP credentials are always scoped to a specific domain. The `login` field is a local-part (e.g., `alice`) that becomes `alice@domain.com` for SMTP authentication.

#### `GET /v3/domains/{domain_name}/credentials` — List Credentials

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `skip` | integer | No | 0 | Number of records to skip (offset pagination) |
| `limit` | integer | No | 100 | Max records to return |

**Response (200):**

```json
{
  "total_count": 2,
  "items": [
    {
      "created_at": "Wed, 08 Mar 2023 23:34:57 +0000",
      "login": "alice@example.com",
      "mailbox": "alice@example.com",
      "size_bytes": null
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `total_count` | integer | Total number of credentials for the domain |
| `items[].created_at` | string | RFC 2822 format with timezone |
| `items[].login` | string | SMTP login (email format: `localpart@domain`) |
| `items[].mailbox` | string | Same as login |
| `items[].size_bytes` | string\|null | Mailbox size — always `null` in mock |

**Pagination:** Offset-based using `skip`/`limit` (not cursor-based).

**Mock behavior:** Return all credentials for the domain, applying skip/limit if provided.

---

#### `POST /v3/domains/{domain_name}/credentials` — Create Credential

**Content-Type:** `multipart/form-data`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `login` | string | Yes | SMTP username (local-part or full email) |
| `password` | string | Yes | SMTP password (5–32 characters) |
| `mailbox` | string | No | Alternative to `login` |
| `system` | boolean | No | System account flag |

**Response (200):**

```json
{
  "message": "Created 1 credentials pair(s)"
}
```

The response may optionally include `note` and `credentials` fields if Mailgun generates a password (when `password` is omitted). For the mock, always require `password` and return the simple message.

**Validation:**
- Password must be 5–32 characters
- Login should be unique within the domain
- If `login` is a local-part (no `@`), the mock should store it as `login@domain_name`

**Mock behavior:** Store the credential. Do not hash the password (useful for test inspection). Set `created_at` to current time in RFC 2822 format. Set `size_bytes` to `null`.

---

#### `PUT /v3/domains/{domain_name}/credentials/{spec}` — Update Credential Password

**Path Parameters:** `spec` (string) — The login local-part (e.g., `alice`, not `alice@example.com`).

**Content-Type:** `multipart/form-data`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `password` | string | Yes | New password (5–32 characters) |

**Response (200):**

```json
{
  "message": "Password changed"
}
```

**Errors:**
- 404 if credential not found

**Mock behavior:** Update the stored password for the matching credential.

---

#### `DELETE /v3/domains/{domain_name}/credentials/{spec}` — Delete Credential

**Path Parameters:** `spec` (string) — The login local-part.

**Response (200):**

```json
{
  "message": "Credentials have been deleted",
  "spec": "alice@example.com"
}
```

**Mock behavior:** Remove the credential from storage. Return the full email in the `spec` field.

---

#### `DELETE /v3/domains/{domain_name}/credentials` — Delete All Domain Credentials

**Response (200):**

```json
{
  "message": "All domain credentials have been deleted",
  "count": 3
}
```

**Mock behavior:** Remove all credentials for the domain. Return the count of deleted credentials.

---

### API Keys — Account-Level

API keys are managed at `/v1/keys`. Keys have a `kind` (scope) and `role` (permissions).

#### Key Kinds

| Kind | Description |
|------|-------------|
| `user` | Account-level key (default). Works across all domains and endpoints (subject to role). |
| `domain` | Domain-specific key. Typically paired with `sending` role to restrict to message sending only. |
| `web` | Web-facing key. |

#### Key Roles (RBAC)

| Role | Description |
|------|-------------|
| `admin` | Full read/write access to all endpoints. Default on Free/Basic plans. |
| `basic` | Read-only access to data/metrics. No suppression access. (Also called "Analyst" in UI.) |
| `developer` | Full read/write to technical endpoints. |
| `sending` | Restricted to `POST /messages` and `POST /messages.mime` on the key's domain only. Used with `domain` kind. |

**RBAC permissions matrix (for reference — enforcement is optional in mock):**

| Endpoint Category | admin | basic | developer | sending |
|---|---|---|---|---|
| Domains | R/W | R | R/W | — |
| Messages | R/W | R | R/W | W (own domain) |
| Webhooks | R/W | R | R/W | — |
| Logs/Events | R/W | R | R/W | — |
| Tags | R/W | R | R/W | — |
| Metrics | R/W | R | R/W | — |
| Suppressions | R/W | — | R/W | — |

---

#### `GET /v1/keys` — List API Keys

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `domain_name` | string | No | Filter by associated domain (for domain keys) |
| `kind` | string | No | Filter by key kind: `domain`, `user`, `web` |

**Response (200):**

```json
{
  "total_count": 2,
  "items": [
    {
      "id": "f2153fd0-f1277777",
      "description": "Production sending key",
      "kind": "domain",
      "role": "sending",
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T10:30:00Z",
      "expires_at": null,
      "is_disabled": false,
      "disabled_reason": null,
      "domain_name": "example.com",
      "requestor": null,
      "user_name": null
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `total_count` | integer | Total keys matching filters |
| `items[].id` | string | Unique key ID (UUID-like) |
| `items[].description` | string | Human-readable description |
| `items[].kind` | string | `domain`, `user`, or `web` |
| `items[].role` | string | `admin`, `basic`, `sending`, or `developer` |
| `items[].created_at` | string | ISO 8601 UTC |
| `items[].updated_at` | string | ISO 8601 UTC |
| `items[].expires_at` | string\|null | ISO 8601 UTC, or null if no expiration |
| `items[].is_disabled` | boolean | Whether the key is disabled |
| `items[].disabled_reason` | string\|null | Reason for disablement |
| `items[].domain_name` | string\|null | Associated domain (for domain keys) |
| `items[].requestor` | string\|null | Associated email address |
| `items[].user_name` | string\|null | Associated user name |

**Important:** The `secret` field is NEVER returned in list responses.

**Mock behavior:** Return all stored keys, applying filters if provided.

---

#### `POST /v1/keys` — Create API Key

**Content-Type:** `multipart/form-data`

**Request Body:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `role` | string | Yes | — | Key role: `admin`, `basic`, `sending`, `developer` |
| `description` | string | No | — | Human-readable description |
| `kind` | string | No | `user` | Key type: `domain`, `user`, `web` |
| `domain_name` | string | No | — | Domain for `domain` kind keys |
| `expiration` | integer | No | 0 | Lifetime in seconds (0 = never expires) |
| `user_id` | string | No | — | Associated user ID |
| `user_name` | string | No | — | Associated user name |
| `email` | string | No | — | Associated email |

**Response (200):**

```json
{
  "message": "great success",
  "key": {
    "id": "f2153fd0-f1277777",
    "description": "Production sending key",
    "kind": "domain",
    "role": "sending",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z",
    "expires_at": null,
    "is_disabled": false,
    "domain_name": "example.com",
    "requestor": null,
    "user_name": null,
    "secret": "key-abc123def456..."
  }
}
```

**Important:** The `secret` field is returned ONLY on creation. It cannot be retrieved later.

**Mock behavior:**
- Generate a unique `id` (UUID)
- Generate a `secret` prefixed with `key-` followed by random hex characters
- Store the key and return with `secret` included
- If `expiration` > 0, compute `expires_at` from current time + expiration seconds

---

#### `DELETE /v1/keys/{key_id}` — Delete API Key

**Path Parameters:** `key_id` (string) — The key ID.

**Response (200):**

```json
{
  "message": "key deleted"
}
```

**Errors:**
- 404 if key not found

**Mock behavior:** Remove the key from storage.

---

#### `POST /v1/keys/public` — Regenerate Public Key

Regenerates the account's public validation key (used for client-side email validation).

**Response (200):**

```json
{
  "key": "pubkey-abc123...",
  "message": "public key regenerated"
}
```

**Mock behavior:** Generate a new `pubkey-` prefixed key, store as the account's public key, return it.

---

### IP Allowlist — Account-Level

The IP Allowlist restricts which IP addresses can access the Mailgun API. This is a security feature — for the mock, it's a simple CRUD store. The mock should NOT enforce the allowlist (that would block the developer's own requests).

**Note:** The API path uses the legacy name `ip_whitelist` but the feature is called "IP Allowlist" in current docs.

#### `GET /v2/ip_whitelist` — List Allowlist Entries

**Response (200):**

```json
{
  "addresses": [
    {
      "ip_address": "10.11.11.111",
      "description": "OnPrem Server"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `addresses` | array | Array of allowlist entries |
| `addresses[].ip_address` | string | IP address or CIDR |
| `addresses[].description` | string | Human-readable description |

**Mock behavior:** Return all stored allowlist entries.

---

#### `POST /v2/ip_whitelist` — Add Allowlist Entry

**Content-Type:** `multipart/form-data`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | Yes | IP address or CIDR to allowlist |
| `description` | string | No | Description (defaults to empty string) |

**Response (200):** Returns the full allowlist (same shape as GET).

**Errors:**
- 400 `"Invalid IP Address or CIDR"` if address format is invalid

**Mock behavior:** Validate the IP/CIDR format, store the entry, return the full allowlist.

---

#### `PUT /v2/ip_whitelist` — Update Allowlist Entry Description

**Content-Type:** `multipart/form-data`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | Yes | IP address to update |
| `description` | string | No | New description |

**Response (200):** Returns the full allowlist.

**Errors:**
- 400 `"IP not found"` if address not in allowlist

**Mock behavior:** Find the entry by `address`, update its `description`, return the full allowlist.

---

#### `DELETE /v2/ip_whitelist` — Delete Allowlist Entry

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `address` | string | Yes | IP address to remove |

**Response (200):** Returns the full allowlist (without the deleted entry).

**Errors:**
- 400 if address not found

**Mock behavior:** Remove the entry, return the remaining allowlist.

---

## Mock Authentication Strategy

The mock needs a configurable authentication model:

### Default mode: Accept any key

Accept any non-empty `Authorization: Basic ...` header where the username is `api`. This is the simplest mode and sufficient for most testing.

### Configurable mode: Key validation

Optionally support a set of preconfigured keys (via mock config or the Keys API). When enabled:
- Validate that the provided key exists in the key store
- Check that the key is not disabled (`is_disabled: false`)
- Check that the key has not expired (`expires_at` is null or in the future)
- Optionally enforce RBAC role permissions

### Domain Sending Key enforcement

When a request uses a domain sending key (`kind: "domain"`, `role: "sending"`):
- Only allow `POST /v3/{domain_name}/messages` and `POST /v3/{domain_name}/messages.mime`
- The `{domain_name}` must match the key's `domain_name`
- Reject all other endpoints with 403

### Webhook signature verification

The mock should provide a configurable webhook signing key (or generate one on startup). This key is used to:
- Sign outgoing webhook payloads: `HMAC-SHA256(signing_key, timestamp + token)`
- Allow clients to verify webhook authenticity using the same algorithm

The signing key should be retrievable via the mock's config/UI so clients can use it for verification.

---

## SDK Patterns

### JavaScript (mailgun.js)
```javascript
// Initialize
const mg = mailgun.client({ username: 'api', key: 'key-xxx' });

// SMTP Credentials
mg.domains.domainCredentials.list('example.com', { limit: 100, skip: 0 });
mg.domains.domainCredentials.create('example.com', { login: 'alice', password: 'secret' });
mg.domains.domainCredentials.update('example.com', 'alice', { password: 'newpass' });
mg.domains.domainCredentials.destroy('example.com', 'alice');
```

### Ruby (mailgun-ruby)
```ruby
mg_client = Mailgun::Client.new('key-xxx')
domains = Mailgun::Domains.new(mg_client)

# SMTP Credentials (no list method in Ruby SDK)
domains.create_smtp_credentials('example.com', login: 'alice', password: 'secret')
domains.update_smtp_credentials('example.com', 'alice', password: 'newpass')
domains.delete_smtp_credentials('example.com', 'alice')
```

### Python (mailgun-python)
```python
client = Client(auth=("api", "key-xxx"))

# SMTP Credentials
client.domains_credentials.get(domain='example.com')
client.domains_credentials.create(domain='example.com', data={'login': 'alice@example.com', 'password': 'secret'})
client.domains_credentials.put(domain='example.com', login='alice', data={'password': 'newpass'})
client.domains_credentials.delete(domain='example.com', login='alice')

# API Keys
client.keys.get(filters={'kind': 'domain'})
client.keys.create(data={'role': 'sending', 'domain_name': 'example.com', 'kind': 'domain'})
client.keys.delete(key_id='f2153fd0-f1277777')
```

---

## Data Model

### SMTP Credential (stored per domain)

```
{
  login: string,          // full email: "alice@example.com"
  password: string,       // stored in plain text for test inspection
  created_at: string,     // RFC 2822: "Wed, 08 Mar 2023 23:34:57 +0000"
  size_bytes: null         // always null in mock
}
```

### API Key (stored account-level)

```
{
  id: string,              // UUID
  secret: string,          // "key-" + hex (stored for auth validation)
  description: string,
  kind: "user" | "domain" | "web",
  role: "admin" | "basic" | "sending" | "developer",
  created_at: string,      // ISO 8601 UTC
  updated_at: string,      // ISO 8601 UTC
  expires_at: string|null, // ISO 8601 UTC or null
  is_disabled: boolean,
  disabled_reason: string|null,
  domain_name: string|null,
  requestor: string|null,
  user_name: string|null
}
```

### IP Allowlist Entry (stored account-level)

```
{
  ip_address: string,      // IP or CIDR
  description: string      // defaults to ""
}
```

---

## Casing Conventions

| Resource | Timestamp format | Timestamp field |
|----------|-----------------|-----------------|
| SMTP Credentials | RFC 2822 (`Wed, 08 Mar 2023 23:34:57 +0000`) | `created_at` (snake_case) |
| API Keys | ISO 8601 UTC (`2024-01-15T10:30:00Z`) | `created_at`, `updated_at`, `expires_at` (snake_case) |
| IP Allowlist | N/A | N/A |

---

## Integration Points

- **All endpoints:** Authentication via HTTP Basic Auth — the middleware that validates keys is a cross-cutting concern used by every other API area.
- **Messages:** Domain sending keys restrict access to message endpoints only.
- **Webhooks:** The webhook signing key is used to sign outgoing webhook payloads (documented in webhooks.md).
- **Subaccounts:** The `X-Mailgun-On-Behalf-Of` header selects a subaccount context. API keys may be associated with specific subaccounts (future — see subaccounts.md).

---

## DKIM Keys — Stub Only

DKIM key management is a production-only concern (actual cryptographic key generation and rotation). The mock should accept these calls and return success responses but doesn't need real key material.

**Endpoints to stub:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/dkim/keys` | List all DKIM keys |
| `POST` | `/v1/dkim/keys` | Create a DKIM key |
| `DELETE` | `/v1/dkim/keys` | Delete a DKIM key |
| `GET` | `/v4/domains/{domain}/keys` | List domain keys |
| `PUT` | `/v4/domains/{domain}/keys/{selector}/activate` | Activate key |
| `PUT` | `/v4/domains/{domain}/keys/{selector}/deactivate` | Deactivate key |
| `PUT` | `/v3/domains/{name}/dkim_authority` | Update DKIM authority |
| `PUT` | `/v3/domains/{name}/dkim_selector` | Update DKIM selector |

---

## Test Scenarios

1. **SMTP credential lifecycle:** Create → List → Update password → Delete → Verify gone
2. **Bulk credential delete:** Create multiple credentials → Delete all → Verify count
3. **Password validation:** Attempt create with < 5 char password → expect error
4. **API key lifecycle:** Create → List → Delete → Verify gone
5. **Key secret visibility:** Create key → verify `secret` in response → List keys → verify `secret` absent
6. **Domain sending key restriction:** Create domain key with `sending` role → use it to send message (success) → use it to list domains (403)
7. **Key filtering:** Create keys with different kinds → list with `kind` filter → verify filtering
8. **IP allowlist CRUD:** Add entry → List → Update description → Delete → Verify empty
9. **Invalid IP rejection:** Add invalid IP address → expect 400 error
10. **Auth rejection:** Send request with empty/missing auth → expect 401

---

## References

- **OpenAPI spec:** `mailgun.yaml` — SMTP credentials at `/v3/domains/{domain_name}/credentials`, API keys at `/v1/keys`, IP allowlist at `/v2/ip_whitelist`
- **Credentials API:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/credentials
- **Keys API:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/keys
- **API Keys help:** https://help.mailgun.com/hc/en-us/articles/203380100-Where-can-I-find-my-API-keys-and-SMTP-credentials
- **RBAC roles:** https://help.mailgun.com/hc/en-us/articles/26016288026907-API-Key-Roles
- **RBAC management:** https://documentation.mailgun.com/docs/mailgun/user-manual/api-key-mgmt/rbac-mgmt
- **JS SDK credentials:** https://github.com/mailgun/mailgun.js (`/lib/Classes/Domains/domainsCredentials.ts`)
- **Ruby SDK credentials:** https://github.com/mailgun/mailgun-ruby (`/lib/mailgun/domains/domains.rb`)
- **Python SDK keys/credentials:** https://github.com/mailgun/mailgun-python
