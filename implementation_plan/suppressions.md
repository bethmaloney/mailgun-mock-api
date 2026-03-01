# Suppressions

Mailgun maintains four per-domain suppression/allowance lists: **Bounces**, **Complaints**, **Unsubscribes**, and **Allowlists** (legacy API name: "whitelists"). When an address appears on any suppression list, Mailgun drops the message during processing and generates a permanent failure event — it never reaches the remote mail server.

## Suppression Types Overview

| Type | What triggers it | Effect on sends | Data model |
|------|-----------------|-----------------|------------|
| **Bounces** | Hard bounce (5xx SMTP error) from delivery attempt, or manual add | Message dropped with `"Not delivering to previously bounced address"` | `address`, `code`, `error`, `created_at` |
| **Complaints** | Recipient marks message as spam (via ESP Feedback Loop), or manual add | Message dropped with `"Not delivering to a user who marked your messages as spam"` | `address`, `created_at` |
| **Unsubscribes** | Recipient clicks unsubscribe link, or manual add | Message dropped with `"Not delivering to unsubscribed address"` | `address`, `tags`, `created_at` |
| **Allowlist** | Manual add only | Prevents address/domain from being added to bounce list | `type`, `value`, `reason`, `createdAt` |

## Integration with Message Sending Pipeline

```
POST /v3/{domain}/messages → HTTP 200 (always accepted)
  → "accepted" event generated
  → Suppression check:
      - Is recipient on BOUNCES list?      → drop, "failed" (permanent), reason: suppress-bounce
      - Is recipient on COMPLAINTS list?   → drop, "failed" (permanent), reason: suppress-complaint
      - Is recipient on UNSUBSCRIBES list?  → drop, "failed" (permanent), reason: suppress-unsubscribe
        (tag-scoped: only suppressed if message tag matches unsubscribe tag, or unsubscribe tag is "*")
      - None matched → proceed to delivery
  → On delivery attempt:
      - Success → "delivered" event
      - Hard bounce (5xx) → "failed" (permanent), address added to bounces
        UNLESS address/domain is on allowlist → bounce NOT recorded
      - Soft bounce (4xx) → retry, eventually "failed" (temporary)
```

Key behaviors:
- The API **always accepts** the message (HTTP 200) — suppression checks happen asynchronously
- Allowlist only prevents entries from being added to the **bounce** list — it does NOT override complaint or unsubscribe suppressions
- Each suppression list is **scoped per-domain** — a bounce on `domain-a.com` does not affect `domain-b.com`
- Suppressed messages generate `permanent_fail` webhook events

## Mock Behavior

The mock should:
1. Store all four suppression types per domain in memory
2. On message send, check suppressions before generating delivery events
3. If suppressed: generate `failed` event with severity `permanent` and appropriate reason (`suppress-bounce`, `suppress-complaint`, `suppress-unsubscribe`)
4. If delivered and a bounce is simulated: check allowlist before adding to bounce list
5. Support all CRUD endpoints, bulk JSON operations, and CSV import
6. Use cursor-based pagination (address-alphabetical ordering) for list endpoints

The mock does NOT need to:
- Actually process FBL (Feedback Loop) complaint notifications from ESPs
- Generate real unsubscribe links in emails (but should support the `%unsubscribe_url%` variable)
- Enforce the 25MB CSV file size limit (accept any size)

---

## API Endpoints

All endpoints are under `/v3/{domain_name}/` and require HTTP Basic Auth (`api:<api_key>`).

### Bounces

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v3/{domain_name}/bounces` | List bounces (paginated) |
| GET | `/v3/{domain_name}/bounces/{address}` | Get single bounce |
| POST | `/v3/{domain_name}/bounces` | Add bounce(s) — single via form-data or up to 1000 via JSON array |
| POST | `/v3/{domain_name}/bounces/import` | Import bounces from CSV file |
| DELETE | `/v3/{domain_name}/bounces/{address}` | Delete single bounce |
| DELETE | `/v3/{domain_name}/bounces` | Clear all bounces for domain |

### Complaints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v3/{domain_name}/complaints` | List complaints (paginated) |
| GET | `/v3/{domain_name}/complaints/{address}` | Get single complaint |
| POST | `/v3/{domain_name}/complaints` | Add complaint(s) — single via form-data or up to 1000 via JSON array |
| POST | `/v3/{domain_name}/complaints/import` | Import complaints from CSV file |
| DELETE | `/v3/{domain_name}/complaints/{address}` | Delete single complaint |
| DELETE | `/v3/{domain_name}/complaints` | Clear all complaints for domain |

