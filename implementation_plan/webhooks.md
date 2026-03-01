# Webhooks

Register webhook URLs to receive event notifications, deliver event payloads to those URLs, and support signature verification. The mock delivers webhooks in real-time when events are generated (e.g., a message is accepted/delivered) and supports both domain-level and account-level webhook configurations.

Mailgun has three generations of webhook management APIs: v3 domain webhooks (per-event-type CRUD), v4 domain webhooks (URL-centric multi-event), and v1 account-level webhooks (cross-domain). All three should be supported. Webhook delivery uses the "Webhooks 2.0" JSON payload format with `signature` and `event-data` top-level objects.

## Endpoints

### Domain Webhooks — v3 (per-event-type model)

The v3 API manages webhooks one event type at a time. Each event type maps to a list of up to 3 URLs.

#### 1. GET `/v3/domains/{domain}/webhooks` — List all domain webhooks

Returns all webhook configurations for a domain, keyed by event type.

**Auth:** HTTP Basic (`api:<key>`)

##### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain` | string | yes | Domain name |

##### Success Response (200)

```json
{
  "webhooks": {
    "delivered": {
      "urls": ["https://example.com/hooks/delivered"]
    },
    "opened": {
      "urls": ["https://example.com/hooks/opened", "https://backup.example.com/hooks/opened"]
    },
    "clicked": null,
    "accepted": null,
    "unsubscribed": null,
    "complained": null,
    "temporary_fail": null,
    "permanent_fail": null
  }
}
```

Event types with no configured URLs return `null`.

##### Error Responses

| Code | Description |
|------|-------------|
| 401 | Unauthorized — invalid API key |
| 404 | Domain not found |

---

#### 2. GET `/v3/domains/{domain_name}/webhooks/{webhook_name}` — Get webhook by event type

Returns the URL(s) registered for a specific event type.

**Auth:** HTTP Basic (`api:<key>`)

##### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain_name` | string | yes | Domain name |
| `webhook_name` | string (enum) | yes | Event type: `accepted`, `delivered`, `opened`, `clicked`, `unsubscribed`, `complained`, `temporary_fail`, `permanent_fail` |

##### Success Response (200)

```json
{
  "webhook": {
    "urls": ["https://example.com/hooks/delivered"]
  }
}
```

##### Error Responses

| Code | Description |
|------|-------------|
| 401 | Unauthorized |
| 404 | Domain or webhook type not found |

---

#### 3. POST `/v3/domains/{domain}/webhooks` — Create webhook

Register one or more URLs for a specific event type.

**Auth:** HTTP Basic (`api:<key>`)
**Content-Type:** `multipart/form-data`

##### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain` | string | yes | Domain name |

##### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string (enum) | yes | Event type: `accepted`, `delivered`, `opened`, `clicked`, `unsubscribed`, `complained`, `temporary_fail`, `permanent_fail` |
| `url` | string | yes | Webhook URL. Specify multiple times for up to 3 URLs per event type. |

##### Success Response (200)

```json
{
  "message": "Webhook has been created",
  "webhook": {
    "urls": ["https://example.com/hooks/delivered"]
  }
}
```

##### Error Responses

| Code | Description |
|------|-------------|
| 400 | Invalid parameters (bad event type, too many URLs, etc.) |
| 401 | Unauthorized |
| 404 | Domain not found |

---

#### 4. PUT `/v3/domains/{domain_name}/webhooks/{webhook_name}` — Update webhook

Replace all URLs for a specific event type.

**Auth:** HTTP Basic (`api:<key>`)
**Content-Type:** `multipart/form-data`

##### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain_name` | string | yes | Domain name |
| `webhook_name` | string (enum) | yes | Event type to update |

##### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | New URL(s). Specify multiple times for up to 3. Replaces all existing URLs. |

##### Success Response (200)

```json
{
  "message": "Webhook has been updated",
  "webhook": {
    "urls": ["https://example.com/hooks/v2/delivered"]
  }
}
```

