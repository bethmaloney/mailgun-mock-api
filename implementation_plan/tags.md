# Tags

Tag management, tag-scoped statistics, and aggregate stats for the mock Mailgun service.

## Overview

Tags are custom string labels attached to outgoing messages via the `o:tag` parameter at send time. Tags are **auto-created** — there is no explicit "create tag" endpoint. When a message is sent with a tag that doesn't exist yet, Mailgun automatically starts tracking it. Tags let users categorize messages and view per-tag performance statistics.

**Key concepts:**
- Tags are scoped to a domain
- Tags are attached at send time via `o:tag` (up to 3 per message per legacy docs, up to 10 per Ruby SDK enforcement)
- Tags are **auto-created** when first used on a message — no create endpoint
- Each tag can have an optional `description` (editable via update)
- Tag stats aggregate event counts (accepted, delivered, opened, clicked, etc.) over time
- Default **5,000 tags** per domain (configurable limit)
- Tag names are **case insensitive**, ASCII only, max 128 characters

**Two API versions exist:**
- **Legacy v3 API** (per-domain, used by all current SDKs) — the mock should fully support this
- **New v1 Analytics API** (account-level) — the mock should stub this

## API Endpoints

### Legacy v3 Tag CRUD

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v3/{domain}/tags` | List all tags (paginated) |
| `GET` | `/v3/{domain}/tags/{tag}` | Get a single tag |
| `PUT` | `/v3/{domain}/tags/{tag}` | Update tag description |
| `DELETE` | `/v3/{domain}/tags/{tag}` | Delete a tag and its stats |

### Legacy v3 Tag Stats

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v3/{domain}/tags/{tag}/stats` | Get time-series stats for a tag |
| `GET` | `/v3/{domain}/tags/{tag}/stats/aggregates/countries` | Get stats aggregated by country |
| `GET` | `/v3/{domain}/tags/{tag}/stats/aggregates/providers` | Get stats aggregated by provider |
| `GET` | `/v3/{domain}/tags/{tag}/stats/aggregates/devices` | Get stats aggregated by device |

### Legacy v3 Reference & Limits

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v3/domains/{domain}/limits/tag` | Get tag count and limit for domain |
| `GET` | `/v3/domains/{domain}/tag/devices` | List supported device types |
| `GET` | `/v3/domains/{domain}/tag/providers` | List supported provider names |
| `GET` | `/v3/domains/{domain}/tag/countries` | List supported country codes |

### Domain Stats (not tag-scoped, but related)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v3/{domain}/stats/total` | Get aggregated domain-level stats |

### New v1 Analytics Tags API (Stub)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/analytics/tags` | List/search tags (JSON body) |
| `PUT` | `/v1/analytics/tags` | Update tag description (JSON body) |
| `DELETE` | `/v1/analytics/tags` | Delete tag (JSON body) |
| `GET` | `/v1/analytics/tags/limits` | Get account-wide tag limits |

---

## Data Models

### Tag

```typescript
{
  tag: string,            // tag name
  description: string,    // human-readable description (default: "")
  "first-seen": string,   // ISO 8601 datetime or null — when tag was first used
  "last-seen": string,    // ISO 8601 datetime or null — when tag was last used
}
```

**Notes:**
- Legacy API uses **hyphenated keys** (`first-seen`, `last-seen`) — not camelCase or snake_case
- The new v1 API uses **snake_case** (`first_seen`, `last_seen`) and adds `account_id`, `parent_account_id`, `account_name`, and `metrics` fields

### Tag List Response

```typescript
{
  items: Tag[],
  paging: {
    first: string,    // URL
    next: string,     // URL
    previous: string, // URL
    last: string,     // URL
  }
}
```

### Tag Limits

```typescript
{
  id: string,      // domain identifier
  limit: number,   // maximum tags allowed (default: 5000)
  count: number,   // current number of tags
}
```

### Stats Response (Time-Series)

```typescript
{
  tag: string,            // tag name
  description: string,    // tag description
  start: string,          // RFC 2822 date, e.g. "Mon, 16 Mar 2024 00:00:00 UTC"
  end: string,            // RFC 2822 date
  resolution: string,     // "hour", "day", or "month"
  stats: StatsEntry[]     // array of time-bucketed stats
}
```

