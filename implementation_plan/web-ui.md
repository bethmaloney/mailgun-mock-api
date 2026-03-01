# Web UI

The mock Mailgun service includes a web-based control panel for inspecting captured messages, viewing events, managing resources, triggering test scenarios, and configuring mock behavior. The UI mirrors the real Mailgun Control Panel structure where practical, but adds mock-specific tools for testing and debugging.

## Design Philosophy

- **Inspection-first**: The primary purpose is inspecting what the mock captured — messages, events, webhook deliveries. CRUD management of domains/templates/etc. is secondary (the API handles that).
- **Mock-aware**: Surfaces mock-specific controls (trigger events, simulate inbound, configure behavior) alongside standard Mailgun-compatible views.
- **API-backed**: Every UI operation goes through the same REST API available to users for programmatic integration testing. The UI is a client of the API, not a separate system.
- **Real-time**: Uses WebSocket to push new messages and events to the UI without polling.

---

## Navigation Structure

The UI uses a sidebar layout modeled after the real Mailgun Control Panel, with mock-specific sections clearly marked.

```
┌─────────────────────────────────────────────────┐
│  Mailgun Mock                          [Config] │
├──────────────┬──────────────────────────────────┤
│              │                                  │
│  Dashboard   │   (main content area)            │
│              │                                  │
│  Messages    │                                  │
│  Events      │                                  │
│  Webhooks    │                                  │
│    Deliveries│                                  │
│              │                                  │
│  Domains     │                                  │
│  Templates   │                                  │
│  Mailing     │                                  │
│    Lists     │                                  │
│  Routes      │                                  │
│  Suppressions│                                  │
│              │                                  │
│  ── Testing ─│                                  │
│  Trigger     │                                  │
│    Events    │                                  │
│  Simulate    │                                  │
│    Inbound   │                                  │
│              │                                  │
│  ── Config ──│                                  │
│  Settings    │                                  │
│  API Keys    │                                  │
│              │                                  │
└──────────────┴──────────────────────────────────┘
```

---

## Pages

### 1. Dashboard

**Purpose**: At-a-glance overview of mock state.

**Content**:
- Total messages captured (all time / last hour)
- Event counts by type (accepted, delivered, failed, opened, clicked, etc.)
- Active domains count
- Recent webhook deliveries (last 5, with status)
- Quick links to Messages, Events, and Settings

**Data sources**: Aggregate queries over stored messages and events. No dedicated endpoint needed — the UI computes from existing list/count endpoints or a lightweight mock-specific summary endpoint.

**Mock-specific summary endpoint**:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/mock/dashboard` | Returns aggregate counts for dashboard display |

Response:
```json
{
  "messages": { "total": 142, "last_hour": 12 },
  "events": {
    "accepted": 142, "delivered": 130, "failed": 8,
    "opened": 45, "clicked": 12, "complained": 1,
    "unsubscribed": 3
  },
  "domains": { "total": 3, "active": 2, "unverified": 1 },
  "webhooks": {
    "configured": 4,
    "recent_deliveries": [
      { "url": "http://localhost:3000/hooks", "event": "delivered", "status_code": 200, "timestamp": 1709337600 }
    ]
  }
}
```

---

### 2. Messages

**Purpose**: Browse and inspect all captured messages.

**List view**:
- Table with columns: From, To, Subject, Domain, Tags, Timestamp, Status (accepted/delivered/failed)
- Domain selector dropdown to filter by sending domain
- Search box supporting: from, to, subject, message-id, tag
- Date range picker
- Pagination (cursor-based, matching Mailgun events API pattern)
- Real-time: new messages appear at top via WebSocket push

**Detail view** (click a message row):
- Tabbed display:
  - **HTML**: Rendered HTML body in an iframe sandbox. Toggle for mobile/desktop width preview.
  - **Text**: Plain text body
  - **Headers**: Table of all message headers (key-value pairs)
  - **Raw**: Full MIME source (syntax-highlighted)
  - **Attachments**: List of attachments with filename, size, content-type. Download link if mock stores attachment bytes.
  - **Events**: Timeline of all events for this message (accepted → delivered → opened → clicked, etc.)
  - **Metadata**: Message options (tracking, tags, variables, template reference, DKIM status)
- Action buttons:
  - **Trigger Event**: Quick-action to generate delivered/opened/clicked/failed events for this message
  - **Delete**: Remove message from mock storage

**API integration**:
- List: Uses `GET /v3/{domain}/events?event=accepted` or a mock-specific messages list endpoint
- Detail: Uses `GET /v3/domains/{domain}/messages/{storage_key}` for stored message content
- Events timeline: Uses `GET /v3/{domain}/events?message-id={id}` filtered to this message

**Mock-specific messages endpoint** (convenience — the events API can serve this but a dedicated endpoint is simpler for the UI):

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/mock/messages` | List all captured messages with filtering |
| `GET` | `/mock/messages/{message_id}` | Get full message detail |
| `DELETE` | `/mock/messages/{message_id}` | Delete a specific message |
| `DELETE` | `/mock/messages` | Clear all messages |

