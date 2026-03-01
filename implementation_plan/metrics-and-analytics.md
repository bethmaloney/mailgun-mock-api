# Metrics & Analytics

Account-level stats, advanced metrics, usage metrics, and bounce classification for the mock Mailgun service.

## Overview

Mailgun provides two generations of analytics APIs:

1. **Legacy v3 Stats API** (deprecated) — simple time-series stats with GET requests, used by older SDK methods
2. **Current v1/v2 Metrics API** — advanced metrics with POST requests, JSON bodies, multi-dimensional filtering, and pagination

The legacy v3 Stats API has **per-domain** stats (covered in [tags.md](./tags.md) since the response shape is shared) and **account-level** stats (covered here). The v1 Metrics API is account-scoped and supports cross-domain, cross-tag, and cross-subaccount querying.

**For the mock:** The legacy v3 account-level stats endpoints should be fully supported (they share the same `StatsResponse` schema as domain/tag stats). The v1/v2 Metrics APIs should be stubbed — they accept requests and return valid response shapes, but compute metrics from the mock's stored events rather than pre-aggregated analytics.

**Cross-references:**
- Per-domain stats (`GET /v3/{domain}/stats/total`) — documented in [tags.md](./tags.md)
- Per-tag stats (`GET /v3/{domain}/tags/{tag}/stats`) — documented in [tags.md](./tags.md)
- Logs API (`POST /v1/analytics/logs`) — documented as a stub in [events-and-logs.md](./events-and-logs.md)
- v1 Analytics Tags API — documented in [tags.md](./tags.md)

---

## API Endpoints

### Legacy v3 Account-Level Stats (Deprecated)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v3/stats/total` | Account-wide aggregated stats |
| `GET` | `/v3/stats/filter` | Filtered/grouped account stats |
| `GET` | `/v3/stats/total/domains` | Per-domain stats for a single time resolution |

### Legacy v3 Domain Aggregates (Deprecated)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v3/{domain}/aggregates/providers` | Aggregate counts by email service provider |
| `GET` | `/v3/{domain}/aggregates/devices` | Aggregate counts by device type |
| `GET` | `/v3/{domain}/aggregates/countries` | Aggregate counts by country |

### Current v1 Metrics API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/analytics/metrics` | Query account metrics with dimensions/filters |
| `POST` | `/v1/analytics/usage/metrics` | Query account usage metrics |