### Stats Entry

Each entry in the `stats` array represents one time bucket:

```typescript
{
  time: string,           // RFC 2822 date for this bucket
  accepted: {
    incoming: number,
    outgoing: number,
    total: number,
  },
  delivered: {
    smtp: number,
    http: number,
    total: number,
  },
  failed: {
    temporary: {
      espblock: number,
    },
    permanent: {
      "suppress-bounce": number,
      "suppress-unsubscribe": number,
      "suppress-complaint": number,
      bounce: number,
      "delayed-bounce": number,
      total: number,
    },
  },
  stored: { total: number },
  opened: { total: number },
  clicked: { total: number },
  unsubscribed: { total: number },
  complained: { total: number },
}
```

### Aggregate Stats Response

Country, provider, and device aggregations share a similar shape — a map of dimension values to event-type counts:

```typescript
// GET /v3/{domain}/tags/{tag}/stats/aggregates/countries
{
  tag: string,
  countries: {
    [countryCode: string]: {
      clicked: number,
      complained: number,
      opened: number,
      unique_clicked: number,
      unique_opened: number,
      unsubscribed: number,
    }
  }
}

// GET /v3/{domain}/tags/{tag}/stats/aggregates/providers
{
  tag: string,
  providers: {
    [providerName: string]: {
      accepted: number,
      clicked: number,
      complained: number,
      delivered: number,
      opened: number,
      unique_clicked: number,
      unique_opened: number,
      unsubscribed: number,
    }
  }
}

// GET /v3/{domain}/tags/{tag}/stats/aggregates/devices
{
  tag: string,
  devices: {
    [deviceType: string]: {
      clicked: number,
      complained: number,
      opened: number,
      unique_clicked: number,
      unique_opened: number,
      unsubscribed: number,
    }
  }
}
```

### Reference Lists Response

Device types, provider names, and country codes share the same response shape:

```typescript
{
  items: string[]  // e.g. ["desktop", "mobile", "tablet", "other"]
}
```

---

## Endpoint Details

### GET `/v3/{domain}/tags` — List Tags

**Query parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `limit` | integer | no | Number of tags to return (default: 100) |
| `page` | string | no | Page direction: `"first"`, `"last"`, `"next"`, `"prev"` |
| `tag` | string | no | Pivot tag name for pagination (marks end of current page) |
| `prefix` | string | no | Return only tags starting with this prefix |

**Response** (200):
```json
{
  "items": [
    {
      "tag": "newsletter",
      "description": "Monthly newsletter",
      "first-seen": "2024-01-15T10:00:00Z",
      "last-seen": "2024-06-01T14:30:00Z"
    },
    {
      "tag": "transactional",
      "description": "",
      "first-seen": "2024-02-01T08:00:00Z",
      "last-seen": "2024-06-02T12:00:00Z"
    }
  ],
  "paging": {
    "first": "https://api.mailgun.net/v3/example.com/tags?page=first",
    "next": "https://api.mailgun.net/v3/example.com/tags?page=next&tag=transactional",
    "previous": "https://api.mailgun.net/v3/example.com/tags?page=prev&tag=newsletter",
    "last": "https://api.mailgun.net/v3/example.com/tags?page=last"
  }
}
```

**Notes:**
- Pagination uses the `tag` query parameter as the cursor/pivot (not `p` like templates)
- Tags are returned in alphabetical order
- The `prefix` filter allows searching by tag name prefix (useful for UI autocomplete)

### GET `/v3/{domain}/tags/{tag}` — Get Tag

**Response** (200):
```json
{
  "tag": "newsletter",
  "description": "Monthly newsletter",
  "first-seen": "2024-01-15T10:00:00Z",
  "last-seen": "2024-06-01T14:30:00Z"
}
```

### PUT `/v3/{domain}/tags/{tag}` — Update Tag

Updates the tag's description. This is the only mutable field.