Query parameters for `GET /mock/messages`:
- `domain` — filter by sending domain
- `from` — filter by sender (substring match)
- `to` — filter by recipient (substring match)
- `subject` — filter by subject (substring match)
- `tag` — filter by tag
- `start` — start timestamp (epoch seconds)
- `end` — end timestamp (epoch seconds)
- `limit` — max results (default 50, max 300)
- `page` — cursor token for pagination

Response:
```json
{
  "items": [
    {
      "id": "<message-id>",
      "storage_key": "eyJhbGciOi...",
      "domain": "example.com",
      "from": "sender@example.com",
      "to": ["recipient@test.com"],
      "subject": "Test email",
      "tags": ["welcome"],
      "timestamp": 1709337600,
      "status": "delivered",
      "has_attachments": true
    }
  ],
  "paging": {
    "next": "/mock/messages?page=...",
    "previous": "/mock/messages?page=..."
  },
  "total_count": 142
}
```

---

### 3. Events

**Purpose**: View event log, matching the Mailgun Logs page.

**List view**:
- Table with columns: Event Type (with color badge), Recipient, Domain, Message Subject, Timestamp, Severity
- Event type filter (multi-select: accepted, delivered, failed, opened, clicked, unsubscribed, complained, stored, rejected)
- Domain filter dropdown
- Search by: recipient, message-id, tag, subject
- Date range picker
- Severity filter for failed events (temporary / permanent)
- Pagination (cursor-based)
- Real-time: new events appear via WebSocket push

**Event type badges** (color coding):
| Event | Color | Icon |
|-------|-------|------|
| accepted | blue | ✓ |
| delivered | green | ✓✓ |
| failed (temporary) | orange | ⚠ |
| failed (permanent) | red | ✗ |
| opened | purple | 👁 |
| clicked | teal | 🔗 |
| unsubscribed | gray | ⊘ |
| complained | red | ⚑ |
| stored | blue | 📦 |
| rejected | red | ⊘ |

**Detail view** (click an event row):
- Full event JSON (formatted, syntax-highlighted)
- Link to parent message (jump to Messages detail view)
- Delivery status details (for delivered/failed): status code, description, MX host
- Geolocation and client info (for opened/clicked): country, city, device, OS, browser
- Webhook delivery status: was a webhook fired for this event? Link to webhook delivery log entry.

**API integration**:
- Uses `GET /v3/{domain}/events` with query parameter filtering
- The mock also supports `GET /v3/events` (account-wide, without domain scope) as a convenience

---

### 4. Webhooks

**Purpose**: Manage webhook configuration and inspect delivery history.

#### 4a. Webhook Configuration

- List configured webhooks by domain (or account-level)
- Shows: URL, event types, domain scope
- Add/edit/delete webhooks
- Test button: sends a test payload to the webhook URL and shows the response

**API integration**: Uses standard Mailgun webhook CRUD endpoints:
- `GET /v3/domains/{domain}/webhooks`
- `POST /v3/domains/{domain}/webhooks`
- `PUT /v3/domains/{domain}/webhooks/{id}`
- `DELETE /v3/domains/{domain}/webhooks/{id}`

#### 4b. Webhook Delivery Log

**Purpose**: Inspect all webhook delivery attempts made by the mock.