### Unsubscribes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v3/{domain_name}/unsubscribes` | List unsubscribes (paginated) |
| GET | `/v3/{domain_name}/unsubscribes/{address}` | Get single unsubscribe |
| POST | `/v3/{domain_name}/unsubscribes` | Add unsubscribe(s) — single via form-data or up to 1000 via JSON array |
| POST | `/v3/{domain_name}/unsubscribes/import` | Import unsubscribes from CSV file |
| DELETE | `/v3/{domain_name}/unsubscribes/{address}` | Delete single unsubscribe |
| DELETE | `/v3/{domain_name}/unsubscribes` | Clear all unsubscribes for domain |

### Allowlist (path: `whitelists`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v3/{domain_name}/whitelists` | List allowlist records (paginated) |
| GET | `/v3/{domain_name}/whitelists/{value}` | Get single allowlist record (by address or domain) |
| POST | `/v3/{domain_name}/whitelists` | Add allowlist record (single only — no batch JSON) |
| POST | `/v3/{domain_name}/whitelists/import` | Import allowlist from CSV file |
| DELETE | `/v3/{domain_name}/whitelists/{value}` | Delete single allowlist record |
| DELETE | `/v3/{domain_name}/whitelists` | Clear all allowlist records for domain |

---

## Request / Response Schemas

### Bounce Model