**Request** (`multipart/form-data` or `application/x-www-form-urlencoded`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | yes | Updated description for the tag |

**Response** (200):
```json
{
  "message": "Tag updated"
}
```

### DELETE `/v3/{domain}/tags/{tag}` — Delete Tag

Removes the tag and all its associated counter data.

**Response** (200):
```json
{
  "message": "Tag has been removed"
}
```

### GET `/v3/domains/{domain}/limits/tag` — Get Tag Limits

**Response** (200):
```json
{
  "id": "example.com",
  "limit": 5000,
  "count": 42
}
```

---

### GET `/v3/{domain}/tags/{tag}/stats` — Get Tag Stats

Returns time-series statistics for a specific tag.

**Query parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `event` | string | yes | Event type(s) to retrieve. Can be specified multiple times. Values: `accepted`, `delivered`, `failed`, `opened`, `clicked`, `unsubscribed`, `complained`, `stored` |
| `start` | string | no | Start date (RFC 2822 or Unix epoch). Default: 7 days ago |
| `end` | string | no | End date (RFC 2822 or Unix epoch). Default: current time |
| `resolution` | string | no | Time bucket size: `"hour"`, `"day"`, `"month"`. Default: `"day"` |
| `duration` | string | no | Period string (e.g. `"1m"`, `"7d"`, `"24h"`). If provided, overrides `start` (calculated backwards from `end`). |

**Response** (200):
```json
{
  "tag": "newsletter",
  "description": "Monthly newsletter",
  "start": "Mon, 16 Mar 2024 00:00:00 UTC",
  "end": "Mon, 23 Mar 2024 00:00:00 UTC",
  "resolution": "day",
  "stats": [
    {
      "time": "Mon, 16 Mar 2024 00:00:00 UTC",
      "accepted": { "incoming": 0, "outgoing": 50, "total": 50 },
      "delivered": { "smtp": 45, "http": 0, "total": 45 },
      "failed": {
        "temporary": { "espblock": 0 },
        "permanent": {
          "suppress-bounce": 1,
          "suppress-unsubscribe": 0,
          "suppress-complaint": 0,
          "bounce": 2,
          "delayed-bounce": 0,
          "total": 3
        }
      },
      "stored": { "total": 0 },
      "opened": { "total": 20 },
      "clicked": { "total": 8 },
      "unsubscribed": { "total": 1 },
      "complained": { "total": 0 }
    }
  ]
}
```

**Notes:**
- All stats are calculated in UTC timezone
- The `event` parameter determines which event types appear in the response — only requested event types are populated, others may be zeroed or omitted
- The `duration` parameter format is `[0-9]+[m,d,h]` (e.g., `1m` = 1 month, `7d` = 7 days, `24h` = 24 hours)

### GET `/v3/{domain}/stats/total` — Get Domain Stats

Same parameters and response structure as tag stats, but aggregated across all messages for the domain (not scoped to a tag). The response does not include `tag` or `description` fields.

**Response** (200):
```json
{
  "start": "Mon, 16 Mar 2024 00:00:00 UTC",
  "end": "Mon, 23 Mar 2024 00:00:00 UTC",
  "resolution": "day",
  "stats": [ ... ]
}
```

---

### GET `/v3/{domain}/tags/{tag}/stats/aggregates/countries` — Countries Aggregate

**Response** (200):
```json
{
  "tag": "newsletter",
  "countries": {
    "US": { "clicked": 100, "complained": 0, "opened": 250, "unique_clicked": 80, "unique_opened": 200, "unsubscribed": 2 },
    "GB": { "clicked": 30, "complained": 0, "opened": 70, "unique_clicked": 25, "unique_opened": 60, "unsubscribed": 0 }
  }
}
```

### GET `/v3/{domain}/tags/{tag}/stats/aggregates/providers` — Providers Aggregate

**Response** (200):
```json
{
  "tag": "newsletter",
  "providers": {
    "gmail.com": { "accepted": 300, "clicked": 80, "complained": 1, "delivered": 295, "opened": 180, "unique_clicked": 65, "unique_opened": 150, "unsubscribed": 1 },
    "yahoo.com": { "accepted": 100, "clicked": 20, "complained": 0, "delivered": 98, "opened": 50, "unique_clicked": 18, "unique_opened": 45, "unsubscribed": 0 }
  }
}
```

### GET `/v3/{domain}/tags/{tag}/stats/aggregates/devices` — Devices Aggregate

**Response** (200):
```json
{
  "tag": "newsletter",
  "devices": {
    "desktop": { "clicked": 60, "complained": 0, "opened": 150, "unique_clicked": 50, "unique_opened": 130, "unsubscribed": 1 },
    "mobile": { "clicked": 40, "complained": 1, "opened": 120, "unique_clicked": 35, "unique_opened": 100, "unsubscribed": 0 },
    "tablet": { "clicked": 10, "complained": 0, "opened": 30, "unique_clicked": 8, "unique_opened": 25, "unsubscribed": 0 }
  }
}
```

---

### GET `/v3/domains/{domain}/tag/devices` — List Supported Devices

Returns the list of device type values the mock recognizes.

**Response** (200):
```json
{
  "items": ["desktop", "mobile", "tablet", "other"]
}
```

### GET `/v3/domains/{domain}/tag/providers` — List Supported Providers

**Response** (200):
```json
{
  "items": ["gmail.com", "yahoo.com", "outlook.com", "hotmail.com", "aol.com", "other"]
}
```

### GET `/v3/domains/{domain}/tag/countries` — List Supported Countries

**Response** (200):
```json
{
  "items": ["US", "GB", "CA", "DE", "FR", "AU", "other"]
}
```

---

### New v1 Analytics Tags API (Stub)

These endpoints use JSON request/response bodies and are account-level (not domain-scoped).

#### POST `/v1/analytics/tags` — List/Search Tags

**Request** (`application/json`):
```json
{
  "tag": "newsletter",
  "include_subaccounts": true,
  "include_metrics": true,
  "pagination": {
    "sort": "last_seen:desc",
    "skip": 0,
    "limit": 10,
    "include_total": true
  }
}
```

**Response** (200):
```json
{
  "items": [
    {
      "tag": "newsletter",
      "description": "Monthly newsletter",
      "first_seen": "2024-01-15T10:00:00Z",
      "last_seen": "2024-06-01T14:30:00Z",
      "account_id": "abc123",
      "parent_account_id": "",
      "account_name": "My Account",
      "metrics": {}
    }
  ],
  "pagination": {
    "sort": "last_seen:desc",
    "skip": 0,
    "limit": 10,
    "total": 1
  }
}
```

#### PUT `/v1/analytics/tags` — Update Tag

**Request** (`application/json`):
```json
{
  "tag": "newsletter",
  "description": "Updated description"
}
```

**Response** (200):
```json
{
  "message": "Tag updated"
}
```

#### DELETE `/v1/analytics/tags` — Delete Tag

**Request** (`application/json`):
```json
{
  "tag": "newsletter"
}
```

**Response** (200):
```json
{
  "message": "Tag has been removed"
}
```

#### GET `/v1/analytics/tags/limits` — Get Tag Limits

**Response** (200):
```json
{
  "limit": 5000,
  "count": 42,
  "limit_reached": false
}
```

---

## Integration with Message Sending

Tags are attached to messages at send time via the `o:tag` parameter (already documented in [messages.md](./messages.md)):

| Field | Type | Description |
|-------|------|-------------|
| `o:tag` | string or string[] | Up to 3 tags per message (legacy docs) or 10 (Ruby SDK enforcement) |

When the mock accepts a message with tags:
1. **Store tags on the message record** — the tags become part of the message metadata
2. **Auto-create tag entries** — if a tag doesn't exist for this domain yet, create it with `first-seen` set to now
3. **Update `last-seen`** — for existing tags, update `last-seen` to now
4. **Events carry tags** — all events generated for tagged messages include the tags in the event payload (already documented in [events-and-logs.md](./events-and-logs.md))
5. **Tag stats increment** — when events are generated, the corresponding tag stat counters increment

### Tag on Stored Messages

When retrieving a stored message, tags appear in the `X-Mailgun-Tag` header:
- **Field:** `X-Mailgun-Tag`
- **Type:** string (JSON array when multiple tags)

---

## Pagination

Tag lists use the same cursor-based pagination as other Mailgun list endpoints, but with `tag` as the pivot parameter (not `p`):

```json
{
  "paging": {
    "first": "https://api.mailgun.net/v3/{domain}/tags?page=first",
    "next": "https://api.mailgun.net/v3/{domain}/tags?page=next&tag={last_tag}",
    "previous": "https://api.mailgun.net/v3/{domain}/tags?page=prev&tag={first_tag}",
    "last": "https://api.mailgun.net/v3/{domain}/tags?page=last"
  }
}
```

Uses the shared `PagingResponse` schema (same as suppressions, events, templates, etc.).

---

## Error Cases

| Scenario | Status | Response |
|----------|--------|----------|
| Tag not found | 404 | `{"message": "tag not found"}` |
| Domain not found | 404 | `{"message": "domain not found"}` |
| Missing required `event` param on stats | 400 | `{"message": "event is required"}` |
| Invalid event type | 400 | `{"message": "invalid event type"}` |
| Tag limit reached (on auto-create) | — | Silently ignored or logged; messages are still accepted |

---

## Mock Behavior Notes

### What the mock should do

1. **Tag auto-creation** — when a message is sent with `o:tag`, create the tag record if it doesn't exist; update `first-seen`/`last-seen` timestamps
2. **Tag CRUD** — list (with pagination and prefix filter), get, update description, delete
3. **Tag stats from events** — derive stats by counting events generated for tagged messages, bucketed by `resolution` (hour/day/month)
4. **Domain stats** — aggregate stats across all messages for a domain (not tag-scoped), same response shape
5. **Tag limits** — return configurable limit (default 5,000) and current count
6. **Prefix filtering** — support `prefix` parameter on list endpoint for filtering tags by name prefix
7. **Pagination** — cursor-based pagination with `tag` as pivot, `limit` for page size

### What the mock should stub

1. **Aggregate stats** (countries/providers/devices) — return empty objects or minimal static data; tracking real country/provider/device data from mock messages is overkill
2. **Reference lists** (devices/providers/countries) — return a static hardcoded list of common values
3. **v1 Analytics Tags API** — accept requests, return correct response shapes, but delegate to the same underlying storage as v3
4. **Stats filtering** (by provider/device/country) — accept the parameters but ignore them when computing stats

### What the mock can skip

- **Real-time stat aggregation** — stats can be computed on-demand from stored events rather than pre-aggregated
- **UTC timezone enforcement** — the mock can use server-local time; precision doesn't matter for testing
- **Duration string parsing** — accept `duration` parameter but can treat it as equivalent to setting `start` = `end` minus duration
- **Stats for deleted tags** — real Mailgun may retain stats after tag deletion; the mock can delete everything

---

## Internal Storage Schema

```typescript
interface StoredTag {
  domain: string;           // owning domain
  tag: string;              // tag name
  description: string;      // user-set description (default: "")
  firstSeen: Date | null;   // when first used on a message
  lastSeen: Date | null;    // when last used on a message
}
```

Stats are **derived from stored events** rather than pre-aggregated. When the stats endpoint is called, the mock queries events for the given domain + tag + time range, then buckets and counts them by event type and resolution.

If pre-aggregation is desired for performance, a counter structure could be added:

```typescript
interface TagStatCounter {
  domain: string;
  tag: string;
  bucket: string;           // time bucket key, e.g. "2024-03-16" for day resolution
  resolution: string;       // "hour", "day", "month"
  accepted_incoming: number;
  accepted_outgoing: number;
  delivered_smtp: number;
  delivered_http: number;
  failed_temporary_espblock: number;
  failed_permanent_bounce: number;
  failed_permanent_suppress_bounce: number;
  failed_permanent_suppress_unsubscribe: number;
  failed_permanent_suppress_complaint: number;
  failed_permanent_delayed_bounce: number;
  stored: number;
  opened: number;
  clicked: number;
  unsubscribed: number;
  complained: number;
}
```

---

## OpenAPI Spec Path Discrepancy

The OpenAPI spec (`mailgun.yaml`) defines tag endpoints using **singular** paths with query parameters:
- `GET /v3/{domain}/tag?tag={name}` (singular, query param)
- `GET /v3/{domain}/tag/stats?tag={name}` (singular, query param)

However, all three client SDKs (Node.js, Ruby, Python) use **plural** paths with path parameters:
- `GET /v3/{domain}/tags/{tag}` (plural, path param)
- `GET /v3/{domain}/tags/{tag}/stats` (plural, path param)

**Recommendation for the mock:** Support **both** path forms. Register routes for:
- `/v3/{domain}/tags/{tag}` (SDKs pattern — primary)
- `/v3/{domain}/tag?tag={name}` (OpenAPI spec pattern — fallback)

This ensures compatibility with both SDK-generated requests and any clients following the OpenAPI spec directly.

---

## Test Scenarios

1. **Tag auto-creation on send** — send a message with `o:tag=newsletter` → tag "newsletter" appears in tag list with correct `first-seen`
2. **Multiple tags on send** — send with `o:tag=tag1&o:tag=tag2` → both tags created
3. **Tag `last-seen` update** — send two messages with same tag → `last-seen` reflects the later message
4. **List tags** — GET tags → paginated list in alphabetical order
5. **List tags with prefix** — GET tags with `prefix=news` → only tags starting with "news"
6. **Get single tag** — GET tag by name → returns tag object with `first-seen`/`last-seen`
7. **Update tag description** — PUT with `description` → tag description updated
8. **Delete tag** — DELETE → tag removed from list
9. **Tag not found** — GET/PUT/DELETE nonexistent tag → 404
10. **Tag limits** — GET limits → returns `limit` and `count`
11. **Tag stats (basic)** — send messages with tag → GET tag stats → counts match sent events
12. **Tag stats with resolution** — GET stats with `resolution=hour` → hourly buckets
13. **Tag stats with date range** — GET stats with `start`/`end` → only events in range
14. **Tag stats with duration** — GET stats with `duration=7d` → last 7 days
15. **Tag stats required event param** — GET stats without `event` → 400 error
16. **Domain stats** — GET `/v3/{domain}/stats/total` → aggregated stats across all tags
17. **Aggregate stats stub** — GET countries/providers/devices aggregates → returns valid response shape
18. **Reference lists** — GET devices/providers/countries → returns string arrays
19. **v1 list tags** — POST `/v1/analytics/tags` → returns tags in v1 format
20. **v1 update tag** — PUT `/v1/analytics/tags` → updates tag description
21. **v1 delete tag** — DELETE `/v1/analytics/tags` → removes tag
22. **v1 limits** — GET `/v1/analytics/tags/limits` → returns limits with `limit_reached` field
23. **Pagination** — list more tags than `limit` → correct `paging` URLs

---

## References

- **OpenAPI spec**: `mailgun.yaml` — tag endpoints at `/v3/{domain}/tag` and `/v3/domains/{domain}/tag/*` paths; schemas: `TagItem`, `TagListResponse`, `TagAggregateResponse`, `TagLimitItem`, `StatsResponse`, `StatTypesResponse`
- **API docs**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/tags
- **Stats docs**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/stats
- **Node.js SDK**: https://github.com/mailgun/mailgun.js — `lib/Classes/Tags/TagsClient.ts` (v1), `lib/Classes/Domains/domainsTags.ts` (v3 deprecated)
- **Ruby SDK**: https://github.com/mailgun/mailgun-ruby — `lib/mailgun/tags/analytics_tags.rb` (v1), `lib/mailgun/tags/tags.rb` (v3 deprecated)
- **Python SDK**: https://github.com/mailgun/mailgun-python — `mailgun/handlers/tags_handler.py` (v3), `mailgun/examples/tags_new_examples.py` (v1)
- **Go SDK stats types**: https://github.com/mailgun/mailgun-go — `stats.go` defines `Accepted`, `Delivered`, `Failed` (with `Temporary`/`Permanent` sub-structs)
