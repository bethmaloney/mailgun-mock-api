# IPs & IP Pools

Return static/mock IP and pool data. This is a stub area — the mock doesn't manage real IPs or sending infrastructure. The goal is to accept calls that real Mailgun clients make and return plausible responses so apps that assign pools to domains or inspect IPs don't break.

---

## Overview

Mailgun provides two related API areas:

1. **IPs** (`/v3/ips`) — List and inspect dedicated IP addresses assigned to an account, and manage IP-to-domain assignments.
2. **IP Pools** (DIPPs) — Group dedicated IPs into named pools for reputation management. SDKs use `/v1/ip_pools`; the OpenAPI spec documents `/v3/ip_pools`. The mock should accept both prefixes.

### What the mock needs

- Return a configurable set of fake IPs (e.g., `127.0.0.1`, `10.0.0.1`) so apps can list/get IPs without errors.
- Support IP Pool CRUD so apps that create/manage pools during setup work correctly.
- Support domain-IP assignment so apps that link IPs or pools to domains get success responses.
- Skip production-only features: IP warmup, dynamic pools, IP bands, billing-gated IP provisioning.

---

## API Endpoints

### IPs — Account-Level

#### `GET /v3/ips` — List Account IPs

Returns all IPs for the account.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `dedicated` | boolean | No | Filter to dedicated IPs only |
| `enabled` | boolean | No | Filter to enabled IPs only |

**Response (200):**