### Current v2 Bounce Classification

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v2/bounce-classification/metrics` | Bounce classification metrics |

---

## Data Models

### StatsResponse (Shared with Tags/Domain Stats)

The same `StatsResponse` schema is used by all v3 stats endpoints. The full schema is documented in [tags.md](./tags.md). Summary:

```typescript
{
  start: string,          // RFC 2822 date
  end: string,            // RFC 2822 date
  resolution: string,     // "hour", "day", or "month"
  stats: StatsEntry[],    // time-bucketed event counts
  // Only present on tag stats:
  tag?: string,
  description?: string,
}
```

Each `StatsEntry` contains nested event-type counters (accepted, delivered, failed, stored, opened, clicked, unsubscribed, complained). See [tags.md](./tags.md) for the full `StatsEntry` schema.

### RegularMetricsResponse (v1 Metrics API)

```typescript
{
  start: string,                    // RFC 2822 date
  end: string,                      // RFC 2822 date
  resolution: string,               // "hour", "day", "month"
  duration: string,                 // e.g. "30d"
  dimensions: string[],             // requested dimensions
  items: MetricsItem[],             // result rows
  aggregates: {                     // only if include_aggregates=true
    metrics: MetricValues
  },
  pagination: {
    sort: string,                   // e.g. "time:asc"
    skip: number,
    limit: number,
    total: number,
  }
}
```

### MetricsItem

```typescript
{
  dimensions: [
    {
      dimension: string,            // e.g. "time", "domain", "tag"
      value: string,                // raw value
      display_value: string,        // human-readable value
    }
  ],
  metrics: MetricValues             // counts and rates
}
```

### MetricValues

All fields are optional. Counts are unsigned integers; rates are strings (percentage-like).

**Count metrics:**

| Metric | Description |
|--------|-------------|
| `accepted_count` | Total accepted messages (incoming + outgoing) |
| `accepted_incoming_count` | Incoming accepted (routes, forwards, mailing lists) |
| `accepted_outgoing_count` | Outgoing accepted (API sends) |
| `delivered_count` | Total delivered (SMTP + HTTP) |
| `delivered_smtp_count` | Delivered via SMTP |
| `delivered_http_count` | Delivered via HTTP (routes/forwards) |
| `delivered_optimized_count` | Delivered via Send Time Optimization |
| `stored_count` | Stored messages |
| `processed_count` | Post-acceptance (delivered + permanent_failed - webhooks - delayed_bounces) |
| `sent_count` | Delivered + failed minus suppressions |
| `opened_count` | Total opens |
| `clicked_count` | Total clicks |
| `unique_opened_count` | Deduplicated opens (per messageID/recipient, 7-day rolling) |
| `unique_clicked_count` | Deduplicated clicks |
| `unsubscribed_count` | Unsubscribe events |
| `complained_count` | Spam complaint events |
| `failed_count` | Total failures (permanent + temporary) |
| `temporary_failed_count` | Temporary failures (retried) |
| `permanent_failed_count` | Permanent failures (not retried) |
| `temporary_failed_esp_block_count` | Temporary ESP blocks |
| `permanent_failed_esp_block_count` | Permanent ESP blocks |
| `bounced_count` | Bounces excluding suppressions |
| `hard_bounces_count` | Hard bounces (invalid address) |
| `soft_bounces_count` | Soft bounces (temporary issues) |
| `delayed_bounce_count` | Initially delivered, later bounced permanently |
| `suppressed_bounces_count` | Suppressed due to prior bounce |
| `suppressed_unsubscribed_count` | Suppressed due to prior unsubscribe |
| `suppressed_complaints_count` | Suppressed due to prior complaint |
| `delivered_first_attempt_count` | Delivered on first attempt |
| `delayed_first_attempt_count` | Temporarily rejected, then retried |
| `delivered_subsequent_count` | Delivered on 2nd+ attempt |
| `delivered_two_plus_attempts_count` | Delivered after 2+ attempts |
| `webhook_count` | Failed webhook events |
| `rate_limit_count` | Rate-limited events |
| `permanent_failed_optimized_count` | Permanent failures for STO messages |
| `permanent_failed_old_count` | "Too old" permanent failures (max retries exhausted) |

**Rate metrics (strings):**

| Metric | Formula |
|--------|---------|
| `delivered_rate` | delivered_count / sent_count |
| `opened_rate` | opened_count / delivered_count |
| `clicked_rate` | clicked_count / delivered_count |
| `unique_opened_rate` | unique_opened_count / delivered_count |
| `unique_clicked_rate` | unique_clicked_count / delivered_count |
| `unsubscribed_rate` | unsubscribed_count / delivered_count |
| `complained_rate` | complained_count / delivered_count |
| `bounce_rate` | bounced_count / processed_count |
| `permanent_fail_rate` | permanent_failed_count / processed_count |
| `temporary_fail_rate` | temporary_failed_count / processed_count |
| `delayed_rate` | delivered_two_plus_attempts_count / delivered_count |
| `fail_rate` | failed_count / processed_count |

### Dimensions

Dimensions control how metrics are grouped in the response.

| Dimension | Description | Example values |
|-----------|-------------|----------------|
| `time` | Time bucket (resolution-dependent) | RFC 2822 datetime |
| `domain` | Sending domain | "example.com" |
| `tag` | Message tag | "newsletter" |
| `ip` | Sending IP | "192.237.158.61" |
| `ip_pool` | IP pool | "Transactional IP Pool" |
| `recipient_domain` | Recipient's domain | "gmail.com" |
| `recipient_provider` | Recipient's provider | "Gmail", "Outlook 365" |
| `country` | Recipient country | "US", "GB" |
| `subaccount` | Subaccount | "Subaccount 1" |
| `bot` | Bot detection | "Apple", "Gmail", "Generic", "None" |
| `device` | Device type | "desktop", "mobile", "tablet" |

**Limits:** Max 3 dimensions per query. Max 10 metrics per query.

### Filter Object

```typescript
{
  AND: [
    {
      attribute: string,         // e.g. "domain", "tag", "ip"
      comparator: string,        // e.g. "="
      values: [
        { label: string, value: string }
      ]
    }
  ]
}
```

### UsageMetricsResponse (v1 Usage API)

Same response shape as `RegularMetricsResponse` but with usage-specific metrics and dimensions.

**Usage dimensions:** `subaccount`, `time`

**Usage metrics:** `email_validation_count`, `email_validation_public_count`, `email_validation_valid_count`, `email_validation_single_count`, `email_validation_bulk_count`, `email_validation_list_count`, `email_preview_count`, `link_validation_count`, `seed_test_count`, `accessibility_count`, `archived_count`, `processed_count`

### BounceClassificationMetricsResponse (v2)

```typescript
{
  start: string,                    // RFC 2822 date
  end: string,                      // RFC 2822 date
  resolution: string,               // resolution label
  duration: string,
  dimensions: string[],
  items: BounceClassificationItem[],
  pagination: {
    sort: string,
    skip: number,
    limit: number,
    total: number,
  }
}
```

**Bounce classification dimensions:** `entity-name`, `domain.name`, `envelope.sending-ip`, `account.name`, `tags`, `tag`, `recipient-domain`, `group-id`, `criticality`, `severity`, `category`

**Bounce classification metrics:** `critical_bounce_count`, `non_critical_bounce_count`, `critical_delay_count`, `non_critical_delay_count`, `classified_failures_count`

---

## Endpoint Details

### GET `/v3/stats/total` — Account-Level Stats

Returns time-series stats aggregated across all domains in the account.

**Query parameters:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `event` | string | yes | — | Event type(s). Repeatable. Values: `accepted`, `delivered`, `failed`, `opened`, `clicked`, `unsubscribed`, `complained`, `stored` |
| `start` | string | no | 7 days ago | Start date (RFC 2822 or Unix epoch) |
| `end` | string | no | current time | End date (RFC 2822 or Unix epoch) |
| `resolution` | string | no | `"day"` | Time bucket: `"hour"`, `"day"`, `"month"` |
| `duration` | string | no | — | Period (e.g. `"1m"`, `"7d"`, `"24h"`). Overrides `start` |

**Response** (200): Same `StatsResponse` shape as domain stats (see [tags.md](./tags.md)).

```json
{
  "end": "Mon, 23 Mar 2024 00:00:00 UTC",
  "resolution": "day",
  "start": "Mon, 16 Mar 2024 00:00:00 UTC",
  "stats": [
    {
      "time": "Mon, 16 Mar 2024 00:00:00 UTC",
      "accepted": { "incoming": 5, "outgoing": 150, "total": 155 },
      "delivered": { "smtp": 140, "http": 5, "total": 145 },
      "failed": {
        "temporary": { "espblock": 1 },
        "permanent": {
          "suppress-bounce": 2, "suppress-unsubscribe": 1, "suppress-complaint": 0,
          "bounce": 3, "delayed-bounce": 0, "total": 6
        }
      },
      "stored": { "total": 3 },
      "opened": { "total": 90 },
      "clicked": { "total": 30 },
      "unsubscribed": { "total": 2 },
      "complained": { "total": 0 }
    }
  ]
}
```

**Notes:**
- The `event` parameter is repeatable: `?event=delivered&event=accepted`
- Only requested event types are populated in the response; others may be zeroed
- All dates in RFC 2822 format, UTC timezone

### GET `/v3/stats/filter` — Filtered/Grouped Account Stats

Returns account stats with optional filtering and grouping.

**Query parameters (in addition to those on `/v3/stats/total`):**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `filter` | string | no | — | Filter expression (e.g. `domain:my.example.com`) |
| `group` | string | no | — | Group by: `total`, `time`, `day`, `month`, `domain`, `ip`, `provider`, `tag`, `country` |

**Response** (200): Same `StatsResponse` shape.

### GET `/v3/stats/total/domains` — Per-Domain Stats Snapshot

Returns stats for all domains at a specific point in time.

**Query parameters:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `event` | string | yes | — | Event type(s). Repeatable. |
| `timestamp` | string | yes | — | The date/time of the resolution point to retrieve |
| `resolution` | string | no | `"day"` | Resolution: `"hour"`, `"day"`, `"month"` |
| `limit` | integer | no | — | Number of domains to skip (for pagination) |

**Response** (200): Same `StatsResponse` shape, with per-domain data.

---

### GET `/v3/{domain}/aggregates/providers` — Provider Aggregates

Returns aggregate counts by email service provider for a domain.

**Response** (200):
```json
{
  "items": {
    "gmail.com": { "accepted": 300, "delivered": 295, "clicked": 80, "opened": 180 },
    "yahoo.com": { "accepted": 100, "delivered": 98, "clicked": 20, "opened": 50 }
  }
}
```

### GET `/v3/{domain}/aggregates/devices` — Device Aggregates

**Response** (200):
```json
{
  "items": {
    "desktop": { "clicked": 60, "opened": 150 },
    "mobile": { "clicked": 40, "opened": 120 },
    "tablet": { "clicked": 10, "opened": 30 }
  }
}
```

### GET `/v3/{domain}/aggregates/countries` — Country Aggregates

**Response** (200):
```json
{
  "items": {
    "US": { "clicked": 100, "opened": 250 },
    "GB": { "clicked": 30, "opened": 70 }
  }
}
```

---

### POST `/v1/analytics/metrics` — Query Account Metrics

The primary current analytics endpoint. Uses JSON request body for complex queries.

**Request** (`application/json`):
```json
{
  "start": "Tue, 24 Sep 2024 00:00:00 +0000",
  "end": "Tue, 24 Oct 2024 00:00:00 +0000",
  "resolution": "day",
  "duration": "30d",
  "dimensions": ["time"],
  "metrics": ["accepted_count", "delivered_count", "opened_rate"],
  "filter": {
    "AND": [
      {
        "attribute": "domain",
        "comparator": "=",
        "values": [
          { "label": "example.com", "value": "example.com" }
        ]
      }
    ]
  },
  "include_subaccounts": false,
  "include_aggregates": true,
  "pagination": {
    "sort": "time:asc",
    "skip": 0,
    "limit": 10
  }
}
```

**Request fields:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `start` | string | no | 7 days ago | Start date (RFC 2822) |
| `end` | string | no | current time | End date (RFC 2822) |
| `resolution` | string | no | `"day"` | `"hour"`, `"day"`, `"month"` |
| `duration` | string | no | — | Period (e.g. `"1d"`, `"2h"`, `"2m"`). Overrides `start` |
| `dimensions` | string[] | no | — | Grouping attributes (max 3) |
| `metrics` | string[] | no | — | Metric names to return (max 10 count + rate) |
| `filter` | object | no | — | Filter predicates (see Filter Object above) |
| `include_subaccounts` | boolean | no | false | Include stats from all subaccounts |
| `include_aggregates` | boolean | no | false | Include top-level aggregate rollup |
| `pagination` | object | no | — | Pagination control |

**Pagination:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `sort` | string | — | `column:direction` (e.g. `"domain:asc"`, `"time:desc"`) |
| `skip` | integer | 0 | Offset |
| `limit` | integer | 10 (non-time), 1500 (time) | Max items. Time dim max: 1500. Others max: 1000 |

**Response** (200): See `RegularMetricsResponse` schema above.

```json
{
  "start": "Tue, 24 Sep 2024 00:00:00 +0000",
  "end": "Tue, 24 Oct 2024 00:00:00 +0000",
  "resolution": "day",
  "duration": "30d",
  "dimensions": ["time"],
  "aggregates": {
    "metrics": {
      "accepted_count": 400,
      "delivered_count": 380,
      "opened_rate": "0.47"
    }
  },
  "items": [
    {
      "dimensions": [
        { "dimension": "time", "value": "Tue, 24 Sep 2024 00:00:00 +0000", "display_value": "Tue, 24 Sep 2024 00:00:00 +0000" }
      ],
      "metrics": {
        "accepted_count": 50,
        "delivered_count": 48,
        "opened_rate": "0.42"
      }
    }
  ],
  "pagination": {
    "sort": "time:asc",
    "skip": 0,
    "limit": 10,
    "total": 30
  }
}
```

**Notes:**
- Rate limit: 500 requests per 10 seconds
- Rates are returned as string values (e.g. `"0.47"`)
- All count values are nullable unsigned integers
- When `include_aggregates` is true, `aggregates.metrics` contains a rollup across all items

---

### POST `/v1/analytics/usage/metrics` — Query Usage Metrics

Same request/response structure as `/v1/analytics/metrics` but for account usage tracking (email validation, preview, etc.). These are Mailgun Optimize/Inspect features that won't produce real data in the mock.

**Request** (`application/json`): Same shape as metrics endpoint, but with usage-specific `dimensions` (`subaccount`, `time`) and `metrics` (e.g. `email_validation_count`, `processed_count`).

**Response** (200): Same `RegularMetricsResponse` shape.

---

### POST `/v2/bounce-classification/metrics` — Bounce Classification

Returns bounce classification metrics broken down by entity, domain, IP, etc.

**Request** (`application/json`):
```json
{
  "start": "Tue, 24 Sep 2024 00:00:00 +0000",
  "end": "Tue, 24 Oct 2024 00:00:00 +0000",
  "resolution": "day",
  "dimensions": ["domain.name"],
  "metrics": ["critical_bounce_count", "non_critical_bounce_count"],
  "filter": {
    "AND": []
  },
  "include_subaccounts": false,
  "pagination": {
    "sort": "critical_bounce_count:desc",
    "skip": 0,
    "limit": 10
  }
}
```

**Response** (200): See `BounceClassificationMetricsResponse` schema above.

**Notes:**
- Items with `classified_failures_count == 0` are not returned
- Replaces the deprecated `GET /v1/bounce-classification/stats` and related endpoints

---

## SDK Usage Patterns

### Node.js SDK

```typescript
// Legacy v3 Stats
const stats = await mg.stats.getDomain('example.com', {
  event: ['delivered', 'accepted'],
  start: new Date('2024-01-01'),
  resolution: 'day',
});
// → GET /v3/example.com/stats/total?event=delivered&event=accepted&...

