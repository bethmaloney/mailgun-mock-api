# Events & Logs

Generate realistic events for sent messages, support event polling, filtering, and pagination. Every message accepted by the mock should produce a lifecycle of events (accepted → delivered/failed) that clients can query. The mock also supports stored message retrieval via storage keys embedded in events.

Mailgun has two event APIs: the legacy Events API (`GET /v3/{domain}/events`) and the newer Logs API (`POST /v1/analytics/logs`). The legacy Events API is the one used by all major client libraries and is the primary target for the mock. The Logs API can be stubbed.

## Endpoints

### 1. GET `/v3/{domain_name}/events` — List events (Events API)

The primary event query endpoint. Returns a paginated list of events for a domain, with filtering and time-range support. This is the endpoint all major client libraries use.

**Auth:** HTTP Basic (`api:<key>`)

#### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain_name` | string | yes | Domain to query events for |

#### Query Parameters

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `begin` | string | 7 days ago | Start of time range. RFC 2822 date or Unix epoch seconds. |
| `end` | string | now | End of time range. RFC 2822 date or Unix epoch seconds. |
| `ascending` | string | auto | `"yes"` for oldest-first, `"no"` for newest-first. Default depends on begin/end relationship. |
| `limit` | integer | 100 | Results per page. Max **300**. |
| `event` | string | — | Filter by event type. Supports `OR`: `"rejected OR failed"`. |
| `from` | string | — | Filter by `From` MIME header address |
| `to` | string | — | Filter by `To` MIME header address |
| `recipient` | string | — | Filter by envelope recipient email |
| `recipients` | string | — | Filter by all potential recipients (for stored events) |
| `subject` | string | — | Filter by subject line |
| `message-id` | string | — | Filter by Mailgun message ID |
| `attachment` | string | — | Filter by attachment filename |
| `list` | string | — | Filter by mailing list address |
| `size` | string | — | Filter by message size (bytes) |
| `tags` | string | — | Filter by user-defined tags |
| `severity` | string | — | Filter by severity: `"temporary"` or `"permanent"` (failed events only) |

**Filter expression syntax:** Within a single filter parameter, `AND`, `OR`, `NOT`, and parentheses are supported. Example: `subject=(Hello AND NOT Rachel) OR (Farewell AND Monica)`. Multiple values of the *same* parameter are combined with AND.

**Timestamp behavior:**
- If `end` < `begin`: pages traverse newer-to-older (descending)
- If `end` > `begin`: pages traverse older-to-newer (ascending)
- If only `begin` is specified with `ascending=yes`: open-ended ascending traversal (ideal for polling)

#### Success Response (200)

```json
{
  "items": [
    {
      "id": "czsjqFATSlC3QtAK-C80nw",
      "event": "accepted",
      "timestamp": 1376325780.160809,
      "log-level": "info",
      "recipient": "user@example.com",
      "recipient-domain": "example.com",
      "method": "http",
      "envelope": {
        "sender": "sender@domain.com",
        "transport": "http",
        "targets": "user@example.com",
        "sending-ip": "192.168.1.1"
      },
      "message": {
        "headers": {
          "to": "user@example.com",
          "message-id": "20130812164300.28108.52546@domain.com",
          "from": "Sender <sender@domain.com>",
          "subject": "Hello"
        },
        "attachments": [],
        "recipients": ["user@example.com"],
        "size": 512
      },
      "flags": {
        "is-authenticated": true,
        "is-test-mode": false,
        "is-system-test": false
      },
      "tags": ["welcome"],
      "user-variables": {},
      "storage": {
        "key": "eyJw...",
        "url": "https://api.mailgun.net/v3/domains/domain.com/messages/eyJw..."
      }
    }
  ],
  "paging": {
    "first": "https://api.mailgun.net/v3/domain.com/events/W3siY...",
    "last": "https://api.mailgun.net/v3/domain.com/events/W3siY...",
    "next": "https://api.mailgun.net/v3/domain.com/events/W3siY...",
    "previous": "https://api.mailgun.net/v3/domain.com/events/Lkawm..."
  }
}
```

#### Error Responses

| Status | Body | When |
|--------|------|------|
| 400 | `{"message": "..."}` | Invalid filter parameters |
| 401 | `"Forbidden"` | Invalid/missing API key |

---

### 2. GET `/v3/{domain_name}/events/{page_token}` — Paginated events page

