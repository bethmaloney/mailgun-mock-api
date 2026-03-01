# Subaccounts

Manage subaccounts under a primary Mailgun account. Subaccounts provide isolated tenancy — each subaccount has its own domains, API keys, SMTP credentials, and sending stats. The primary account manages subaccounts and can act on their behalf via the `X-Mailgun-On-Behalf-Of` header.

This is a stub area — the mock needs basic CRUD and the on-behalf-of header mechanism so apps that use multi-tenancy don't break. Feature controls and sending limits are optional extras.

---

## Overview

Mailgun subaccounts are child accounts under a primary (parent) account. Key characteristics:

1. **Isolated resources** — Each subaccount has wholly separate domains, API keys, SMTP credentials, users, settings, and statistics/logs.
2. **Shared plan** — Subaccounts share the parent account's plan, usage allocations, and billing.
3. **Parent-managed** — Only the parent account can create, enable, disable, or delete subaccounts.
4. **On-behalf-of header** — The parent account can make any API call scoped to a subaccount by setting the `X-Mailgun-On-Behalf-Of` header with the subaccount ID.
5. **v5 API** — All subaccount endpoints use `/v5/accounts/subaccounts`, unlike most Mailgun endpoints which use `/v3` or `/v4`.
6. **Enterprise feature** — Available only to contract/enterprise customers in production Mailgun.

### Subaccount statuses

| Status | Description |
|--------|-------------|
| `open` | Active, can send email and use all enabled features |
| `disabled` | Suspended — cannot send, but data is preserved |
| `closed` | Permanently closed — data may be deleted (not recoverable) |

### What the mock needs

- Subaccount CRUD (list, get, create, disable, enable, delete)
- `X-Mailgun-On-Behalf-Of` header support on all endpoints — scopes operations to a subaccount's isolated resources
- Subaccount ID generation (24-character hex strings, MongoDB ObjectID format)
- Timestamps in RFC 1123 / HTTP-date format (e.g., `"Wed, 06 Nov 2024 19:48:29 GMT"`)
- Sending limit CRUD (optional — stub with stored values)
- Feature flag management (optional — stub with stored values)

### What the mock can skip