```json
{
  "items": ["127.0.0.1", "10.0.0.1"],
  "total_count": 2,
  "assignable_to_pools": ["10.0.0.1"]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `items` | `string[]` | Array of IP address strings |
| `total_count` | `integer` | Number of IPs returned |
| `assignable_to_pools` | `string[]` | IPs eligible for pool assignment (include if pools feature is "enabled") |

**Mock behavior:** Return all IPs in the mock's IP store. Filters are optional — apply if provided. The mock starts with a default set of IPs (configurable) or an empty set.

---

#### `GET /v3/ips/{ip}` — Get IP Details

**Path Parameters:** `ip` (string) — The IP address.

**Response (200):**

```json
{
  "ip": "127.0.0.1",
  "rdns": "mock.mailgun.net",
  "dedicated": true
}
```

| Field | Type | Description |
|-------|------|-------------|
| `ip` | `string` | The IP address |
| `rdns` | `string` | Reverse DNS hostname |
| `dedicated` | `boolean` | Whether this is a dedicated IP |

**Mock behavior:** Look up the IP in the store. Return 404 if not found.

---

### IPs — Domain-Level

#### `GET /v3/domains/{domain}/ips` — List IPs for Domain

Returns IPs currently assigned to a domain.

**Response (200):** Array of IP objects (same shape as `GET /v3/ips/{ip}`).

```json
{
  "items": [
    { "ip": "127.0.0.1", "rdns": "mock.mailgun.net", "dedicated": true }
  ],
  "total_count": 1
}
```

**Mock behavior:** Return IPs linked to the domain. Empty array if none assigned.

---

#### `POST /v3/domains/{domain}/ips` — Assign IP to Domain

Assigns an IP (or links a pool) to a domain.

**Request body (form data):**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `ip` | string | Yes* | IP address to assign |
| `pool_id` | string | Yes* | Pool ID to link (alternative to `ip`) |

*One of `ip` or `pool_id` is required.

**Response (200):**

```json
{
  "message": "success"
}
```

**Mock behavior:** Add the IP to the domain's IP list, or link the pool to the domain. Return 404 if domain or IP/pool not found.

---

#### `DELETE /v3/domains/{domain}/ips/{ip}` — Remove IP from Domain

Multi-purpose endpoint — behavior depends on the `{ip}` path parameter value:

| `{ip}` value | Behavior |
|--------------|----------|
| Valid IP address | Remove that IP from the domain's pool |
| `"all"` | Remove the entire domain pool |
| `"ip_pool"` | Unlink the currently linked DIPP from the domain |

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `ip` | string | Replacement IP (mutually exclusive with `pool_id`) |
| `pool_id` | string | Replacement pool ID (mutually exclusive with `ip`) |

**Response (200):**

```json
{
  "message": "success"
}
```

**Mock behavior:** Perform the removal/unlink. The mock does not need to enforce replacement requirements — just accept the parameters and return success.

---

#### `GET /v3/ips/{ip}/domains` — List Domains for IP

Returns domains that have this IP assigned.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | integer | No | Max results per page |
| `skip` | integer | No | Number of results to skip |
| `search` | string | No | Filter by domain name substring |

**Response (200):**

```json
{
  "items": [
    { "domain": "example.com", "ips": ["127.0.0.1"] }
  ],
  "total_count": 1
}
```

**Mock behavior:** Scan domains in store and return those with this IP assigned. Support pagination via `limit`/`skip`.

---

### IP Pools — CRUD

> **Version note:** The OpenAPI spec documents these at `/v3/ip_pools`, but the Node.js and Python SDKs use `/v1/ip_pools`. The mock should accept both `/v1/ip_pools` and `/v3/ip_pools` path prefixes.

#### `GET /v3/ip_pools` — List IP Pools

**Response (200):**

```json
{
  "ip_pools": [
    {
      "pool_id": "60140bc1fee3e84dec5abeeb",
      "name": "transactional",
      "description": "Pool for transactional email",
      "ips": ["127.0.0.1"],
      "is_linked": true,
      "is_inherited": false
    }
  ],
  "message": "success"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `ip_pools` | `object[]` | Array of pool objects |
| `ip_pools[].pool_id` | `string` | Unique pool identifier |
| `ip_pools[].name` | `string` | Pool name |
| `ip_pools[].description` | `string` | Pool description |
| `ip_pools[].ips` | `string[]` | IPs in the pool |
| `ip_pools[].is_linked` | `boolean` | Whether any domains reference this pool |
| `ip_pools[].is_inherited` | `boolean` | Whether inherited from parent account |
| `message` | `string` | `"success"` |

---

#### `GET /v3/ip_pools/{pool_id}` — Get Pool Details

**Response (200):**

```json
{
  "pool_id": "60140bc1fee3e84dec5abeeb",
  "name": "transactional",
  "description": "Pool for transactional email",
  "ips": ["127.0.0.1"],
  "is_linked": true,
  "linked_domains": [
    { "id": "abc123", "name": "example.com" }
  ],
  "message": "success"
}
```

Additional field vs. list endpoint:

| Field | Type | Description |
|-------|------|-------------|
| `linked_domains` | `object[]` | Domains linked to this pool (only in detail view) |

---

#### `POST /v3/ip_pools` — Create IP Pool

**Request body (form data):**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Pool name |
| `description` | string | Yes | Pool description |
| `ip` | string | No | IP to add (may be specified multiple times) |

**Response (200):**

```json
{
  "message": "success",
  "pool_id": "60140bc1fee3e84dec5abeeb"
}
```

**Mock behavior:** Generate a pool ID (hex string, 24 chars like a MongoDB ObjectId), store the pool, return the ID.

---

#### `PATCH /v3/ip_pools/{pool_id}` — Update IP Pool

**Request body (form data):**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | No | New name |
| `description` | string | No | New description |
| `add_ip` | string | No | IP to add (repeatable) |
| `remove_ip` | string | No | IP to remove (repeatable) |
| `link_domain` | string | No | Domain ID to link (repeatable) |
| `unlink_domain` | string | No | Domain ID to unlink (repeatable) |

**Response (200):**

```json
{
  "message": "success"
}
```

**Mock behavior:** Apply updates to the pool in the store. The Python SDK uses `add_ip` field name; Node.js uses `ips` array in create but the PATCH endpoint accepts `add_ip`/`remove_ip`. Support both.

---

#### `DELETE /v3/ip_pools/{pool_id}` — Delete IP Pool

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `ip` | string | Replacement IP for domains using this pool |
| `pool_id` | string | Replacement pool ID for domains using this pool |

The special value `"shared"` can be used for the replacement IP.

**Response (200):**

```json
{
  "message": "started"
}
```

Note: The response message is `"started"` (not `"success"`) because real Mailgun processes the deletion asynchronously.

**Mock behavior:** Delete the pool immediately. Accept but ignore replacement parameters. Return `"started"` for API compatibility.

---

### IP Pool — IP Management

#### `PUT /v3/ip_pools/{pool_id}/ips/{ip}` — Add IP to Pool

**Response (200):**

```json
{
  "message": "started"
}
```

---

#### `DELETE /v3/ip_pools/{pool_id}/ips/{ip}` — Remove IP from Pool

**Response (200):**

```json
{
  "message": "started"
}
```

---

#### `POST /v3/ip_pools/{pool_id}/ips.json` — Add Multiple IPs to Pool

**Request body (JSON):**

```json
{
  "ips": ["10.0.0.1", "10.0.0.2"]
}
```

**Response (200):**

```json
{
  "message": "started"
}
```

---

### IP Pool — Domain Listing

#### `GET /v3/ip_pools/{pool_id}/domains` — List Domains Linked to Pool

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 10 | Max results (10–500) |
| `page` | string | — | Encoded page token from previous response |

**Response (200):**

```json
{
  "domains": [
    { "id": "abc123", "name": "example.com" }
  ],
  "paging": {
    "first": "https://...",
    "next": "https://..."
  }
}
```

---

## Stub-Only Endpoints (Accept and Return 200)

These endpoints exist in the real API but are production-only concerns. The mock should accept calls and return success responses with empty/minimal data.

### IP Warmup

| Method | Path | Mock Response |
|--------|------|---------------|
| `GET` | `/v3/ip_warmups` | `{ "items": [], "paging": {} }` |
| `GET` | `/v3/ip_warmups/{addr}` | 404 (no warmup plans in mock) |
| `POST` | `/v3/ip_warmups/{addr}` | `{ "message": "success" }` |
| `DELETE` | `/v3/ip_warmups/{addr}` | `{ "message": "success" }` |

### IP Provisioning

| Method | Path | Mock Response |
|--------|------|---------------|
| `GET` | `/v3/ips/request/new` | `{ "allowed": { "dedicated": 0, "shared": 0 } }` |
| `POST` | `/v3/ips/request/new` | `{ "message": "success" }` |

### IP Bands (Deprecated)

| Method | Path | Mock Response |
|--------|------|---------------|
| `POST` | `/v3/ips/{addr}/ip_band` | `{ "message": "success" }` |

### Dynamic IP Pools

| Method | Path | Mock Response |
|--------|------|---------------|
| `POST` | `/v3/dynamic_pools/{pool_name}/{ip}` | `{ "message": "success" }` |

---

## Mock Data Model

### IP Record

```typescript
interface MockIP {
  ip: string;           // e.g., "127.0.0.1"
  rdns: string;         // e.g., "mock.mailgun.net"
  dedicated: boolean;   // always true for mock IPs
}
```

### IP Pool Record

```typescript
interface MockIPPool {
  pool_id: string;      // generated 24-char hex string
  name: string;
  description: string;
  ips: string[];         // IP addresses in this pool
  is_linked: boolean;    // computed: true if any domain references this pool
  is_inherited: boolean; // always false in mock (no parent accounts)
}
```

### Domain-IP Association

Each domain in the mock can have:
- A list of assigned IPs (direct assignment)
- An optional linked pool ID (pool assignment)

These are stored as part of the domain record (see domains.md).

---

## Integration Points

### With Messages (`o:ip-pool` parameter)

When sending a message, Mailgun accepts an `o:ip-pool` parameter to select which IP pool to use. The mock should:
- Accept the parameter without error
- Optionally validate that the pool exists (return 400 if not found)
- Store the pool reference on the message record for inspection

### With Domains

- `POST /v3/domains` and domain updates can include pool assignment
- The domain detail response includes IP information
- Domain-IP endpoints are documented above

### With Subaccounts

- The v5 subaccount DIPP delegation endpoints (`/v5/accounts/subaccounts/ip_pools/...`) exist but are complex and unlikely to be needed in mock scenarios. Skip entirely unless subaccount support demands it.

---

## Default Mock Configuration

The mock should start with a reasonable default state:

```json
{
  "ips": [
    { "ip": "127.0.0.1", "rdns": "mock-1.mailgun.net", "dedicated": true },
    { "ip": "10.0.0.1", "rdns": "mock-2.mailgun.net", "dedicated": true }
  ],
  "ip_pools": []
}
```

Users can add/remove IPs and create pools via the API or through mock configuration.

---

## SDK Compatibility Notes

| SDK | IPs | IP Pools | Notes |
|-----|-----|----------|-------|
| **Node.js** (`mailgun.js`) | `client.ips.list()`, `client.ips.get(ip)` | `client.ip_pools.list()`, `.create()`, `.update()`, `.delete()` | Pools use `/v1/ip_pools` path prefix |
| **Python** (`mailgun-python`) | `client.ips.get()` | `client.ippools.get()`, `.create()`, `.patch()`, `.delete()` | Also uses `/v1/ip_pools`; domain-IP ops via `client.domains_ips` |
| **Ruby** (`mailgun-ruby`) | No dedicated client | No dedicated client | Users call raw HTTP methods with manual paths |
| **Go** (`mailgun-go`) | `ListIPs()`, `GetIP()`, `ListDomainIPs()`, `AddDomainIP()`, `DeleteDomainIP()` | No pool methods | IPs only, no pool support |

**Version prefix discrepancy:** The OpenAPI spec documents pools at `/v3/ip_pools`, but Node.js and Python SDKs use `/v1/ip_pools`. The mock must accept both path prefixes to ensure compatibility.

---

## Test Scenarios

1. **List IPs** — Returns default mock IPs; respects `dedicated` filter
2. **Get IP detail** — Returns IP info; 404 for unknown IP
3. **Create pool** — Returns generated pool_id; pool appears in list
4. **Update pool** — Add/remove IPs; rename pool
5. **Delete pool** — Pool removed from list; `"started"` message returned
6. **Assign IP to domain** — IP appears in domain's IP list
7. **Link pool to domain** — Domain's `ip_pool` field set
8. **Unlink pool from domain** — `DELETE /v3/domains/{domain}/ips/ip_pool` clears the link
9. **Message with `o:ip-pool`** — Message accepted; pool name stored on message record
10. **SDK round-trip** — Node.js `client.ip_pools.list()` → `create()` → `update()` → `delete()` lifecycle works

---

## References

- **OpenAPI spec:** `mailgun.yaml` — paths under `/v3/ips`, `/v3/ip_pools`, `/v3/ip_warmups`, `/v3/dynamic_pools`, `/v3/domains/{name}/ips`
- **IPs API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/ips
- **IP Pools API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/ip-pools
- **Node.js SDK:** `lib/Classes/IPs.ts`, `lib/Classes/IPPools.ts` — https://github.com/mailgun/mailgun.js
- **Python SDK:** `mailgun/handlers/ips_handler.py`, `mailgun/handlers/ip_pools_handler.py` — https://github.com/mailgun/mailgun-python
- **Go SDK:** `ips.go` — https://github.com/mailgun/mailgun-go
- **IP Pools blog post:** https://www.mailgun.com/blog/product/mailgun-ip-pools-domain-keys/