Clients follow the opaque URLs from the `paging` object. The page token is a base64-encoded cursor appended as a path segment.

**Auth:** HTTP Basic (`api:<key>`)

The response shape is identical to the main events endpoint.

---

### 3. POST `/v1/analytics/logs` — Query event logs (Logs API)

The newer analytics-oriented endpoint. Uses POST with a JSON body instead of GET with query params. Supports more advanced filtering, metric aggregation, and unique event deduplication.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (JSON)

```json
{
  "start": "Tue, 01 Jan 2024 00:00:00 -0000",
  "end": "Wed, 02 Jan 2024 00:00:00 -0000",
  "duration": "1d",
  "events": ["accepted", "delivered", "failed"],
  "filter": {
    "AND": [
      { "attribute": "domain", "comparator": "=", "values": [{"label": "example.com", "value": "example.com"}] }
    ]
  },
  "pagination": {
    "sort": "timestamp:desc",
    "limit": 100,
    "token": "..."
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `start` | string | no | RFC 2822 start time |
| `end` | string | no | RFC 2822 end time |
| `duration` | string | yes | Duration string (e.g., `"1d"`, `"2h"`) |
| `events` | string[] | no | Filter by event types |
| `filter` | object | no | Complex filter predicates (AND groups of attribute/comparator/values) |
| `pagination` | object | no | Sort, limit (max 100), and token-based pagination |
| `include_subaccounts` | boolean | no | Include subaccount events |

#### Success Response (200)

```json
{
  "start": "2024-01-01T00:00:00Z",
  "end": "2024-01-02T00:00:00Z",
  "items": [ /* LogEvent objects */ ],
  "pagination": {
    "previous": "token...",
    "next": "token...",
    "first": "token...",
    "last": "token...",
    "total": 150
  },
  "aggregates": {
    "all": 150,
    "metrics": { "accepted": 100, "delivered": 45, "failed": 5 }
  }
}
```

---

## Event Types

| Event Type | Log Level | Description |
|------------|-----------|-------------|
| `accepted` | `info` | Mailgun accepted the request and placed the message in queue |
| `delivered` | `info` | Message was accepted by the recipient's email server |
| `failed` (temporary) | `warn` | Delivery failed temporarily; Mailgun will retry |
| `failed` (permanent) | `error` | Delivery failed permanently; message dropped |
| `rejected` | `warn`/`error` | Mailgun rejected the request (e.g., suppressed address, policy violation) |
| `opened` | `info` | Recipient opened the email (requires open tracking) |
| `clicked` | `info` | Recipient clicked a link (requires click tracking) |
| `unsubscribed` | `warn` | Recipient clicked the unsubscribe link (requires unsubscribe tracking) |
| `complained` | `warn` | Recipient marked the email as spam |
| `stored` | `info` | An incoming message was stored (inbound routing) |
| `list_member_uploaded` | `info` | Member successfully added to a mailing list |
| `list_member_upload_error` | `warn` | Error adding member to a mailing list |
| `list_uploaded` | `info` | Batch member upload completed for a mailing list |

### Failed Event Severity

| Severity | Description |
|----------|-------------|
| `temporary` | Soft bounce — Mailgun will retry delivery |
| `permanent` | Hard bounce — message dropped, no retry |

### Failed Event Reason Values

| Reason | Description |
|--------|-------------|
| `bounce` | Recipient server rejected the message |
| `espblock` | ESP-level block |
| `suppress-bounce` | Recipient previously bounced (in suppression list) |
| `suppress-complaint` | Recipient previously complained |
| `suppress-unsubscribe` | Recipient previously unsubscribed |
| `greylisted` | Temporary greylisting by recipient server |
| `old` | Message expired (exceeded retry window) |
| `generic` | Generic failure |

---

## Event Object Schemas

### Common Fields (all events)

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique event ID (unique within a day) |
| `event` | string | Event type name (lowercase) |
| `timestamp` | number | Unix epoch seconds with microsecond precision (e.g., `1376325780.160809`) |
| `log-level` | string | `"info"`, `"warn"`, or `"error"` |

### Accepted Event

```
{
  id, event, timestamp, log-level,           // common
  method: string,                            // "http" or "smtp"
  envelope: Envelope,
  message: Message,
  flags: Flags,
  recipient: string,                         // individual recipient
  recipient-domain: string,
  originating-ip: string,                    // sender's IP
  tags: string[],
  campaigns: Campaign[],
  user-variables: object,
  storage: Storage
}
```

### Delivered Event

```
{
  id, event, timestamp, log-level,           // common
  envelope: Envelope,
  message: Message,
  flags: Flags,
  recipient: string,
  recipient-domain: string,
  recipient-provider: string,                // e.g., "Gmail", "Yahoo"
  method: string,
  tags: string[],
  campaigns: Campaign[],
  storage: Storage,
  delivery-status: DeliveryStatus,
  user-variables: object
}
```

### Failed Event

```
{
  id, event, timestamp, log-level,           // common
  envelope: Envelope,
  message: Message,
  flags: Flags,
  recipient: string,
  recipient-domain: string,
  recipient-provider: string,
  method: string,
  tags: string[],
  campaigns: Campaign[],
  storage: Storage,
  delivery-status: DeliveryStatus,
  severity: "temporary" | "permanent",
  reason: string,                            // e.g., "bounce", "suppress-bounce"
  user-variables: object
}
```

### Rejected Event

```
{
  id, event, timestamp, log-level,           // common
  reject: { reason: string, description: string },
  message: Message,
  storage: Storage,
  flags: Flags,
  tags: string[],
  campaigns: Campaign[],
  user-variables: object
}
```

### Opened Event

```
{
  id, event, timestamp, log-level,           // common
  message: { headers: { message-id: string } },
  campaigns: Campaign[],
  mailing-list: MailingList | null,
  recipient: string,
  recipient-domain: string,
  tags: string[],
  ip: string,                                // recipient's IP
  client-info: ClientInfo,
  geolocation: GeoLocation,
  user-variables: object
}
```

### Clicked Event

```
{
  id, event, timestamp, log-level,           // common
  url: string,                               // the clicked URL
  message: { headers: { message-id: string } },
  campaigns: Campaign[],
  mailing-list: MailingList | null,
  recipient: string,
  recipient-domain: string,
  tags: string[],
  ip: string,
  client-info: ClientInfo,
  geolocation: GeoLocation,
  user-variables: object
}
```

### Unsubscribed Event

```
{
  id, event, timestamp, log-level,           // common
  message: { headers: { message-id: string } },
  campaigns: Campaign[],
  mailing-list: MailingList | null,
  recipient: string,
  recipient-domain: string,
  tags: string[],
  ip: string,
  client-info: ClientInfo,
  geolocation: GeoLocation,
  user-variables: object
}
```

### Complained Event

```
{
  id, event, timestamp, log-level,           // common
  message: Message,
  campaigns: Campaign[],
  recipient: string,
  tags: string[],
  user-variables: object
}
```

### Stored Event

```
{
  id, event, timestamp, log-level,           // common
  message: Message,
  storage: Storage,
  flags: Flags,
  tags: string[],
  campaigns: Campaign[],
  user-variables: object
}
```

---

## Supporting Schemas

### Envelope

```
{
  sender: string,              // envelope sender
  transport: string,           // "http" or "smtp"
  targets: string,             // envelope recipient(s)
  sending-ip: string,          // IP used for delivery
  mail-from: string,           // MAIL FROM address
  sending-host: string         // sending hostname
}
```

### Message

```
{
  headers: {
    to: string,                // To header
    from: string,              // From header
    message-id: string,        // Message-ID header (REQUIRED)
    subject: string            // Subject header
  },
  attachments: [{
    filename: string,
    content-type: string,
    size: integer              // bytes
  }],
  recipients: string[],        // all recipients (for stored events)
  size: integer                // total message size in bytes
}
```

### Storage

```
{
  key: string,                 // storage key for retrieval
  url: string                  // full URL to retrieve stored message
}
```

### Flags

```
{
  is-authenticated: boolean,
  is-test-mode: boolean,
  is-system-test: boolean,
  is-routed: boolean,
  is-big: boolean,
  is-delayed-bounce: boolean
}
```

### DeliveryStatus

```
{
  code: integer,               // SMTP status code (e.g., 250, 550)
  attempt-no: integer,         // delivery attempt number
  message: string,             // SMTP response message
  description: string,         // human-readable description
  enhanced-code: string,       // enhanced SMTP code (e.g., "5.1.1")
  mx-host: string,             // MX host used
  session-seconds: number,     // SMTP session duration
  retry-seconds: integer,      // seconds until next retry
  tls: boolean,                // TLS used
  certificate-verified: boolean,
  utf8: boolean,               // UTF-8 used
  bounce-type: string          // "hard", "soft"
}
```

### ClientInfo

```
{
  client-type: string,         // "browser", "mobile browser", "client", "robot"
  client-os: string,           // e.g., "Windows 10"
  client-name: string,         // e.g., "Chrome"
  device-type: string,         // "desktop", "mobile", "tablet", "unknown"
  user-agent: string,
  bot: string                  // bot identifier if applicable
}
```

### GeoLocation

```
{
  country: string,
  region: string,
  city: string
}
```

### MailingList

```
{
  address: string,             // list email address
  list-id: string,             // list identifier
  sid: string                  // subscription ID
}
```

### Campaign

```
{
  id: string,
  name: string
}
```

### Paging

```
{
  first: string,               // full URL to first page
  last: string,                // full URL to last page
  next: string,                // full URL to next page
  previous: string             // full URL to previous page
}
```

All four paging keys are always present. Page URLs contain opaque base64-encoded tokens as path segments (e.g., `/v3/domain.com/events/W3siY...`). Clients follow these URLs directly — they do not construct page tokens.

---

## Pagination Behavior

### URL-based cursor pagination

The Events API uses opaque URL-based pagination, not offset/limit:

1. Initial request: `GET /v3/{domain}/events?begin=...&limit=100`
2. Response includes `paging.next` URL
3. Client follows `paging.next` to get the next page
4. Empty `items` array signals the end of results

### Polling pattern

For real-time event monitoring:

1. Start with `begin=<now>&ascending=yes`
2. Follow `paging.next` repeatedly
3. When `items` is empty, wait 15+ seconds and retry the same `next` URL
4. New events will appear as they're generated

### Page token format for the mock

The mock should generate opaque page tokens that encode:
- The current position (timestamp + event ID for stable cursor)
- The original query filters
- Sort direction

A simple approach: base64-encode a JSON object with these fields, use it as the path segment.

### Deduplication

Events from the same timestamp may span pages. Clients should handle potential duplicates at page boundaries (using the event `id` for deduplication).

---

## Event Generation

### Lifecycle: message send → events

When the mock accepts a message via POST `/v3/{domain}/messages`:

1. **`accepted` event** — Generated immediately for each recipient. Contains full message metadata and storage key.
2. **`delivered` event** — Generated after a configurable delay (default: immediate or ~1 second). Contains delivery status with SMTP 250 code.
3. Optionally, the mock can be configured to generate `failed` events instead of `delivered` (for testing failure scenarios).

### Event generation rules

| Trigger | Events Generated |
|---------|-----------------|
| Message accepted (per recipient) | `accepted` |
| Message delivered (per recipient) | `delivered` with `delivery-status.code: 250` |
| Message fails permanently | `failed` with `severity: "permanent"`, appropriate `reason` |
| Message fails temporarily | `failed` with `severity: "temporary"` |
| Recipient in bounce suppression list | `failed` with `reason: "suppress-bounce"` |
| Recipient in complaint suppression list | `failed` with `reason: "suppress-complaint"` |
| Recipient in unsubscribe suppression list | `failed` with `reason: "suppress-unsubscribe"` |
| Open tracking pixel loaded | `opened` |
| Link clicked | `clicked` |
| Unsubscribe link clicked | `unsubscribed` |
| Spam complaint | `complained` |

### Configurable behavior

The mock should support configuration flags to control event generation:

| Setting | Default | Description |
|---------|---------|-------------|
| `auto_deliver` | `true` | Automatically generate `delivered` events after `accepted` |
| `delivery_delay_ms` | `0` | Delay before generating `delivered` event (milliseconds) |
| `default_delivery_status_code` | `250` | Default SMTP code for delivered events |

### Mock-specific: manual event triggers

The mock should provide additional endpoints (not part of real Mailgun API) to manually trigger events for testing:

- **POST `/mock/events/{domain}/deliver/{message_id}`** — Generate a `delivered` event
- **POST `/mock/events/{domain}/fail/{message_id}`** — Generate a `failed` event (with configurable severity/reason)
- **POST `/mock/events/{domain}/open/{message_id}`** — Generate an `opened` event
- **POST `/mock/events/{domain}/click/{message_id}`** — Generate a `clicked` event
- **POST `/mock/events/{domain}/unsubscribe/{message_id}`** — Generate an `unsubscribed` event
- **POST `/mock/events/{domain}/complain/{message_id}`** — Generate a `complained` event

These allow test code to simulate the full email lifecycle without waiting for real recipient actions.

---

## Mock Behavior

### What the mock does

1. **Auto-generate events on send:** When a message is accepted, immediately create `accepted` events (one per recipient). Optionally auto-create `delivered` events.
2. **Store events:** Maintain an in-memory event log per domain, queryable via GET `/v3/{domain}/events`.
3. **Support filtering:** Filter events by `event`, `from`, `to`, `recipient`, `subject`, `message-id`, `tags`, `severity`, and time range (`begin`/`end`).
4. **URL-based pagination:** Generate opaque page tokens in `paging` URLs. All four paging keys (`first`, `last`, `next`, `previous`) must be present.
5. **Event polling:** Support ascending traversal with empty-page signaling for polling patterns.
6. **Link events to messages:** Each event references the message via `message.headers.message-id` and `storage.key`.
7. **Suppression integration:** When sending, check suppression lists (bounces, complaints, unsubscribes) and generate `failed` events with appropriate `reason` instead of `delivered`.
8. **Realistic event IDs:** Generate unique event IDs (short alphanumeric strings).
9. **Realistic timestamps:** Use Unix epoch seconds with microsecond precision (float64).
10. **Manual event triggers:** Provide mock-specific endpoints to simulate opens, clicks, unsubscribes, and complaints for testing.
11. **Log level mapping:** Set `log-level` correctly per event type (info/warn/error).

### What the mock skips

- The Logs API (`POST /v1/analytics/logs`) — stub with a basic response if needed, but not a primary target
- Complex filter expression parsing (AND/OR/NOT within a field) — support simple equality matching
- Real geolocation data (return static mock values for opened/clicked events)
- Real client-info detection (return static mock values)
- Campaign tracking (accept `campaigns` field, return it in events, but no campaign management)
- Metric aggregation and unique event deduplication (Logs API feature)
- Event retention enforcement (keep all events; optionally support a configurable max)
- Rate limiting on the events endpoint

### Storage model

Each event should capture:

```
{
  id: string,                    // unique event ID
  event: string,                 // event type
  timestamp: number,             // Unix epoch with microseconds
  log-level: string,             // "info" | "warn" | "error"
  domain: string,                // domain name (for routing)

  // Message reference (most events)
  message_id: string,            // links to the message
  storage_key: string,           // storage key for retrieval

  // Recipient info
  recipient: string,
  recipient_domain: string,

  // Full event payload (stored as JSON, varies by event type)
  payload: object                // the complete event object as returned by the API
}
```

Events are indexed by:
- `domain` + `timestamp` (primary query path)
- `domain` + `message_id` (lookup by message)
- `domain` + `event` (filter by type)

---

## SDK Compatibility Notes

### Node.js (mailgun.js)

```javascript
// Query events
const events = await mg.events.get('domain.com', {
  event: 'delivered',
  begin: 'Tue, 01 Jan 2024 00:00:00 -0000',
  limit: 100
});
// events.items = [...]
// events.pages = { first: {...}, last: {...}, next: {...}, previous: {...} }