- Actual resource isolation enforcement (the mock can store subaccount_id on resources and filter by it, but doesn't need strict ACL)
- IP pool delegation to subaccounts (`/v5/accounts/subaccounts/{id}/ip_pool`, `/v5/accounts/subaccounts/{id}/ip`)
- Child account limit enforcement (parent can't exceed allotted child count)
- Billing integration

---

## Data Model

### Subaccount object

```json
{
  "id": "646d00a1b32c35364a2ad34f",
  "name": "My subaccount",
  "status": "open",
  "created_at": "Wed, 06 Nov 2024 19:48:29 GMT",
  "updated_at": "Wed, 28 May 2025 20:03:05 GMT",
  "features": {
    "email_preview": { "enabled": false },
    "inbox_placement": { "enabled": false },
    "sending": { "enabled": true },
    "validations": { "enabled": false },
    "validations_bulk": { "enabled": false }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | 24-character hex string (MongoDB ObjectID format) |
| `name` | string | Human-readable name |
| `status` | string | One of: `open`, `disabled`, `closed` |
| `created_at` | string | RFC 1123 timestamp in GMT |
| `updated_at` | string | RFC 1123 timestamp in GMT |
| `features` | object | Feature flags (optional in responses — Ruby SDK omits this) |

**Note on `features`:** The OpenAPI spec defines `FeaturesResponse` as `type: object` with `additionalProperties: true`, meaning the shape is loosely defined. The known feature keys are `email_preview`, `inbox_placement`, `sending`, `validations`, and `validations_bulk`, each with an `{ enabled: boolean }` value. The mock should return all five features on create/get, defaulting `sending` to enabled and the rest to disabled.

### Response wrappers

- **Single subaccount:** `{ "subaccount": { ... } }`
- **List of subaccounts:** `{ "subaccounts": [ ... ], "total": <number> }`
- **Success:** `{ "success": true }`
- **Error:** `{ "message": "<error description>" }`
- **Delete:** `{ "message": "Subaccount successfully deleted" }`

---

## API Endpoints

### `GET /v5/accounts/subaccounts` — List Subaccounts

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `sort` | string | No | — | `"asc"` or `"desc"` (sorts by name) |
| `filter` | string | No | — | Filter by subaccount name (partial match) |
| `limit` | integer | No | 10 | Number of results to return (1–1000) |
| `skip` | integer | No | 0 | Number of results to skip (offset pagination) |
| `enabled` | boolean | No | — | `true` = only enabled (`open`), `false` = only disabled |
| `closed` | boolean | No | — | `true` = include closed, `false` = exclude closed |

**Pagination:** Uses `skip`/`limit` offset-based pagination (NOT cursor-based like suppressions).

**Response (200):**

```json
{
  "subaccounts": [
    {
      "id": "646d00a1b32c35364a2ad34f",
      "name": "My subaccount",
      "status": "open",
      "created_at": "Wed, 06 Nov 2024 19:48:29 GMT",
      "updated_at": "Wed, 28 May 2025 20:03:05 GMT"
    }
  ],
  "total": 1
}
```

**Mock behavior:** Return all stored subaccounts, applying `filter`, `enabled`, `closed`, `sort`, `skip`, and `limit` as in-memory filters.

---

### `GET /v5/accounts/subaccounts/{subaccount_id}` — Get Subaccount

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Response (200):**

```json
{
  "subaccount": {
    "id": "646d00a1b32c35364a2ad34f",
    "name": "My subaccount",
    "status": "open",
    "created_at": "Wed, 06 Nov 2024 19:48:29 GMT",
    "updated_at": "Wed, 28 May 2025 20:03:05 GMT",
    "features": {
      "sending": { "enabled": true },
      "email_preview": { "enabled": false },
      "inbox_placement": { "enabled": false },
      "validations": { "enabled": false },
      "validations_bulk": { "enabled": false }
    }
  }
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 404 | `{ "message": "Not Found" }` | Subaccount ID doesn't exist |

---

### `POST /v5/accounts/subaccounts` — Create Subaccount

**Request body:** `application/x-www-form-urlencoded` or `multipart/form-data`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Name for the new subaccount |

**Note:** The OpenAPI spec defines `name` as a query parameter, but all SDKs send it as form data. The mock should accept both.

**Response (200):**

```json
{
  "subaccount": {
    "id": "646d00a1b32c35364a2ad34f",
    "name": "My subaccount",
    "status": "open",
    "created_at": "Wed, 06 Nov 2024 19:48:29 GMT",
    "updated_at": "Wed, 06 Nov 2024 19:48:29 GMT",
    "features": {
      "sending": { "enabled": true },
      "email_preview": { "enabled": false },
      "inbox_placement": { "enabled": false },
      "validations": { "enabled": false },
      "validations_bulk": { "enabled": false }
    }
  }
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "Bad request" }` | Missing `name` parameter |

**Mock behavior:**
- Generate a 24-character hex ID
- Set `status` to `"open"`
- Set `created_at` and `updated_at` to current time in RFC 1123 format
- Initialize features with `sending: true`, all others `false`

---

### `POST /v5/accounts/subaccounts/{subaccount_id}/disable` — Disable Subaccount

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Query Parameters (optional):**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `reason` | string | No | Reason for disabling |
| `note` | string | No | Additional note |

**Response (200):**

```json
{
  "subaccount": {
    "id": "646d00a1b32c35364a2ad34f",
    "name": "My subaccount",
    "status": "disabled",
    "created_at": "Wed, 06 Nov 2024 19:48:29 GMT",
    "updated_at": "Wed, 28 May 2025 20:03:05 GMT"
  }
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "subaccount is already disabled" }` | Already disabled |
| 404 | `{ "message": "Not Found" }` | Subaccount ID doesn't exist |

**Mock behavior:** Set `status` to `"disabled"`, update `updated_at`. Store `reason` and `note` if provided (not returned in responses, but may be useful for mock inspection).

---

### `POST /v5/accounts/subaccounts/{subaccount_id}/enable` — Enable Subaccount

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Response (200):**

```json
{
  "subaccount": {
    "id": "646d00a1b32c35364a2ad34f",
    "name": "My subaccount",
    "status": "open",
    "created_at": "Wed, 06 Nov 2024 19:48:29 GMT",
    "updated_at": "Wed, 28 May 2025 20:03:05 GMT"
  }
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "Parent account has reached its allotted child limit" }` | Child limit exceeded (mock can skip this) |
| 404 | `{ "message": "Not Found" }` | Subaccount ID doesn't exist |

**Mock behavior:** Set `status` to `"open"`, update `updated_at`.

---

### `DELETE /v5/accounts/subaccounts` — Delete Subaccount

**Note:** This endpoint uses the `X-Mailgun-On-Behalf-Of` **header** to identify the subaccount to delete — the subaccount ID is NOT in the URL path.

**Required Headers:**

| Header | Type | Required | Description |
|--------|------|----------|-------------|
| `X-Mailgun-On-Behalf-Of` | string | Yes | The subaccount ID to delete |

**Response (200):**

```json
{
  "message": "Subaccount successfully deleted"
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "Bad request" }` | Missing header or invalid ID |
| 403 | `{ "message": "Forbidden" }` | Not authorized |

**Mock behavior:** Remove the subaccount from storage. Optionally clean up associated resources (domains, credentials, etc.) or leave them orphaned.

---

### `GET /v5/accounts/subaccounts/{subaccount_id}/limit/custom/monthly` — Get Sending Limit

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Response (200):**

```json
{
  "limit": 10000,
  "current": 0,
  "period": "1m"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `limit` | number | The configured monthly sending limit |
| `current` | number | Messages sent in the current period |
| `period` | string | Timeframe code: `"1m"` (1 month), `"1d"` (1 day), `"1h"` (1 hour) |

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "Not a subaccount" }` | ID is not a subaccount |
| 403 | `{ "message": "Forbidden - user does not have permission to view this resource" }` | Unauthorized |
| 404 | `{ "message": "No threshold for account" }` | No custom limit set |

**Mock behavior:** Return stored limit values. Return 404 if no custom limit has been set for this subaccount.

---

### `PUT /v5/accounts/subaccounts/{subaccount_id}/limit/custom/monthly` — Set Sending Limit

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number | Yes | The monthly sending limit to set |

**Response (200):**

```json
{
  "success": true
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "Invalid subaccount ID" }` | Invalid subaccount ID |

**Mock behavior:** Store the limit value. Initialize `current` to 0 and `period` to `"1m"`.

---

### `DELETE /v5/accounts/subaccounts/{subaccount_id}/limit/custom/monthly` — Remove Sending Limit

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Response (200):**

```json
{
  "success": true
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "Could not delete threshold for account" }` | No limit to delete or invalid ID |

**Mock behavior:** Remove the stored limit for this subaccount.

---

### `PUT /v5/accounts/subaccounts/{subaccount_id}/features` — Update Features

**Path Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `subaccount_id` | string | Yes | The subaccount ID |

**Request body:** `application/x-www-form-urlencoded` with JSON-encoded values per feature key.

Each feature key is sent as a form field with a JSON-stringified `{ "enabled": <boolean> }` value:

```
email_preview={"enabled":true}&sending={"enabled":false}
```

| Feature Key | Description |
|-------------|-------------|
| `email_preview` | Email preview access |
| `inbox_placement` | Inbox placement testing access |
| `sending` | Ability to send messages |
| `validations` | Email validation access |
| `validations_bulk` | Bulk email validation access |

**Response (200):**

```json
{
  "features": {
    "email_preview": { "enabled": true },
    "inbox_placement": { "enabled": false },
    "sending": { "enabled": true },
    "validations": { "enabled": true },
    "validations_bulk": { "enabled": false }
  }
}
```

**Error responses:**

| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{ "message": "No valid updates provided" }` | No valid feature keys |
| 404 | `{ "message": "Not Found" }` | Subaccount ID doesn't exist |

**Mock behavior:** Merge provided feature values into stored features. Return the full features object.

---

## `X-Mailgun-On-Behalf-Of` Header

This is the primary mechanism for parent accounts to operate on subaccount resources. It's a cross-cutting concern that affects ALL endpoints, not just subaccount CRUD.

### How it works

1. The parent account authenticates with its own API key via HTTP Basic Auth
2. The `X-Mailgun-On-Behalf-Of` header is set to a subaccount ID
3. The API call is scoped to that subaccount — accessing its domains, messages, suppressions, etc.
4. If the header is absent, the call operates on the parent account

### SDK patterns

Both the Node.js and Ruby SDKs implement this as a **sticky/stateful** header:

```javascript
// Node.js
const mg = mailgun({ apiKey: 'primary-key', ... });
mg.setSubaccount('646d00a1b32c35364a2ad34f');  // all calls now scoped
await mg.messages.create('subaccount-domain.com', { ... });
mg.resetSubaccount();  // back to primary
```

```ruby
# Ruby
mg_client = Mailgun::Client.new('primary-key')
mg_client.set_subaccount('646d00a1b32c35364a2ad34f')
mg_client.send_message('subaccount-domain.com', message_params)
mg_client.reset_subaccount
```

### Mock implementation

The mock should:
1. Check for `X-Mailgun-On-Behalf-Of` header on every request
2. If present, validate the subaccount ID exists and is not disabled
3. Scope all storage lookups/writes to that subaccount's isolated namespace
4. If absent, operate on the primary/default account

For simplicity, the mock can use the subaccount ID as a namespace prefix for storage keys (e.g., `subaccount:646d00a1/domains/...`), or store a `subaccount_id` field on each resource and filter by it.

---

## Integration Points

### With Domains (`domains.md`)

- Domains belong to a subaccount (or the primary account). The `subaccount_id` field appears in domain list responses when `include_subaccounts=true` is used.
- When `X-Mailgun-On-Behalf-Of` is set, domain operations are scoped to that subaccount's domains.

### With Messages (`messages.md`)

- When `X-Mailgun-On-Behalf-Of` is set, messages are sent from the subaccount's domains.
- The mock should store the subaccount context on each sent message for inspection.

### With Events (`events-and-logs.md`)

- Event polling can include `include_subaccounts=true` to see events from all subaccounts.
- Webhook payloads at the account level include `domain-name` and `account-id` fields that map to the subaccount.

### With Credentials & Keys (`credentials-and-keys.md`)

- Each subaccount has its own API keys and SMTP credentials.
- The `X-Mailgun-On-Behalf-Of` header uses the **parent account's** API key — not the subaccount's key.

### With IPs & IP Pools (`ips-and-pools.md`)

- IP pools can be delegated to subaccounts via `/v5/accounts/subaccounts/{id}/ip_pool` (PUT/DELETE)
- Individual IPs can be linked via `/v5/accounts/subaccounts/{id}/ip` (PUT)
- These are complex async operations — stub only.

### With Metrics & Analytics (`metrics-and-analytics.md`)

- Metrics endpoints accept `include_subaccounts` parameter and `subaccount` dimension/filter.
- Stats can be aggregated across all subaccounts or queried per subaccount.

---

## SDK Support Matrix

| Operation | Node.js | Ruby | Python |
|-----------|---------|------|--------|
| List subaccounts | `subaccounts.list()` | `get_subaccounts()` / `list()` | — |
| Get subaccount | `subaccounts.get(id)` | `info(id)` | — |
| Create subaccount | `subaccounts.create(name)` | `create(name)` | — |
| Disable subaccount | `subaccounts.disable(id)` | `disable(id)` | — |
| Enable subaccount | `subaccounts.enable(id)` | `enable(id)` | — |
| Delete subaccount | `subaccounts.destroy(id)` | — | — |
| Get sending limit | `subaccounts.getMonthlySendingLimit(id)` | — | — |
| Set sending limit | `subaccounts.setMonthlySendingLimit(id, limit)` | — | — |
| Update features | `subaccounts.updateSubaccountFeature(id, features)` | — | — |
| Set on-behalf-of | `setSubaccount(id)` | `set_subaccount(id)` | — |
| Reset on-behalf-of | `resetSubaccount()` | `reset_subaccount` | — |

The Python SDK has no subaccount support. The Ruby SDK only supports the core 5 CRUD operations plus the on-behalf-of header. The Node.js SDK has the most complete support.

---

## Stub Endpoints (Accept and Return Static Responses)

These subaccount-adjacent endpoints exist in the OpenAPI spec but are production-only concerns. The mock should accept the calls and return success.

### IP Pool Delegation

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v5/accounts/subaccounts/ip_pools/all` | List DIPPs delegated to subaccounts |
| `PUT` | `/v5/accounts/subaccounts/{subaccountId}/ip_pool` | Delegate a DIPP to a subaccount |
| `DELETE` | `/v5/accounts/subaccounts/{subaccountId}/ip_pool` | Revoke a DIPP from a subaccount |
| `PUT` | `/v5/accounts/subaccounts/{subaccountId}/ip` | Link a dedicated IP to a subaccount |

### Account-Level Sending Limits

These are non-subaccount endpoints that share the same schema:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v5/accounts/limit/custom/monthly` | Get account sending limit |
| `PUT` | `/v5/accounts/limit/custom/monthly` | Set account sending limit |
| `DELETE` | `/v5/accounts/limit/custom/monthly` | Remove account sending limit |
| `PUT` | `/v5/accounts/limit/custom/enable` | Re-enable account disabled for hitting limit |

---

## Test Scenarios

1. **Basic CRUD:** Create subaccount → get → list → disable → enable → delete
2. **List filtering:** Create multiple subaccounts, test `filter`, `enabled`, `sort`, `skip`, `limit`
3. **Disable idempotency:** Disabling an already-disabled subaccount returns 400
4. **Delete via header:** Verify `X-Mailgun-On-Behalf-Of` header is required for delete
5. **On-behalf-of scoping:** Set header, create a domain, verify domain is associated with subaccount
6. **Not found:** Get/disable/enable with non-existent ID returns 404
7. **Sending limits:** Set → get → remove limit cycle
8. **Feature updates:** Update individual features, verify merge behavior

---

## References

- **OpenAPI spec:** `mailgun.yaml` — `/v5/accounts/subaccounts*` paths (lines 12219–12929), Subaccount schemas (lines 20558–20616), Feature/Limit schemas (lines 20544–20657)
- **API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/subaccounts
- **Subaccounts user guide:** https://documentation.mailgun.com/docs/mailgun/user-manual/subaccounts/
- **On-behalf-of usage:** https://documentation.mailgun.com/docs/mailgun/user-manual/subaccounts/subaccounts-api-requests
- **Feature controls:** https://documentation.mailgun.com/docs/mailgun/user-manual/subaccounts/subaccounts-features
- **Node.js SDK:** `mailgun.js` — `lib/Classes/Subaccounts.ts`, `lib/Types/Subaccounts/Subaccounts.ts`
- **Ruby SDK:** `mailgun-ruby` — `lib/mailgun/subaccounts/subaccounts.rb`, `docs/Subaccounts.md`
- **Go SDK:** `mailgun-go` — `subaccounts.go`, `mtypes/subaccounts.go`