##### Error Responses

| Code | Description |
|------|-------------|
| 400 | Invalid parameters |
| 401 | Unauthorized |
| 404 | Domain or webhook type not found |

---

#### 5. DELETE `/v3/domains/{domain_name}/webhooks/{webhook_name}` — Delete webhook

Remove all URLs for a specific event type.

**Auth:** HTTP Basic (`api:<key>`)

##### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain_name` | string | yes | Domain name |
| `webhook_name` | string (enum) | yes | Event type to delete |

##### Success Response (200)

```json
{
  "message": "Webhook has been deleted",
  "webhook": {
    "urls": []
  }
}
```

##### Error Responses

| Code | Description |
|------|-------------|
| 401 | Unauthorized |
| 404 | Domain or webhook type not found |

---

### Domain Webhooks — v4 (URL-centric model)

The v4 API manages webhooks by URL, allowing one URL to be associated with multiple event types in a single request. All v4 endpoints return the full webhooks map (same shape as GET v3).

#### 6. POST `/v4/domains/{domain}/webhooks` — Create webhook (v4)

Associate a URL with one or more event types.

**Auth:** HTTP Basic (`api:<key>`)
**Content-Type:** `application/x-www-form-urlencoded`

##### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain` | string | yes | Domain name |

##### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | The webhook URL to register |
| `event_types` | string (enum, repeated) | yes | Event types to associate. Specify multiple times. |

##### Success Response (200)

```json
{
  "webhooks": {
    "delivered": { "urls": ["https://example.com/hooks"] },
    "opened": { "urls": ["https://example.com/hooks"] },
    "clicked": null,
    "accepted": null,
    "unsubscribed": null,
    "complained": null,
    "temporary_fail": null,
    "permanent_fail": null
  }
}
```

##### Error Responses

| Code | Description |
|------|-------------|
| 400 | Invalid parameters |
| 401 | Unauthorized |
| 404 | Domain not found |

---

#### 7. PUT `/v4/domains/{domain}/webhooks` — Update webhook (v4)

Replace the event type associations for a URL. The URL must already be registered.

**Auth:** HTTP Basic (`api:<key>`)
**Content-Type:** `application/x-www-form-urlencoded`

##### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | The webhook URL to update |
| `event_types` | string (enum, repeated) | yes | New event types. Replaces existing associations for this URL. |

##### Success Response (200)

Same shape as POST v4 — returns full webhooks map.

---

#### 8. DELETE `/v4/domains/{domain}/webhooks` — Delete webhook (v4)

Remove a URL from all event types. Supports comma-separated URLs to delete multiple at once.

**Auth:** HTTP Basic (`api:<key>`)

##### Query Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `url` | string | yes | URL(s) to delete. Comma-separated for multiple: `url1,url2,url3` |

##### Success Response (200)

Returns full webhooks map after deletion.

---

### Account-Level Webhooks — v1

Account-level webhooks fire for events across ALL domains under the account (and subaccounts). They use a different data model — each webhook has a unique `webhook_id`, a `description`, a single `url`, and a list of `event_types`.

#### 9. GET `/v1/webhooks` — List account webhooks

**Auth:** HTTP Basic (`api:<key>`)

##### Query Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `webhook_ids` | string | no | Comma-separated webhook IDs to filter |

##### Success Response (200)