// Pagination — follow next page
const nextPage = await mg.events.get('domain.com', { page: events.pages.next.page });
```

The Node SDK extends `NavigationThruPages` — it extracts page tokens from full URLs by splitting on a URL separator, then passes the token as a `page` query parameter which gets appended as a path segment `/events/<token>`.

### Python (mailgun-python)

```python
# Query events
response = client.events.get(domain="domain.com", filters={
    "event": "rejected OR failed",
    "begin": "Tue, 24 Nov 2020 09:00:00 -0000",
    "ascending": "yes",
    "limit": 10,
    "recipient": "user@example.com"
})
events = response.json()

# Pagination — follow paging.next URL manually
next_url = events["paging"]["next"]
```

### Ruby (mailgun-ruby)

```ruby
# Stateful event iteration
events = Mailgun::Events.new(mg_client, "domain.com")
events.next(event: "delivered")  # first page
events.next  # follows internal paging state

# Enumerable iteration (auto-paginates)
events.each do |event|
  puts event["event"]
end
```

The Ruby SDK stores `@paging_next` / `@paging_previous` internally. It extracts page tokens from URLs via regex: `URI.parse(url).path[/v\d/<domain>/events/(.+)/, 1]`.

### Go (mailgun-go)

```go
// List events with iterator
iter := mg.ListEvents(&mailgun.ListEventOptions{
  Begin: time.Now().Add(-24 * time.Hour),
  Limit: 100,
  Filter: map[string]string{"event": "delivered"},
})
var events []mailgun.Event
for iter.Next(ctx, &events) {
  for _, e := range events {
    // e is a typed event struct
  }
}