- Table with columns: Timestamp, URL, Event Type, Domain, HTTP Status, Response Time, Attempt #
- Filter by: domain, event type, status (success/failure), URL
- Color-coded status: 2xx green, 4xx/5xx red, pending yellow

**Detail view** (click a delivery row):
- Request: method, URL, headers, payload (formatted JSON)
- Response: status code, headers, body (truncated)
- Retry history (if realistic retry mode enabled)

**API integration**: Uses mock-specific endpoint:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/mock/webhooks/deliveries` | List all webhook delivery attempts |

Query parameters:
- `domain` — filter by domain
- `event` — filter by event type
- `url` — filter by webhook URL (substring)
- `status` — filter: `success` (2xx), `failure` (non-2xx), `pending`
- `limit`, `page` — pagination

Response:
```json
{
  "items": [
    {
      "id": "del_abc123",
      "timestamp": 1709337600,
      "webhook_id": "wh_xyz",
      "url": "http://localhost:3000/hooks",
      "event_type": "delivered",
      "domain": "example.com",
      "message_id": "<msg-id>",
      "request": {
        "headers": { "Content-Type": "application/json" },
        "body": { "signature": {}, "event-data": {} }
      },
      "response": {
        "status_code": 200,
        "headers": {},
        "body": "OK"
      },
      "response_time_ms": 45,
      "attempt": 1,
      "success": true
    }
  ],
  "paging": { "next": "...", "previous": "..." },
  "total_count": 89
}
```

---

### 5. Domains

**Purpose**: View and manage domains configured in the mock.

- Domain list with status badges (active = green, unverified = orange)
- Domain detail: DNS records (SPF, DKIM, MX, tracking CNAME), tracking settings, SMTP credentials
- Add/delete domain
- Verify button (in manual verification mode)
- Link to domain's messages, events, and suppressions

**API integration**: Uses standard Mailgun domain endpoints (`GET/POST /v4/domains`, `GET/DELETE /v3/domains/{name}`, etc.)

---

### 6. Templates

**Purpose**: View and manage email templates.

- Template list by domain
- Template detail: name, description, engine, version list
- Version detail: rendered preview (with sample variables), raw template source
- No drag-and-drop editor needed — template creation is via API; the UI is for inspection

**API integration**: Uses standard Mailgun template endpoints (`GET/POST /v3/{domain}/templates`, etc.)

---

### 7. Mailing Lists

**Purpose**: View and manage mailing lists and members.

- List of mailing lists with address, member count, description
- Member list per mailing list: address, name, subscribed status, vars
- Add/remove members

**API integration**: Uses standard Mailgun mailing list endpoints (`GET /v3/lists/pages`, `GET /v3/lists/{address}/members/pages`, etc.)

---

### 8. Routes

**Purpose**: View and manage inbound routes.

- Route list with priority, expression, actions, description
- Add/edit/delete routes
- Expression tester: enter a sample email (from, to, subject) and see which routes would match
- Link to "Simulate Inbound" page for testing

**API integration**: Uses standard Mailgun route endpoints (`GET/POST /v3/routes`, etc.)

---

### 9. Suppressions

**Purpose**: Manage bounces, complaints, unsubscribes, and allowlist.

- Four tabs: Bounces, Complaints, Unsubscribes, Allowlist
- Domain selector dropdown
- Each tab shows a searchable table:
  - **Bounces**: address, error, code, created_at
  - **Complaints**: address, count, created_at
  - **Unsubscribes**: address, tags, created_at
  - **Allowlist**: value, type (domain/address), reason, createdAt
- Add/delete individual entries
- Bulk add via textarea (one address per line)
- Clear all button per suppression type

**API integration**: Uses standard Mailgun suppression endpoints (`GET/POST/DELETE /v3/{domain}/bounces`, etc.)

---

## Mock-Specific Testing Tools

These pages provide capabilities unique to the mock service, not present in the real Mailgun Control Panel.

### 10. Trigger Events

**Purpose**: Manually generate events for captured messages to simulate the email lifecycle.

**UI**:
- Message selector (search by domain, recipient, subject, message-id)
- Event type buttons: Deliver, Fail, Open, Click, Unsubscribe, Complain
- For "Fail": severity selector (temporary/permanent), custom error message
- For "Click": URL field (which link was clicked)
- For "Open": optional geolocation/device fields
- Batch trigger: select multiple messages, apply same event to all
- Result: shows generated event JSON, confirms webhook fired (if configured)

**API integration**: Uses mock-specific event trigger endpoints (defined in `events-and-logs.md`):

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/mock/events/{domain}/deliver/{message_id}` | Generate a `delivered` event |
| `POST` | `/mock/events/{domain}/fail/{message_id}` | Generate a `failed` event |
| `POST` | `/mock/events/{domain}/open/{message_id}` | Generate an `opened` event |
| `POST` | `/mock/events/{domain}/click/{message_id}` | Generate a `clicked` event |
| `POST` | `/mock/events/{domain}/unsubscribe/{message_id}` | Generate an `unsubscribed` event |
| `POST` | `/mock/events/{domain}/complain/{message_id}` | Generate a `complained` event |