const accountStats = await mg.stats.getAccount({
  event: ['delivered'],
});
// → GET /v3/stats/total?event=delivered

// v1 Metrics
const metrics = await mg.metrics.getAccount({
  start: new Date('2024-09-24'),
  end: new Date('2024-10-24'),
  dimensions: ['time'],
  metrics: ['accepted_count', 'delivered_count'],
  include_aggregates: true,
});
// → POST /v1/analytics/metrics

const usage = await mg.metrics.getAccountUsage({
  dimensions: ['subaccount'],
  metrics: ['processed_count'],
});
// → POST /v1/analytics/usage/metrics
```

### Ruby SDK

```ruby
# v1 Metrics
client = Mailgun::Metrics.new(api_key)
result = client.account_metrics({
  start: 'Tue, 24 Sep 2024 00:00:00 +0000',
  end: 'Tue, 24 Oct 2024 00:00:00 +0000',
  resolution: 'day',
  dimensions: ['time'],
  metrics: ['accepted_count', 'delivered_count'],
})
# → POST /v1/analytics/metrics

# Legacy v3 Stats (via Tags class, deprecated)
tags = Mailgun::Tags.new(client)
stats = tags.get_tag_stats('example.com', 'newsletter', { event: 'delivered' })
# → GET /v3/example.com/tags/newsletter/stats?event=delivered
```

### Python SDK

```python
# v1 Metrics
result = client.analytics_metrics.create(data={
    'start': 'Tue, 24 Sep 2024 00:00:00 +0000',
    'end': 'Tue, 24 Oct 2024 00:00:00 +0000',
    'dimensions': ['time'],
    'metrics': ['accepted_count', 'delivered_count'],
})
# → POST /v1/analytics/metrics