```json
{
  "webhooks": [
    {
      "webhook_id": "wh_abc123",
      "description": "All events to main endpoint",
      "url": "https://example.com/hooks/all",
      "event_types": ["delivered", "opened", "clicked"],
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

---

#### 10. POST `/v1/webhooks` — Create account webhook

**Auth:** HTTP Basic (`api:<key>`)
**Content-Type:** `multipart/form-data`

##### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | Webhook URL |
| `event_types` | string (enum, repeated) | yes | Event types to subscribe to |
| `description` | string | no | User-provided description |

##### Success Response (200)

```json
{
  "webhook_id": "wh_abc123"
}
```

##### Error Responses

| Code | Description |
|------|-------------|
| 400 | Invalid parameters |
| 403 | Forbidden |
| 409 | Conflict — duplicate URL/event_types combination |

---

#### 11. GET `/v1/webhooks/{webhook_id}` — Get account webhook by ID

**Auth:** HTTP Basic (`api:<key>`)

##### Success Response (200)

```json
{
  "webhook_id": "wh_abc123",
  "description": "All events to main endpoint",
  "url": "https://example.com/hooks/all",
  "event_types": ["delivered", "opened", "clicked"],
  "created_at": "2024-01-15T10:30:00Z"
}
```

---

#### 12. PUT `/v1/webhooks/{webhook_id}` — Update account webhook

**Auth:** HTTP Basic (`api:<key>`)
**Content-Type:** `multipart/form-data`

##### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | New URL |
| `event_types` | string (enum, repeated) | yes | New event types |
| `description` | string | no | New description |

##### Success Response: 204 No Content

##### Error Responses

| Code | Description |
|------|-------------|
| 400 | Invalid parameters |
| 403 | Forbidden |
| 404 | Webhook not found |
| 409 | Conflict |

---

#### 13. DELETE `/v1/webhooks/{webhook_id}` — Delete account webhook by ID

**Auth:** HTTP Basic (`api:<key>`)

##### Success Response: 204 No Content

---

#### 14. DELETE `/v1/webhooks` — Bulk delete account webhooks

**Auth:** HTTP Basic (`api:<key>`)

##### Query Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `webhook_ids` | string | no | Comma-separated IDs to delete |
| `all` | boolean | no | Set `true` to delete all account webhooks |

If both `webhook_ids` and `all` are provided, `webhook_ids` takes precedence.

##### Success Response: 204 No Content

---

### Webhook Signing Key — v5

The signing key is used for verifying webhook payloads. Separate from the API key.

#### 15. GET `/v5/accounts/http_signing_key` — Get signing key

**Auth:** HTTP Basic (`api:<key>`)

##### Success Response (200)

```json
{
  "message": "success",
  "http_signing_key": "key-abc123..."
}
```

---

#### 16. POST `/v5/accounts/http_signing_key` — Regenerate signing key

**Auth:** HTTP Basic (`api:<key>`)

##### Success Response (200)

```json
{
  "message": "success",
  "http_signing_key": "key-new456..."
}
```

---

## Event Types

All webhook endpoints support the same 8 event types:

| Event Type | Webhook Name | Description | log-level |
|------------|-------------|-------------|-----------|
| `accepted` | `accepted` | Mailgun accepted the message for delivery | `info` |
| `delivered` | `delivered` | Message successfully delivered to recipient's mail server | `info` |
| `opened` | `opened` | Recipient opened the email (requires open tracking) | `info` |
| `clicked` | `clicked` | Recipient clicked a link (requires click tracking) | `info` |
| `unsubscribed` | `unsubscribed` | Recipient unsubscribed | `warn` |
| `complained` | `complained` | Recipient marked as spam (ISP feedback loop) | `warn` |
| `temporary_fail` | `temporary_fail` | Soft bounce; Mailgun will retry | `warn` |
| `permanent_fail` | `permanent_fail` | Hard bounce; no further attempts | `error` |

Note: In the event payload, `temporary_fail` and `permanent_fail` are both represented as `"event": "failed"` with a `"severity"` field (`"temporary"` or `"permanent"`). The webhook registration uses the `_fail` suffixed names.

---

## Webhook Payload Format (Webhooks 2.0)

When an event occurs and a matching webhook is configured, Mailgun POSTs a JSON payload to the webhook URL. The payload always has two top-level keys: `signature` and `event-data`.

### Top-Level Structure

```json
{
  "signature": {
    "timestamp": "1529006854",
    "token": "a8ce0edb2dd8301dee6c2405235584e45aa91d1e9f979f3de0",
    "signature": "d2271d12299f6592d9d44cd9d250f0704e4674c30d79d07c47a66f95ce71cf55"
  },
  "event-data": {
    "event": "delivered",
    "timestamp": 1529006854.329574,
    ...
  }
}
```

### Signature Object

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string | Unix epoch seconds (string, not number) |
| `token` | string | Randomly generated 50-character string (unique per request) |
| `signature` | string | HMAC-SHA256 hex digest for verification |

### Signature Verification Algorithm

1. Concatenate `timestamp` + `token` (no separator)
2. Compute HMAC-SHA256 using the **Webhook Signing Key** (NOT the API key) as the secret
3. Hex-encode the result (lowercase)
4. Compare with the `signature` value using constant-time comparison

```javascript
// Node.js example
const crypto = require('crypto');
const computed = crypto
  .createHmac('sha256', webhookSigningKey)
  .update(timestamp + token)
  .digest('hex');