Request body (all event triggers):
```json
{
  "recipient": "user@example.com",
  "severity": "permanent",
  "error_message": "550 User not found",
  "url": "https://example.com/link",
  "geolocation": { "country": "US", "city": "San Francisco" },
  "client_info": { "device_type": "desktop", "client_os": "macOS" }
}
```
All fields optional — the mock fills sensible defaults.

---

### 11. Simulate Inbound

**Purpose**: Simulate receiving an inbound email to test route evaluation and forwarding. Real Mailgun receives email via MX records, which the mock can't replicate — this provides the equivalent.

**UI**:
- Form fields: From, To, Subject, Body (text), Body (HTML), Headers (key-value editor), Attachments (file upload)
- "Send Inbound" button
- Result panel: shows which routes matched, what actions were triggered (forward URL, stored message, stop), and the generated `stored` event

**API integration**: Uses mock-specific endpoint (defined in `routes.md`):

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/mock/inbound/{domain}` | Simulate an inbound email arriving at the domain |

Request body (`multipart/form-data`):
```
from: sender@external.com
to: recipient@example.com
subject: Inbound test
body-plain: Hello from outside
body-html: <p>Hello from outside</p>
Content-Type: ... (for attachments)
```

Response:
```json
{
  "matched_routes": [
    {
      "id": "route-abc",
      "expression": "match_recipient('recipient@example.com')",
      "actions": ["forward('http://localhost:3000/inbound')", "store()"]
    }
  ],
  "actions_taken": [
    { "action": "forward", "url": "http://localhost:3000/inbound", "status_code": 200 },
    { "action": "store", "storage_key": "eyJhbGciOi..." }
  ],
  "event_id": "evt_xyz"
}
```

---

### 12. Settings (Mock Configuration)

**Purpose**: Configure mock behavior. Settings are persisted in memory (reset on restart) or optionally via config file / environment variables.

**UI**: Organized into sections with toggles and inputs.

#### Event Generation

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `auto_deliver` | boolean | `true` | Automatically generate `delivered` events after `accepted` |
| `delivery_delay_ms` | number | `0` | Delay (ms) before generating delivered event |
| `default_delivery_status_code` | number | `250` | SMTP status code for delivered events |
| `auto_fail_rate` | number | `0.0` | Fraction of messages that auto-generate `failed` instead of `delivered` (0.0–1.0) |

#### Domain Behavior

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `domain_auto_verify` | boolean | `true` | New domains created as `active` with valid DNS |
| `sandbox_domain` | string | `sandbox*.mailgun.org` | Pre-seeded sandbox domain name |

#### Webhook Delivery

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `webhook_retry_mode` | enum | `immediate` | `immediate`: single attempt, no retries. `realistic`: full retry schedule per Mailgun spec |
| `webhook_timeout_ms` | number | `5000` | Timeout for webhook HTTP requests |

#### Authentication

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `auth_mode` | enum | `accept_any` | `accept_any`: accept any non-empty API key. `validate`: check key exists and is enabled |
| `signing_key` | string | `key-mock-signing-...` | Deterministic webhook signing key (retrievable via API) |

#### Storage

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `store_attachment_bytes` | boolean | `false` | Whether to store actual attachment content (true) or just metadata (false) |
| `max_messages` | number | `0` (unlimited) | Maximum messages to retain. Oldest evicted first when exceeded |
| `max_events` | number | `0` (unlimited) | Maximum events to retain |

#### Global Actions

- **Reset All Data**: Clear all messages, events, domains, suppressions, etc. Returns mock to fresh state.
- **Reset Messages & Events Only**: Keep domain/webhook/template configuration, clear captured data.

**API integration**:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/mock/config` | Get current mock configuration |
| `PUT` | `/mock/config` | Update mock configuration (partial update) |
| `POST` | `/mock/reset` | Reset all mock data |
| `POST` | `/mock/reset/messages` | Reset messages and events only |