// Poll for events
poller := mg.PollEvents(&mailgun.ListEventOptions{
  Begin: time.Now(),
  PollInterval: 30 * time.Second,
})
```

The Go SDK parses events into typed structs (`Accepted`, `Delivered`, `Failed`, etc.) via a `ParseEvents()` function that dispatches on the `event` field.

---

## Test Scenarios

1. **Basic event query:** GET events → 200, returns items + paging
2. **Events after send:** POST message, GET events → `accepted` event present
3. **Auto-delivered events:** POST message with auto_deliver=true → `accepted` + `delivered` events
4. **Filter by event type:** GET events?event=delivered → only delivered events
5. **Filter by recipient:** GET events?recipient=user@example.com → only events for that recipient
6. **Filter by message-id:** GET events?message-id=<id> → only events for that message
7. **Filter by tags:** GET events?tags=welcome → only events with that tag
8. **Filter by severity:** GET events?severity=permanent → only permanent failures
9. **Time range filtering:** GET events?begin=<t1>&end=<t2> → events within range
10. **Ascending order:** GET events?ascending=yes → oldest first
11. **Pagination:** GET events?limit=1 → 1 item, follow `paging.next` → next item
12. **Empty page:** Paginate past all events → empty `items` array
13. **Paging URLs present:** Response always has `paging.first`, `paging.last`, `paging.next`, `paging.previous`
14. **Suppression integration:** Add address to bounce list, send message → `failed` event with `reason: "suppress-bounce"`
15. **Manual event trigger:** POST mock trigger for `opened` → event appears in event log
16. **Event fields correct:** Verify `accepted` event has `envelope`, `message`, `storage`, `flags` fields
17. **Delivery status fields:** Verify `delivered` event has `delivery-status` with `code: 250`
18. **Failed event fields:** Verify `failed` event has `severity`, `reason`, `delivery-status`
19. **Event ID uniqueness:** Multiple events → all have unique `id` values
20. **Timestamp precision:** Event timestamps have microsecond precision (float64)
21. **Log level mapping:** `accepted` → `info`, `failed` (permanent) → `error`, `complained` → `warn`
22. **Multiple recipients:** Send to 3 recipients → 3 separate `accepted` events
23. **Event type OR filter:** GET events?event=rejected+OR+failed → both types returned
24. **Auth failure:** GET events without valid auth → 401

## References

- **OpenAPI spec:** `mailgun.yaml` — events endpoint (`/v3/{domain_name}/events`), logs endpoint (`/v1/analytics/logs`), stored messages (`/v3/domains/{domain_name}/messages/{storage_key}`); schemas: `EventResponse`, `EventType`, `EventSeverityType`, `DeliveryStatusObject`, `LogsRequest`, `LogsResponse`, `LogEvent`
- **API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/events
- **Event structure:** https://documentation.mailgun.com/docs/mailgun/user-manual/events/event-structure
- **Event types help center:** https://help.mailgun.com/hc/en-us/articles/203661564-Events-Logs-Message-Event-Types
- **Search queries help center:** https://help.mailgun.com/hc/en-us/articles/203879300-Events-Logs-Search-Queries
- **Tracking failures:** https://documentation.mailgun.com/docs/mailgun/user-manual/tracking-messages/tracking-failures
- **Go SDK events package:** https://pkg.go.dev/github.com/mailgun/mailgun-go/v4/events
- **Go SDK source:** https://github.com/mailgun/mailgun-go/blob/main/events.go
- **Node SDK:** https://github.com/mailgun/mailgun.js — `EventClient` class in `/lib/Classes/Events.ts`
- **Ruby SDK:** https://github.com/mailgun/mailgun-ruby — `Mailgun::Events` in `/lib/mailgun/events/events.rb`
- **Python SDK:** https://github.com/mailgun/mailgun-python — events examples in `/mailgun/examples/events_examples.py`