```json
{
  "address": "foo@bar.com",
  "code": "550",
  "error": "No such mailbox",
  "created_at": "Thu, 11 Dec 2025 01:49:40 UTC"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | yes | Email address that bounced |
| `code` | string | yes | SMTP error code (default: `"550"`) |
| `error` | string | yes | SMTP error message (default: `""`) |
| `created_at` | string (RFC 2822) | yes | Timestamp of bounce event |

### Complaint Model

```json
{
  "address": "user@example.com",
  "created_at": "Thu, 11 Dec 2025 01:49:40 UTC"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | yes | Email address that complained |
| `created_at` | string (RFC 2822) | yes | Timestamp of complaint event |

Note: The OpenAPI spec does not include a `count` field, but the API documentation and Go SDK show complaints have a `count` field tracking repeated complaints from the same address. The mock should include `count` (default `1`).

### Unsubscribe Model

```json
{
  "address": "user@example.com",
  "tags": ["newsletter"],
  "created_at": "Thu, 11 Dec 2025 01:49:40 UTC"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | yes | Email address that unsubscribed |
| `tags` | string[] | yes | Tags unsubscribed from; `["*"]` means all mail |
| `created_at` | string (RFC 2822) | yes | Timestamp of unsubscribe event |

Note: The Go SDK model also includes an `ID` field (`string`). The mock should generate an ID for each unsubscribe record.

### Allowlist Model

```json
{
  "type": "domain",
  "value": "example.com",
  "reason": "Partner domain",
  "createdAt": "Thu, 11 Dec 2025 01:49:40 UTC"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"domain"` or `"address"` |
| `value` | string | yes | The domain name or email address |
| `reason` | string | yes | User-provided reason for allowlisting |
| `createdAt` | string (RFC 2822) | yes | Timestamp — **camelCase** (unlike other suppression types) |

---

## Endpoint Details

### List Endpoints (GET collection)

All list endpoints share the same query parameters and pagination structure.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 100 | Max records per page (max: 1000) |
| `page` | string | (first page) | Cursor direction: `next`, `previous`, or `last` |
| `address` | string | — | Address used as cursor divider between pages |
| `term` | string | — | Filter records where address starts with this substring |

**Response (200):**

```json
{
  "items": [ ... ],
  "paging": {
    "first": "https://api.mailgun.net/v3/{domain}/bounces?page=first",
    "next": "https://api.mailgun.net/v3/{domain}/bounces?page=next&address=foo@bar.com",
    "previous": "https://api.mailgun.net/v3/{domain}/bounces?page=prev&address=foo@bar.com",
    "last": "https://api.mailgun.net/v3/{domain}/bounces?page=last"
  }
}
```

Pagination is cursor-based using alphabetical address ordering. The `paging` object always contains all four link fields (`first`, `next`, `previous`, `last`).

### Get Single Record (GET by address/value)

Returns the record object directly (not wrapped in `items`).

**200 OK:** The model object (bounce/complaint/unsubscribe/allowlist)

**404 Not Found:**
```json
{ "message": "Address not found in bounces table" }
```

Error messages by type:
- Bounces: `"Address not found in bounces table"`
- Complaints: `"No spam complaints found for this address"`
- Unsubscribes: `"Address not found in unsubscribers table"`
- Allowlists: `"Address/Domain not found in allowlist table"`

### Create / Add Records (POST collection)

Supports two content types:

**Form-data (single record):**

| Type | Fields |
|------|--------|
| Bounce | `address` (required), `code` (optional, default `"550"`), `error` (optional, default `""`), `created_at` (optional, default now) |
| Complaint | `address` (required), `created_at` (optional, default now) |
| Unsubscribe | `address` (required), `tag` (optional, default `"*"`), `created_at` (optional, default now) |
| Allowlist | `address` (optional) OR `domain` (optional) — one required, `address` takes priority |

Note for unsubscribes: the form-data field is `tag` (singular string), not `tags`.

**JSON array (up to 1000 records):**

Content-Type must be `application/json`. Request body is a JSON array of record objects. For unsubscribes, the JSON field is `tags` (plural, array of strings). Allowlists do NOT support JSON batch creation.

**200 OK:**
```json
{ "message": "4 addresses have been added to the bounces table" }
```

**400 Bad Request:**
```json
{ "message": "Batch size should be less than 1000" }
```

### CSV Import (POST collection/import)

Content-Type: `multipart/form-data` with a `file` field containing CSV data (max 25 MB).

**CSV columns by type:**

| Type | Columns |
|------|---------|
| Bounces | `address` (required), `code` (optional), `error` (optional), `created_at` (optional) |
| Complaints | `address` (required), `created_at` (optional) |
| Unsubscribes | `address` (required), `tags` (optional, default `*`), `created_at` (optional) |
| Allowlists | `address` (optional), `domain` (optional) — one per row |

**202 Accepted:**
```json
{ "message": "file uploaded successfully for processing. standby..." }
```

Note: The real API processes CSV imports asynchronously (hence 202). The mock can process synchronously and still return 202.

### Delete Single Record (DELETE by address/value)

**200 OK:**
```json
{
  "message": "Bounced addresses for this domain have been removed",
  "address": "foo@bar.com"
}
```

For allowlists, the response uses `value` instead of `address`:
```json
{
  "message": "Allowlist address/domain has been removed",
  "value": "example.com"
}
```

**404 Not Found:** Same messages as GET single record.

Behavior: After deletion, delivery to the address resumes until it triggers suppression again.

### Clear All (DELETE collection)

**200 OK:**
```json
{ "message": "Bounced addresses for this domain have been removed" }
```

Messages by type:
- Bounces: `"Bounced addresses for this domain have been removed"`
- Complaints: `"Complaint addresses for this domain have been removed"`
- Unsubscribes: `"Unsubscribe addresses for this domain have been removed"`
- Allowlists: `"Allowlist addresses/domains for this domain have been removed"`

---

## Domain Unsubscribe Tracking Settings

Related endpoint for configuring auto-inserted unsubscribe links:

| Method | Path | Description |
|--------|------|-------------|
| PUT | `/v3/domains/{name}/tracking/unsubscribe` | Update unsubscribe tracking settings |

**Request (form-data):**

| Field | Type | Description |
|-------|------|-------------|
| `active` | boolean | Enable/disable unsubscribe tracking |
| `html_footer` | string | HTML footer with unsubscribe link for HTML email part |
| `text_footer` | string | Text footer with unsubscribe link for plain text part |

**Response (200):**
```json
{
  "message": "Domain tracking settings have been updated",
  "unsubscribe": {
    "active": true,
    "html_footer": "<a href=\"%unsubscribe_url%\">Unsubscribe</a>",
    "text_footer": "To unsubscribe visit: %unsubscribe_url%"
  }
}
```

This endpoint is already documented in the Domains plan doc — listed here for completeness since it directly relates to unsubscribe functionality.

---

## IP Allowlist (v2 — Account Level)

There is a separate account-level IP allowlist API at `/v2/ip_whitelist` for managing which IPs can access the Mailgun API. This is a security/access-control feature, not a sending suppression.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v2/ip_whitelist` | List IP allowlist entries |
| POST | `/v2/ip_whitelist` | Add IP allowlist entry |
| PUT | `/v2/ip_whitelist` | Update IP allowlist entry description |
| DELETE | `/v2/ip_whitelist` | Delete IP allowlist entry |

**Model:**
```json
{
  "addresses": [
    { "ip_address": "10.11.11.111", "description": "OnPrem Server" }
  ]
}
```

**Mock recommendation:** Stub these endpoints (accept calls, return 200 with empty data). The IP allowlist is a security feature that doesn't affect email sending behavior in a mock context.

---

## Test Scenarios

### Suppression CRUD
1. **Add a bounce** via form-data → verify it appears in list → delete it → verify delivery resumes
2. **Batch add bounces** via JSON array (multiple records) → verify all appear in list
3. **Import bounces from CSV** → verify 202 response and records are created
4. **Clear all bounces** → verify list is empty
5. Same CRUD patterns for complaints, unsubscribes, and allowlists

### Suppression Checking on Send
6. **Send to bounced address** → message accepted (200) → `failed` event with reason `suppress-bounce`
7. **Send to complained address** → message accepted (200) → `failed` event with reason `suppress-complaint`
8. **Send to unsubscribed address (tag: *)** → message dropped
9. **Send to tag-unsubscribed address** with different tag → message delivered
10. **Send to tag-unsubscribed address** with matching tag → message dropped

### Allowlist Behavior
11. **Allowlisted address bounces** → delivery fails but address NOT added to bounce list
12. **Allowlisted domain bounces** → same behavior for any address at that domain
13. **Allowlisted + complained** → message still dropped (allowlist doesn't override complaints)

### Pagination
14. **List with limit** → verify correct number of items returned
15. **Page through results** using `next`/`previous` → verify cursor-based ordering
16. **Filter with term** → verify prefix matching on address

### Edge Cases
17. **Add duplicate address** → should update/upsert, not create duplicate
18. **Delete non-existent address** → 404 response
19. **Allowlist: provide both address and domain** → `address` takes priority
20. **Unsubscribe: form-data uses `tag` (singular), JSON uses `tags` (plural array)**

---

## Client Library Patterns

### mailgun.js (Node/TypeScript)
```javascript
// Unified suppressions client with type parameter
const bounces = await mg.suppressions.list('example.com', 'bounces');
const bounce = await mg.suppressions.get('example.com', 'bounces', 'user@example.com');
await mg.suppressions.create('example.com', 'bounces', { address: 'user@example.com', code: 550 });
await mg.suppressions.create('example.com', 'bounces', [/* array of records */]); // batch
await mg.suppressions.destroy('example.com', 'bounces', 'user@example.com');
await mg.suppressions.upload('example.com', 'bounces', csvFile); // CSV import

// Suppression types: 'bounces', 'complaints', 'unsubscribes', 'whitelists'
```

### mailgun-python
```python
# Dynamic attribute-based access
bounces = client.bounces.get(domain="example.com")
client.bounces.create(data={"address": "user@example.com", "code": 550}, domain="example.com")
client.bounces.delete(domain="example.com", bounce_address="user@example.com")
client.bounces_import.create(domain="example.com", files={"file": csv_bytes})

# Same pattern for: complaints, unsubscribes, whitelists
# Import uses: complaints_import, unsubscribes_import, whitelists_import
```

---

## References

### OpenAPI Spec
- `mailgun.yaml` — Bounces endpoints (lines ~7400-7700), Complaints (~7700-8000), Unsubscribes (~8400-9300), Whitelists (~9300-9700)

### API Documentation
- [Bounces API Reference](https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/bounces)
- [Complaints API Reference](https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/complaints)
- [Unsubscribes API Reference](https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/unsubscribe)
- [Suppressions Help Article](https://help.mailgun.com/hc/en-us/articles/360012287493-Suppressions-Bounces-Complaints-Unsubscribes-Allowlists)
- [Unsubscribe Handling & Links](https://help.mailgun.com/hc/en-us/articles/203306610-Unsubscribe-Handling-Links)

### Client Libraries
- [mailgun.js — SuppressionsClient](https://github.com/mailgun/mailgun.js/blob/main/lib/Classes/Suppressions/SuppressionsClient.ts)
- [mailgun-python — suppressions_handler](https://github.com/mailgun/mailgun-python/blob/main/mailgun/handlers/suppressions_handler.py)
- [mailgun-go — bounces.go, mtypes/](https://github.com/mailgun/mailgun-go)