`GET /mock/config` response:
```json
{
  "event_generation": {
    "auto_deliver": true,
    "delivery_delay_ms": 0,
    "default_delivery_status_code": 250,
    "auto_fail_rate": 0.0
  },
  "domain_behavior": {
    "domain_auto_verify": true,
    "sandbox_domain": "sandbox123.mailgun.org"
  },
  "webhook_delivery": {
    "webhook_retry_mode": "immediate",
    "webhook_timeout_ms": 5000
  },
  "authentication": {
    "auth_mode": "accept_any",
    "signing_key": "key-mock-signing-key-000000000000"
  },
  "storage": {
    "store_attachment_bytes": false,
    "max_messages": 0,
    "max_events": 0
  }
}
```

`PUT /mock/config` accepts partial updates — only include fields to change:
```json
{
  "event_generation": { "auto_deliver": false },
  "webhook_delivery": { "webhook_retry_mode": "realistic" }
}
```

---

### 13. API Keys

**Purpose**: View and manage API keys and SMTP credentials (when `auth_mode` is `validate`).

- List API keys with role, created date, status (active/disabled)
- Create/delete keys
- SMTP credentials per domain
- In `accept_any` mode, shows a notice that authentication is disabled

**API integration**: Uses standard Mailgun key/credential endpoints (documented in `credentials-and-keys.md`)

---

## Real-Time Updates

The mock UI uses WebSocket for live updates. A single WebSocket connection per browser tab receives all push events.

### WebSocket Endpoint

| Path | Description |
|------|-------------|
| `/mock/ws` | WebSocket endpoint for real-time UI updates |

### Message Types

```json
{ "type": "message.new", "data": { "id": "...", "domain": "...", "from": "...", "to": [...], "subject": "...", "timestamp": 1709337600 } }
{ "type": "event.new", "data": { "id": "...", "event": "delivered", "domain": "...", "recipient": "...", "message_id": "...", "timestamp": 1709337600 } }
{ "type": "webhook.delivery", "data": { "id": "...", "url": "...", "event": "...", "status_code": 200, "timestamp": 1709337600 } }
{ "type": "config.updated", "data": { "field": "auto_deliver", "value": false } }
{ "type": "data.reset", "data": { "scope": "all" } }
```

### Connection Behavior

- Auto-reconnect on disconnect (exponential backoff: 1s, 2s, 4s, 8s, max 30s)
- Optional: browser notification on new message (user opt-in via UI toggle)
- Connection status indicator in UI header (connected = green dot, disconnected = red dot)

---

## All Mock-Specific Endpoints (Summary)

These are non-standard Mailgun endpoints unique to the mock service. All are prefixed with `/mock/`.