# Usage Metrics
result = client.analytics_usage_metrics.create(data={...})
# → POST /v1/analytics/usage/metrics

# Bounce Classification
result = client.bounceclassification_metrics.create(data={...})
# → POST /v2/bounce-classification/metrics
```

---

## Mock Behavior Notes

### What the mock should fully support

1. **Account-level stats (`GET /v3/stats/total`)** — aggregate events across all domains, same response shape as domain stats. The mock computes stats from stored events.
2. **Event parameter handling** — support repeatable `event` parameter, `start`/`end` date parsing (RFC 2822 + Unix epoch), `resolution`, and `duration`.
3. **Stats derived from events** — the mock should compute stats on-demand from stored events rather than pre-aggregating. Query events for the given time range, bucket by resolution, and count by event type.

### What the mock should stub

1. **Account stats filter (`GET /v3/stats/filter`)** — accept parameters, but `filter` and `group` can be ignored. Return the same response as `/v3/stats/total`.
2. **Per-domain stats snapshot (`GET /v3/stats/total/domains`)** — return stats for each domain the mock knows about. Can derive from stored events.
3. **Domain aggregates (providers/devices/countries)** — return static/empty response objects. Tracking real provider/device/country data from mock messages is unnecessary.
4. **v1 Metrics API (`POST /v1/analytics/metrics`)** — accept JSON body, compute basic metrics from stored events. Support `time` and `domain` dimensions. Other dimensions (ip, tag, country, etc.) can return empty/minimal data.
5. **v1 Usage Metrics (`POST /v1/analytics/usage/metrics`)** — accept requests, return empty items with zeroed metrics. Usage features (email validation, preview) don't exist in the mock.
6. **v2 Bounce Classification (`POST /v2/bounce-classification/metrics`)** — accept requests, return empty items. Bounce classification details are production-only.

### What the mock can skip

- **Rate limiting** (500 req / 10 sec) — not needed for testing
- **Hourly data retention** (2-month real limit) — the mock keeps everything
- **Unique open/click deduplication** — the mock can treat `unique_opened_count` as equal to `opened_count` (or implement simple in-memory dedup if needed)
- **Send Time Optimization metrics** — `delivered_optimized_count` etc. will always be 0
- **Provider/country/device/bot detection** — return "Unknown" or "other" for all engagement dimensions
- **Deprecated v1 bounce classification GET endpoints** (`GET /v1/bounce-classification/stats`, etc.) — skip entirely

### Computing Stats from Events

The mock derives all stats from its stored events. The algorithm:

1. **Filter events** by time range (`start` to `end`) and scope (domain or account-wide)
2. **Bucket events** by resolution (`hour`/`day`/`month`)
3. **Count events** by type within each bucket
4. **Map event types** to the nested `StatsEntry` structure:
   - `accepted` event → increment `accepted.outgoing` (or `accepted.incoming` for inbound)
   - `delivered` event → increment `delivered.smtp`
   - `failed` event with `severity=permanent` → increment `failed.permanent.bounce`
   - `failed` event with `reason=suppress-bounce` → increment `failed.permanent.suppress-bounce`
   - `opened` event → increment `opened.total`
   - etc.

For the v1 Metrics API, the same event data is used but the response is restructured into the `RegularMetricsResponse` format with `dimensions` and flat `metrics` objects.

---

## `include_subaccounts` Query Parameter

The v1 Metrics endpoints and several list endpoints support `include_subaccounts` to aggregate data across all subaccounts. The mock should:

1. Accept the parameter on all v1 analytics endpoints
2. When `true`, include events from all subaccounts (or ignore if subaccount isolation is disabled)
3. When `false` (default), scope to the current account/subaccount context (per `X-Mailgun-On-Behalf-Of` header)

See [subaccounts.md](./subaccounts.md) for the header mechanism.

---

## Error Cases

| Scenario | Status | Response |
|----------|--------|----------|
| Missing required `event` param (v3 stats) | 400 | `{"message": "event is required"}` |
| Invalid event type | 400 | `{"message": "invalid event type"}` |
| Invalid resolution value | 400 | `{"message": "invalid resolution"}` |
| Invalid date format | 400 | `{"message": "invalid date format"}` |
| Too many dimensions (>3) | 400 | `{"message": "too many dimensions"}` |
| Too many metrics (>10) | 400 | `{"message": "too many metrics"}` |
| Unauthorized | 401 | `{"message": "Forbidden"}` |

---

## Test Scenarios

1. **Account stats (v3)** — send messages across two domains → GET `/v3/stats/total?event=delivered` → counts from both domains
2. **Account stats with multiple events** — GET `/v3/stats/total?event=delivered&event=accepted` → both event types in response
3. **Account stats with resolution** — GET stats with `resolution=hour` → hourly buckets
4. **Account stats with date range** — GET stats with explicit `start`/`end` → only events in range
5. **Account stats with duration** — GET stats with `duration=7d` → last 7 days from `end`
6. **Account stats missing event** — GET `/v3/stats/total` without `event` → 400 error
7. **Filtered stats (v3)** — GET `/v3/stats/filter?event=delivered&group=domain` → valid response
8. **Domain aggregates stub** — GET `/v3/{domain}/aggregates/providers` → valid response shape
9. **v1 metrics basic** — POST `/v1/analytics/metrics` with `dimensions=["time"]` → time-series items
10. **v1 metrics with filter** — POST with domain filter → scoped results
11. **v1 metrics with aggregates** — POST with `include_aggregates=true` → aggregates in response
12. **v1 metrics pagination** — POST with `pagination.limit=5` → paginated results with total
13. **v1 usage metrics stub** — POST `/v1/analytics/usage/metrics` → valid response with zero counts
14. **v2 bounce classification stub** — POST `/v2/bounce-classification/metrics` → valid empty response

---

## References

- **OpenAPI spec**: `mailgun.yaml` — stats endpoints at `/v3/stats/*` and `/v3/{domain}/stats/*`; metrics at `/v1/analytics/metrics` and `/v1/analytics/usage/metrics`; bounce classification at `/v2/bounce-classification/metrics`. Schemas: `StatsResponse`, `RegularMetricsResponse`, `UsageMetricsResponse`, `MetricsResponse` (bounce classification)
- **Stats docs**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/stats
- **Metrics docs**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/metrics
- **Node.js SDK**: https://github.com/mailgun/mailgun.js — `lib/Classes/Stats/StatsClient.ts` (v3), `lib/Classes/Metrics/MetricsClient.ts` (v1)
- **Ruby SDK**: https://github.com/mailgun/mailgun-ruby — `lib/mailgun/metrics/metrics.rb` (v1)
- **Python SDK**: https://github.com/mailgun/mailgun-python — `mailgun/handlers/metrics_handler.py` (v1)
- **Go SDK**: https://github.com/mailgun/mailgun-go — `stats.go` (v3), `analytics.go` and `mtypes/*.go` (v1)
- **Tag stats**: See [tags.md](./tags.md) for per-domain and per-tag stats (same `StatsResponse` schema)
- **Logs API**: See [events-and-logs.md](./events-and-logs.md) for `POST /v1/analytics/logs` (stubbed)