const isValid = crypto.timingSafeEqual(
  Buffer.from(computed), Buffer.from(signature)
);
```

### Event-Data Fields

The `event-data` object is the same event structure documented in [events-and-logs.md](./events-and-logs.md). Key fields shared across all event types:

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | Event type name (`delivered`, `failed`, `opened`, etc.) |
| `timestamp` | float | Unix timestamp with fractional seconds |
| `id` | string | Unique event ID |
| `recipient` | string | Recipient email address |
| `recipient-domain` | string | Domain part of recipient |
| `log-level` | string | `info`, `warn`, or `error` |
| `tags` | string[] | Tags attached to the message |
| `campaigns` | array | Campaign data (legacy, usually empty) |
| `user-variables` | object | Custom `v:` variables from send |
| `message.headers` | object | `to`, `from`, `subject`, `message-id` |

#### Delivery events (accepted, delivered, failed) additionally include:

| Field | Type | Description |
|-------|------|-------------|
| `envelope.sending-ip` | string | Sending IP address |
| `envelope.sender` | string | MAIL FROM address |
| `envelope.transport` | string | `"smtp"` |
| `envelope.targets` | string | RCPT TO address |
| `storage.url` | string | URL to retrieve stored message |
| `storage.key` | string | Storage key |
| `flags.is-routed` | boolean | Whether message was routed |
| `flags.is-authenticated` | boolean | Whether DKIM/SPF passed |
| `flags.is-system-test` | boolean | System test flag |
| `flags.is-test-mode` | boolean | Whether `o:testmode` was set |
| `delivery-status.code` | integer | SMTP response code |
| `delivery-status.message` | string | SMTP response message |
| `delivery-status.attempt-no` | integer | Delivery attempt number |
| `delivery-status.tls` | boolean | TLS was used |
| `delivery-status.mx-host` | string | MX host |
| `delivery-status.session-seconds` | float | SMTP session duration |

#### Failed events additionally include:

| Field | Type | Description |
|-------|------|-------------|
| `severity` | string | `"temporary"` or `"permanent"` |
| `reason` | string | Failure reason (`generic`, `bounce`, `suppress-bounce`, `suppress-complaint`, `suppress-unsubscribe`) |
| `delivery-status.retry-seconds` | integer | Seconds until next retry (temporary only) |

#### Engagement events (opened, clicked, unsubscribed) additionally include:

| Field | Type | Description |
|-------|------|-------------|
| `ip` | string | Client IP address |
| `geolocation.country` | string | Country code |
| `geolocation.region` | string | Region code |
| `geolocation.city` | string | City name |
| `client-info.client-name` | string | Browser/client name |
| `client-info.client-os` | string | OS name |
| `client-info.user-agent` | string | Full user-agent string |
| `client-info.device-type` | string | `desktop`, `mobile`, `tablet` |
| `client-info.client-type` | string | `browser`, `mobile browser` |

#### Clicked events additionally include:

| Field | Type | Description |
|-------|------|-------------|
| `url` | string | The URL that was clicked |

### Example: Delivered Webhook Payload

```json
{
  "signature": {
    "timestamp": "1534190556",
    "token": "68f5e45cc3258acdeb5d9dba65ed9f2595c8a0dee3efe9c9af",
    "signature": "252067c7f453e461dbc33c746f8470decadf70b4cd8b5edd8a72284055b4f04d"
  },
  "event-data": {
    "event": "delivered",
    "timestamp": 1521472262.9082,
    "id": "CPgfbmQMTCKtHW6uIWtuVe",
    "log-level": "info",
    "recipient": "alice@example.com",
    "recipient-domain": "example.com",
    "tags": ["welcome"],
    "campaigns": [],
    "user-variables": {},
    "flags": {
      "is-routed": false,
      "is-authenticated": true,
      "is-system-test": false,
      "is-test-mode": false
    },
    "envelope": {
      "sending-ip": "209.61.154.250",
      "sender": "bob@suet.co",
      "transport": "smtp",
      "targets": "alice@example.com"
    },
    "storage": {
      "url": "https://se.api.mailgun.net/v3/domains/suet.co/messages/message_key",
      "key": "message_key"
    },
    "message": {
      "headers": {
        "to": "Alice <alice@example.com>",
        "from": "Bob <bob@suet.co>",
        "subject": "Welcome!",
        "message-id": "20130503182626.18666.16540@suet.co"
      },
      "attachments": [],
      "size": 512
    },
    "delivery-status": {
      "tls": true,
      "mx-host": "smtp-in.example.com",
      "code": 250,
      "message": "OK",
      "attempt-no": 1,
      "session-seconds": 0.433,
      "utf8": true,
      "certificate-verified": true,
      "description": ""
    }
  }
}
```

---

## Webhook Delivery Behavior

### Retry Policy

| Attempt | Delay after previous |
|---------|---------------------|
| 1 | Immediate |
| 2 | 10 minutes |
| 3 | 10 minutes |
| 4 | 15 minutes |
| 5 | 30 minutes |
| 6 | 1 hour |
| 7 | 2 hours |
| 8 | 4 hours |

Total retry window: ~8 hours, 7 retry attempts.

### Response Code Handling

| HTTP Response | Behavior |
|---------------|----------|
| 200 | Success — webhook accepted, no retry |
| 406 | Explicit rejection — no retry |
| Any other code | Retry per schedule above |

### Critical Exception

**`delivered` event webhooks are NOT retried.** If the endpoint is down when a delivery event fires, that notification is permanently lost. This is a known Mailgun behavior.

### URL Limits

- Maximum **3 URLs** per event type per domain
- Each URL receives the same payload independently

### Deduplication

If the same URL is configured at both domain-level and account-level for the same event type, Mailgun sends the event only **once** to that URL.

### Account-Level Extra Fields

Webhooks fired at the account level include additional fields in `event-data`:

| Field | Type | Description |
|-------|------|-------------|
| `domain-name` | string | Domain the event originated from |
| `account-id` | string | Account/subaccount ID |

---

## Mock Behavior

### Webhook Storage

The mock stores webhooks in memory, organized as:
- **Domain webhooks:** `{ domain → { event_type → url[] } }` — supports both v3 and v4 management
- **Account webhooks:** `[ { webhook_id, url, event_types[], description, created_at } ]`
- **Signing key:** One per account, auto-generated on first use, retrievable/regenerable via v5 endpoints

### Webhook Delivery Pipeline

When an event is generated (via the message → event pipeline defined in [events-and-logs.md](./events-and-logs.md)):

1. Determine the event type (e.g., `delivered`)
2. Look up matching domain-level webhook URLs for that event type on the event's domain
3. Look up matching account-level webhook URLs for that event type
4. Deduplicate URLs across both levels
5. For each unique URL:
   a. Generate a `signature` object (timestamp, random 50-char token, HMAC-SHA256 digest)
   b. Build the payload: `{ signature, "event-data": <event object> }`
   c. POST the payload as `Content-Type: application/json` to the URL
   d. Record delivery attempt (status code, response time)

### Retry Simulation

The mock should support a **configurable retry mode**:

- **`immediate` (default):** Deliver webhooks synchronously when events are generated. No retries — if the endpoint fails, log the failure and move on. This is the simplest mode for local dev.
- **`realistic`:** Queue webhook deliveries and implement the full retry schedule (10m, 10m, 15m, 30m, 1h, 2h, 4h). Respect the 200/406/retry logic. Skip retries for `delivered` events. This mode is useful for testing retry handling.

### Signature Generation

The mock generates valid signatures that consumers can verify:

1. Generate `timestamp` = current Unix epoch seconds (as string)
2. Generate `token` = 50 random hex characters
3. Compute `signature` = HMAC-SHA256(`signing_key`, `timestamp` + `token`).hexdigest()

The signing key is deterministic per mock instance (e.g., `"key-mock-signing-key-000000000000"`) so consumers can hardcode it in tests, or retrieve it via the GET signing key endpoint.

### Mock-Specific Features

#### Webhook Delivery Log

The mock should maintain a queryable log of all webhook delivery attempts:

```
GET /mock/webhooks/deliveries
```

Returns:
```json
{
  "deliveries": [
    {
      "id": "wd_001",
      "webhook_url": "https://example.com/hooks",
      "event_type": "delivered",
      "event_id": "evt_abc123",
      "domain": "example.com",
      "status_code": 200,
      "response_time_ms": 45,
      "attempt": 1,
      "timestamp": "2024-01-15T10:30:00Z",
      "payload": { ... }
    }
  ]
}
```

#### Manual Webhook Trigger

Allow users to manually fire a webhook for testing:

```
POST /mock/webhooks/trigger
```

Body:
```json
{
  "domain": "example.com",
  "event_type": "delivered",
  "recipient": "alice@example.com",
  "message_id": "msg_abc123"
}
```

This generates a synthetic event and delivers it to all matching webhook URLs.

---

## Constraints and Limits

| Constraint | Value | Mock Handling |
|------------|-------|---------------|
| Max URLs per event type | 3 | Enforce; return 400 if exceeded |
| Webhook management rate limit | 300 req/min | Ignore in mock |
| HTTPS requirement for URLs | Required in production | Accept HTTP too (local dev convenience) |
| Caching delay for changes | Up to 10 min in production | Apply immediately in mock |
| `delivered` event retry | Not retried in production | Configurable; default no retry |
| Custom variables payload cap | 4KB | Ignore in mock (no truncation) |

---

## v3 vs v4 API Differences

| Aspect | v3 | v4 |
|--------|----|----|
| Path pattern | `/v3/domains/{domain}/webhooks/{webhook_name}` | `/v4/domains/{domain}/webhooks` |
| Model | One event type per request | One URL associated with multiple event types |
| Content-Type | `multipart/form-data` | `application/x-www-form-urlencoded` |
| Create | POST with `id` (event type) + `url` | POST with `url` + `event_types` (repeated) |
| Update | PUT to `/{webhook_name}` with `url` | PUT with `url` + `event_types` |
| Delete | DELETE `/{webhook_name}` | DELETE with `?url=` query param (comma-separated) |
| Response | `{ message, webhook: { urls } }` | `{ webhooks: { <full map> } }` |

Both APIs operate on the same underlying data. Changes via v3 are reflected in v4 responses and vice versa.

---

## Domain vs Account Webhooks

| Aspect | Domain-Level | Account-Level |
|--------|-------------|---------------|
| Scope | Single domain | All domains under the account |
| API | `/v3/domains/{domain}/webhooks` (v3) or `/v4/domains/{domain}/webhooks` (v4) | `/v1/webhooks` |
| URL limit | 3 per event type per domain | No documented per-event limit |
| Identifier | Event type name | Unique `webhook_id` |
| Extra fields in payload | Standard | Adds `domain-name`, `account-id` |
| Deduplication | Combined with account-level by URL | Combined with domain-level by URL |

---

## Integration Points

### Event Pipeline → Webhook Delivery

The events system (see [events-and-logs.md](./events-and-logs.md)) generates events when messages are processed. The webhook system subscribes to these events:

- **Message accepted** → `accepted` event → fire `accepted` webhooks
- **Message delivered** → `delivered` event → fire `delivered` webhooks (no retry)
- **Message failed** → `failed` event with severity → fire `temporary_fail` or `permanent_fail` webhooks
- **Suppression hit** → `failed` event with reason `suppress-*` → fire `permanent_fail` webhooks
- **Open tracked** → `opened` event → fire `opened` webhooks
- **Click tracked** → `clicked` event → fire `clicked` webhooks
- **Unsubscribe** → `unsubscribed` event → fire `unsubscribed` webhooks
- **Complaint** → `complained` event → fire `complained` webhooks

### Mock Event Triggers → Webhook Delivery

The mock-specific trigger endpoints defined in [events-and-logs.md](./events-and-logs.md) (`/mock/events/{domain}/deliver/{message_id}`, etc.) should also trigger webhook delivery for the generated events.

---

## Test Scenarios

1. **CRUD lifecycle:** Create, list, get, update, delete webhooks via v3 API
2. **v4 multi-event:** Create a webhook for multiple event types in one call, verify it appears in v3 list
3. **v3/v4 interop:** Create via v3, read via v4, and vice versa
4. **Account webhooks:** Create account-level webhooks, verify they fire for events on any domain
5. **Webhook delivery on send:** Send a message, verify `accepted` webhook fires with correct payload
6. **Signature verification:** Verify the webhook payload signature using the signing key
7. **Signing key rotation:** Regenerate signing key, verify new webhooks use new key
8. **URL limit enforcement:** Try to add a 4th URL to an event type, expect 400
9. **Deduplication:** Same URL at domain and account level, verify single delivery
10. **Delivery log:** Send message, check `/mock/webhooks/deliveries` for delivery records
11. **Manual trigger:** Use `/mock/webhooks/trigger` to fire a test webhook
12. **Failed delivery:** Configure webhook to unreachable URL, verify failure is logged
13. **Delete cascade:** Delete a domain, verify its webhooks are cleaned up

---

## References

### OpenAPI Spec (primary source)
- `mailgun.yaml` — lines 1352-2051 (domain webhooks v3/v4), 2784-3172 (account webhooks v1), 12035-12087 (signing key v5)
- Response schemas: lines 15331-15592 (domain), 15905-16067 (account), 20617-20625 (signing key)

### API Documentation
- Webhooks API Reference: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/webhooks
- Securing Webhooks: https://documentation.mailgun.com/docs/mailgun/user-manual/webhooks/securing-webhooks
- Webhook Retries: https://documentation.mailgun.com/docs/mailgun/user-manual/webhooks/webhook-retries
- Configuring Webhooks: https://documentation.mailgun.com/docs/mailgun/user-manual/webhooks/configuring-webhooks

### Help Center
- Webhooks Overview: https://help.mailgun.com/hc/en-us/articles/202236504-Webhooks

### Client Libraries
- Node.js (mailgun.js) `WebhooksClient`: https://github.com/mailgun/mailgun.js — list/get/create/update/destroy, no built-in verification
- Ruby (mailgun-ruby) `Mailgun::Webhooks`: https://github.com/mailgun/mailgun-ruby — includes `create_all`/`remove_all` convenience methods
- Python (mailgun-python): https://github.com/mailgun/mailgun-python — get/create/delete
- PHP (mailgun-php) `Webhook`: https://github.com/mailgun/mailgun-php — includes `verifyWebhookSignature()`
- Go (mailgun-go) `webhooks.go`: https://github.com/mailgun/mailgun-go — includes `VerifyWebhookSignature()`