| Method | Path | Source Doc | Category |
|--------|------|-----------|----------|
| `GET` | `/mock/dashboard` | web-ui.md | Dashboard summary |
| `GET` | `/mock/messages` | web-ui.md | Message list |
| `GET` | `/mock/messages/{message_id}` | web-ui.md | Message detail |
| `DELETE` | `/mock/messages/{message_id}` | web-ui.md | Delete message |
| `DELETE` | `/mock/messages` | web-ui.md | Clear all messages |
| `POST` | `/mock/events/{domain}/deliver/{message_id}` | events-and-logs.md | Trigger delivered event |
| `POST` | `/mock/events/{domain}/fail/{message_id}` | events-and-logs.md | Trigger failed event |
| `POST` | `/mock/events/{domain}/open/{message_id}` | events-and-logs.md | Trigger opened event |
| `POST` | `/mock/events/{domain}/click/{message_id}` | events-and-logs.md | Trigger clicked event |
| `POST` | `/mock/events/{domain}/unsubscribe/{message_id}` | events-and-logs.md | Trigger unsubscribed event |
| `POST` | `/mock/events/{domain}/complain/{message_id}` | events-and-logs.md | Trigger complained event |
| `GET` | `/mock/webhooks/deliveries` | webhooks.md | Webhook delivery log |
| `POST` | `/mock/webhooks/trigger` | webhooks.md | Manual webhook trigger |
| `POST` | `/mock/inbound/{domain}` | routes.md | Simulate inbound email |
| `GET` | `/mock/config` | web-ui.md | Get mock configuration |
| `PUT` | `/mock/config` | web-ui.md | Update mock configuration |
| `POST` | `/mock/reset` | web-ui.md | Reset all data |
| `POST` | `/mock/reset/messages` | web-ui.md | Reset messages & events |
| `WS` | `/mock/ws` | web-ui.md | WebSocket for real-time updates |

---

## Technical Notes

### Architecture

- The UI is a single-page application (SPA) served by the same HTTP server as the API.
- The mock serves UI static assets at the root path (`/`) and API endpoints at their standard Mailgun paths (`/v3/...`, `/v4/...`, `/v1/...`) plus mock-specific paths (`/mock/...`).
- No separate build step for development — the UI is bundled at build time and served as static files.

### Mailgun CP Parity vs. Mock Focus

The real Mailgun Control Panel has sections the mock intentionally omits:
- **Optimize** (inbox placement, email preview) — production-only
- **Validations** (email address validation) — stub at API level only
- **Billing / Account Settings** — not applicable
- **Security (2FA, SSO)** — not applicable

The mock adds sections the real CP doesn't have:
- **Trigger Events** — manually advance message lifecycle
- **Simulate Inbound** — test route evaluation without SMTP/MX
- **Settings** — mock behavior configuration
- **Webhook Delivery Log** — inspect what the mock sent to webhook URLs (real Mailgun has no equivalent)

### UI Technology

No specific framework is prescribed. Recommendations:
- Lightweight SPA framework (e.g., Vue, Svelte, or React) for interactive components
- Server-side rendering is not needed — the UI is for local dev/testing
- Dark mode support via CSS custom properties / media query
- Mobile-responsive is low priority (this is a developer tool used on desktop)

### Authentication

The Web UI does not require authentication by default. It is a local development tool. An optional `ui_password` setting can be added later if needed for shared environments.

---

## References

### Mailgun Control Panel
- [The Mailgun Control Panel](https://help.mailgun.com/hc/en-us/articles/360021388013-The-Mailgun-Control-Panel)
- [Reporting Dashboard](https://help.mailgun.com/hc/en-us/articles/4402703701019-Reporting-Dashboard)
- [Suppressions Management](https://help.mailgun.com/hc/en-us/articles/360012287493-Suppressions-Bounces-Complaints-Unsubscribes-Allowlists)
- [Events & Logs: Message Event Types](https://help.mailgun.com/hc/en-us/articles/203661564-Events-Logs-Message-Event-Types)
- [Events & Logs: Search Queries](https://help.mailgun.com/hc/en-us/articles/203879300-Events-Logs-Search-Queries)

### Mock Email Service UI Inspiration
- [MailHog](https://github.com/mailhog/MailHog) — SSE real-time updates, Jim chaos monkey
- [Mailpit](https://github.com/axllent/mailpit) — advanced search query language, WebSocket updates, tagging, HTML compatibility checking
- [Mailtrap](https://mailtrap.io/) — bounce emulator, spam analysis, HTML compatibility
- [Papercut SMTP](https://github.com/ChangemakerStudios/Papercut-SMTP) — forwarding rules, dark mode

### Internal Plan References
- `events-and-logs.md` — mock event trigger endpoints
- `webhooks.md` — webhook delivery log and manual trigger endpoints
- `routes.md` — inbound message simulation endpoint
- `credentials-and-keys.md` — authentication modes
- `domains.md` — domain verification modes
