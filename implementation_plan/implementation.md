# Implementation Plan

Tracks implementation progress across all feature areas. Each phase builds on the previous — complete phases roughly in order, though tasks within a phase can be parallelized. Refer to the individual plan docs (linked) for endpoint specs, field schemas, and behavioral details.

## Progress

| Phase | Area | Status | Notes |
|-------|------|--------|-------|
| 0 | Foundation & Cross-cutting | in progress | Auth, pagination, config, response formatting |
| 1 | Domains | done | CRUD, verification, tracking, connection settings |
| 2 | Credentials & Keys | done | SMTP creds, API keys, IP allowlist |
| 3 | Messages & Storage | done | Send, store, retrieve, resend |
| 4 | Events & Logs | in progress | Generation pipeline, querying, mock triggers |
| 5 | Suppressions | in progress | Bounces, complaints, unsubscribes, allowlist |
| 6 | Templates | in progress | CRUD, versioning, Handlebars rendering |
| 7 | Tags & Stats | in progress | Auto-creation, time-series stats, v1 stubs |
| 8 | Mailing Lists | in progress | List/member CRUD, bulk ops, send integration |
| 9 | Webhooks | pending | v3/v4/v1 APIs, delivery pipeline, signing |
| 10 | Routes | pending | CRUD, expression parser, inbound simulation |
| 11 | IPs & IP Pools | pending | Stub — static IPs, pool CRUD |
| 12 | Subaccounts | pending | CRUD, limits, feature flags, isolation |
| 13 | Metrics & Analytics | pending | v3 stats, v1 metrics, usage, bounce classification |
| 14 | Web UI — Foundation | pending | Shell, routing, dashboard |
| 15 | Web UI — Messages & Events | pending | Message/event list and detail views |
| 16 | Web UI — Management | pending | Domains, suppressions, templates, lists, webhooks, routes |
| 17 | Web UI — Testing & Real-time | pending | Event triggers, inbound sim, config, WebSocket |

---

## Phase 0: Foundation & Cross-cutting Infrastructure

Shared infrastructure referenced by every feature area. Must be complete before domain-specific work begins.

### Database & Models
- [x] Define GORM base model with common fields (ID, timestamps) and consistent primary key strategy
- [x] Set up migration framework — auto-migrate all models on startup
- [ ] Add seed data support (e.g., default IPs for IP Pools, see [ips-and-pools.md](./ips-and-pools.md))

### Authentication Middleware
- [x] Implement HTTP Basic Auth middleware (`username: "api"`, `password: <key>`) — see [credentials-and-keys.md](./credentials-and-keys.md)
- [x] Support configurable auth mode: `none` (accept anything), `single` (one master key), `full` (per-key RBAC) — see [web-ui.md](./web-ui.md) mock config
- [x] Extract domain from URL path and validate it exists (shared across all `/{domain}/` routes)

### Shared Pagination
- [x] Cursor/URL-based pagination (events, suppressions, templates, tags, mailing lists) — opaque `next`/`previous` URLs per [events-and-logs.md](./events-and-logs.md)
- [x] Skip/limit offset pagination (mailing list members, routes) — `skip` + `limit` params
- [x] Token-based pagination (v1 analytics endpoints) — `cursor` in response body

### Global Mock Configuration
- [x] Define config struct with all mock settings: auto-delivery mode, domain verification mode, webhook retry mode, auth mode, storage limits — consolidated from [web-ui.md](./web-ui.md)
- [x] `GET /mock/config` and `PUT /mock/config` endpoints
- [x] Pass config reference to all subsystems so changes take effect immediately

### Response Formatting
- [x] Standard Mailgun success envelope: `{ "message": "...", "id": "..." }` or `{ "items": [...], "paging": {...} }`
- [x] Standard error responses: `{ "message": "..." }` with appropriate HTTP status codes
- [x] Support both `application/json` and `multipart/form-data` request parsing (many endpoints accept both)
- [x] Handle `application/x-www-form-urlencoded` for credential/key endpoints

### Subaccount Scoping (cross-cutting)
- [ ] Middleware to extract `X-Mailgun-On-Behalf-Of` header and set subaccount context on request — see [subaccounts.md](./subaccounts.md)
- [ ] All resource models include optional `subaccount_id` field for soft isolation
- [ ] List endpoints filter by subaccount context; `include_subaccounts` param bypasses filter

### Mock Utility Endpoints
- [x] `POST /mock/reset` — clear all data
- [x] `POST /mock/reset/{domain}` — clear data for a single domain
- [x] `POST /mock/reset/messages` — clear only messages and events
- [x] `GET /mock/health` — health check (already exists)

---

## Phase 1: Domains

Foundation for all other resources. Nothing else works without domains.

> Plan doc: [domains.md](./domains.md)

### Domain CRUD
- [x] Model: `Domain` with all fields (name, state, type, spam_action, wildcard, web_scheme, etc.)
- [x] `POST /v4/domains` — create domain with auto-generated DNS records (SPF, DKIM, MX, CNAME)
- [x] `GET /v4/domains` — list with pagination, `state` and `authority` filters
- [x] `GET /v4/domains/{name}` — get single domain with DNS records
- [x] `PUT /v4/domains/{name}` — update mutable fields (spam_action, wildcard, web_scheme, web_prefix)
- [x] `DELETE /v3/domains/{name}` — soft delete

### Domain Verification
- [x] `PUT /v4/domains/{name}/verify` — verify DNS records
- [x] Mock verification modes: `auto` (always pass), `manual` (require explicit verify call) — controlled via mock config
- [x] Generate realistic DNS record values on domain creation

### Tracking Settings
- [x] `GET /v3/domains/{name}/tracking` — get all tracking settings
- [x] `PUT /v3/domains/{name}/tracking/open` — update open tracking (active bool)
- [x] `PUT /v3/domains/{name}/tracking/click` — update click tracking (active yes/no/htmlonly)
- [x] `PUT /v3/domains/{name}/tracking/unsubscribe` — update unsubscribe tracking (active bool, custom_html_footer, custom_text_footer)

### Connection Settings
- [x] `GET /v3/domains/{name}/connection` — get connection settings
- [x] `PUT /v3/domains/{name}/connection` — update (require_tls, skip_verification)

### DKIM Management
- [x] `PUT /v3/domains/{name}/dkim_authority` — enable/disable
- [x] `PUT /v3/domains/{name}/dkim_selector` — update selector

---

## Phase 2: Credentials & Keys

Auth infrastructure for the rest of the API. Implements alongside or immediately after Domains.

> Plan doc: [credentials-and-keys.md](./credentials-and-keys.md)

### SMTP Credentials
- [x] Model: `SMTPCredential` (domain-scoped, login as `user@domain`)
- [x] `GET /v3/domains/{domain}/credentials` — list with pagination
- [x] `POST /v3/domains/{domain}/credentials` — create (password 5–32 chars)
- [x] `PUT /v3/domains/{domain}/credentials/{login}` — update password
- [x] `DELETE /v3/domains/{domain}/credentials/{login}` — delete

### API Keys
- [x] Model: `APIKey` with role, domain scope, active status, description
- [x] `GET /v1/keys` — list all keys
- [x] `POST /v1/keys` — create key (`key-` prefix + 48 hex chars, secret shown once)
- [x] `DELETE /v1/keys/{id}` — delete/deactivate
- [x] `POST /v1/keys/{id}/regenerate` — regenerate secret
- [x] `GET /v1/keys/public` — get public verification key

### IP Allowlist (stub)
- [x] `GET /v2/ip_whitelist` — list
- [x] `POST /v2/ip_whitelist` — add
- [x] `PUT /v2/ip_whitelist` — update
- [x] `DELETE /v2/ip_whitelist` — delete

---

## Phase 3: Messages & Storage

Core message acceptance pipeline. Generates events that feed into all downstream systems.

> Plan doc: [messages.md](./messages.md)

### Message Sending
- [x] Model: `StoredMessage` with all fields (from, to, cc, bcc, subject, body-plain, body-html, attachments, headers, tags, variables)
- [x] `POST /v3/{domain}/messages` — accept multipart/form-data, validate required fields (from, to, subject or template), store message
- [x] Parse recipient variables (`recipient-variables` JSON) for batch sending
- [x] Handle `o:tag` (up to 10 tags per message), `o:tracking-*` overrides, `o:deliverytime`, `o:testmode`
- [x] Handle custom headers (`h:X-*`), custom variables (`v:*`)
- [x] Return `{ "id": "<message-id>", "message": "Queued. Thank you." }`

### MIME Sending
- [x] `POST /v3/{domain}/messages.mime` — accept raw MIME with `to` override

### Message Storage & Retrieval
- [x] Generate storage keys (opaque format, e.g., `mock-<uuid>`)
- [x] `GET /v3/domains/{domain}/messages/{storage_key}` — retrieve stored message with full headers
- [x] `DELETE /v3/domains/{domain}/messages/{storage_key}` — delete stored message
- [x] Store attachment bytes; serve via storage URL in message detail

### Message Resend
- [x] `POST /v3/domains/{domain}/messages/{storage_key}` — resend to new recipients

### Sending Queues (stub)
- [x] `GET /v3/domains/{name}/sending_queues` — return queue status (always empty/idle)
- [x] `DELETE /v3/{domain}/envelopes` — purge queue (no-op)

---

## Phase 4: Events & Logs

Event generation pipeline — produces events for every message lifecycle transition.

> Plan doc: [events-and-logs.md](./events-and-logs.md)

### Event Generation
- [x] Model: `Event` with type, timestamp (microsecond precision), message headers, tags, recipient, delivery-status, etc.
- [x] Generate `accepted` event per recipient on message send
- [x] Auto-generate delivery events based on mock config mode:
  - `immediate`: generate `delivered` (or `failed`) synchronously on send
  - `delayed`: generate after configurable delay
  - `manual`: only via mock trigger endpoints
- [ ] Check suppression lists before generating `delivered` — produce `failed` with reason if suppressed
- [x] Generate unique event IDs and realistic `log-level`, `delivery-status` fields

### Event Querying
- [x] `GET /v3/{domain}/events` — list with opaque URL-based pagination
- [x] Filter support: `event` type, `recipient`, `from`, `subject`, `tags`, `severity`, `message-id`
- [x] Time range: `begin`, `end` (RFC 2822, Unix epoch, or shorthand)
- [x] AND/OR/NOT filter expressions on `event` param
- [x] Ascending/descending order
- [x] Generate opaque `next`/`previous` page URLs

### Mock Event Triggers
- [x] `POST /mock/events/{domain}/deliver/{message_id}` — trigger delivered event
- [x] `POST /mock/events/{domain}/fail/{message_id}` — trigger failed event
- [x] `POST /mock/events/{domain}/open/{message_id}` — trigger opened event
- [x] `POST /mock/events/{domain}/click/{message_id}` — trigger clicked event
- [x] `POST /mock/events/{domain}/unsubscribe/{message_id}` — trigger unsubscribed event
- [x] `POST /mock/events/{domain}/complain/{message_id}` — trigger complained event

---

## Phase 5: Suppressions

Per-domain suppression lists that integrate with the message pipeline.

> Plan doc: [suppressions.md](./suppressions.md)

### Bounces
- [x] Model: `Bounce` (address, code, error, created_at — snake_case)
- [x] `GET /v3/{domain}/bounces` — list with pagination
- [x] `GET /v3/{domain}/bounces/{address}` — get single
- [x] `POST /v3/{domain}/bounces` — add single or batch (JSON array)
- [x] `DELETE /v3/{domain}/bounces/{address}` — delete single
- [x] `DELETE /v3/{domain}/bounces` — clear all
- [x] `POST /v3/{domain}/bounces/import` — CSV import (return 202 + async task)

### Complaints
- [x] Model: `Complaint` (address, count, created_at)
- [x] Same 6 endpoints as bounces at `/v3/{domain}/complaints/*`

### Unsubscribes
- [x] Model: `Unsubscribe` (address, tags, created_at)
- [x] Same 6 endpoints as bounces at `/v3/{domain}/unsubscribes/*`
- [x] Handle `tag`/`tags` field inconsistency (singular string vs plural array)

### Allowlist
- [x] Model: `AllowlistEntry` (value, type: "address"|"domain", createdAt — camelCase)
- [x] Same 6 endpoints at `/v3/{domain}/whitelists/*`
- [ ] Allowlist check: prevents automatic bounce recording but doesn't override complaints/unsubscribes

### Suppression Integration
- [ ] Hook into message send pipeline: check all suppression lists per recipient before generating delivery events
- [ ] Auto-create suppression entries on relevant events (bounce → bounces list, complaint → complaints list)

---

## Phase 6: Templates

Server-side templates with versioning and rendering.

> Plan doc: [templates.md](./templates.md)

### Template CRUD
- [x] Model: `Template` (name lowercased, description, createdAt — camelCase)
- [x] `GET /v3/{domain}/templates` — list with pagination (paginate by name)
- [x] `GET /v3/{domain}/templates/{name}` — get single (optionally include active version)
- [x] `POST /v3/{domain}/templates` — create (with optional initial version)
- [x] `PUT /v3/{domain}/templates/{name}` — update description
- [x] `DELETE /v3/{domain}/templates/{name}` — delete with all versions

### Version Management
- [x] Model: `TemplateVersion` (tag lowercased, template body, engine, active flag, mjml field, comment)
- [x] `GET /v3/{domain}/templates/{name}/versions` — list versions (paginate by tag)
- [x] `GET /v3/{domain}/templates/{name}/versions/{tag}` — get single version
- [x] `POST /v3/{domain}/templates/{name}/versions` — create version (max 40 per template)
- [x] `PUT /v3/{domain}/templates/{name}/versions/{tag}` — update (setting `active: yes` deactivates others)
- [x] `DELETE /v3/{domain}/templates/{name}/versions/{tag}` — delete version
- [x] `PUT /v3/{domain}/templates/{name}/versions/{tag}/copy/{new_tag}` — copy version

### Template Rendering
- [ ] Integrate Handlebars rendering (Go library) with custom `equal` helper
- [ ] Resolve `t:template` / `template` param at message send time
- [ ] Substitute `recipient-variables` and `v:*` custom variables into template
- [ ] Inject `t:headers` as additional message headers

---

## Phase 7: Tags & Stats

Auto-created labels on messages with time-series statistics.

> Plan doc: [tags.md](./tags.md)

### Tag CRUD
- [x] Model: `Tag` (name, description, first-seen, last-seen — hyphenated for v3)
- [x] Auto-create tags on message send (from `o:tag` param, up to 10 per message)
- [x] `GET /v3/{domain}/tags` — list with pagination and prefix filter
- [x] `GET /v3/{domain}/tags/{tag}` — get single
- [x] `PUT /v3/{domain}/tags/{tag}` — update description
- [x] `DELETE /v3/{domain}/tags/{tag}` — delete (detaches from events, doesn't delete events)

### Tag Statistics
- [x] `GET /v3/{domain}/tags/{tag}/stats` — time-series stats bucketed by `resolution` (hour/day/month)
- [x] `GET /v3/{domain}/tags/{tag}/stats/aggregates/countries` — aggregate by country
- [x] `GET /v3/{domain}/tags/{tag}/stats/aggregates/providers` — aggregate by provider
- [x] `GET /v3/{domain}/tags/{tag}/stats/aggregates/devices` — aggregate by device
- [x] `GET /v3/domains/{domain}/limits/tag` — tag count and limit

### Domain-level Stats
- [x] `GET /v3/{domain}/stats/total` — domain-level time-series stats (shared computation with tag stats)
- [x] Support `event` param as repeatable query parameter

### Path Discrepancy Handling
- [ ] Support both `/v3/{domain}/tags/{tag}` (SDK) and `/v3/{domain}/tag?tag=...` (OpenAPI) paths

### v1 Analytics Tags API (stub)
- [ ] `POST /v1/analytics/tags` — list tags (POST with JSON body)
- [ ] `PUT /v1/analytics/tags/{tag}` — update
- [ ] `DELETE /v1/analytics/tags/{tag}` — delete
- [ ] `GET /v1/analytics/tags/limits` — limits

---

## Phase 8: Mailing Lists

List and member CRUD with bulk operations.

> Plan doc: [mailing-lists.md](./mailing-lists.md)

### List CRUD
- [x] Model: `MailingList` (address, name, description, access_level, reply_preference, members_count, created_at)
- [x] `GET /v3/lists/pages` — list with cursor-based pagination
- [x] `GET /v3/lists/{address}` — get single
- [x] `POST /v3/lists` — create
- [x] `PUT /v3/lists/{address}` — update
- [x] `DELETE /v3/lists/{address}` — delete

### Member CRUD
- [x] Model: `MailingListMember` (address, name, vars JSON, subscribed, list_address)
- [x] `GET /v3/lists/{address}/members/pages` — list with cursor pagination
- [x] `GET /v3/lists/{address}/members` — list with offset pagination (skip/limit)
- [x] `GET /v3/lists/{address}/members/{member_address}` — get single
- [x] `POST /v3/lists/{address}/members` — add single member
- [x] `PUT /v3/lists/{address}/members/{member_address}` — update
- [x] `DELETE /v3/lists/{address}/members/{member_address}` — delete

### Bulk Operations
- [x] `POST /v3/lists/{address}/members.json` — bulk add/upsert via JSON array
- [ ] `POST /v3/lists/{address}/members/import` — CSV import
- [x] Track `members_count` on list model (increment/decrement on member changes)

### List Sending Integration
- [ ] Expand list address to members on message send
- [ ] Substitute `%recipient.varname%` from member `vars`
- [ ] Generate `%mailing_list_unsubscribe_url%` pointing back to mock

---

## Phase 9: Webhooks

Register URLs and deliver event payloads with signatures.

> Plan doc: [webhooks.md](./webhooks.md)

### v3 Domain Webhooks (per event type)
- [ ] Model: `Webhook` (domain, event_type, urls[], active)
- [ ] `GET /v3/domains/{domain}/webhooks` — list all
- [ ] `GET /v3/domains/{domain}/webhooks/{webhook_name}` — get single
- [ ] `POST /v3/domains/{domain}/webhooks` — create
- [ ] `PUT /v3/domains/{domain}/webhooks/{webhook_name}` — update
- [ ] `DELETE /v3/domains/{domain}/webhooks/{webhook_name}` — delete

### v4 Domain Webhooks (URL-centric)
- [ ] `POST /v4/domains/{domain}/webhooks` — create
- [ ] `PUT /v4/domains/{domain}/webhooks/{webhook_id}` — update
- [ ] `DELETE /v4/domains/{domain}/webhooks/{webhook_id}` — delete

### v1 Account Webhooks
- [ ] `GET /v1/webhooks` — list
- [ ] `POST /v1/webhooks` — create
- [ ] `PUT /v1/webhooks/{webhook_id}` — update
- [ ] `DELETE /v1/webhooks/{webhook_id}` — delete

### Webhook Delivery Pipeline
- [ ] Build event payload in Webhooks 2.0 format (`signature` + `event-data`)
- [ ] Sign payloads with HMAC-SHA256 using webhook signing key
- [ ] Deliver via HTTP POST to registered URLs
- [ ] Retry logic: configurable `immediate` (1 retry) or `realistic` (8 attempts with backoff) — see [web-ui.md](./web-ui.md) config
- [ ] Deduplicate across domain-level and account-level webhooks

### Webhook Signing Key
- [ ] `GET /v5/accounts/http_signing_key` — get current signing key
- [ ] `POST /v5/accounts/http_signing_key` — rotate signing key

### Mock Webhook Inspection
- [ ] `GET /mock/webhooks/deliveries` — list delivery attempts with status
- [ ] `POST /mock/webhooks/trigger` — manually trigger a webhook delivery

---

## Phase 10: Routes

Account-level inbound email routing.

> Plan doc: [routes.md](./routes.md)

### Route CRUD
- [ ] Model: `Route` (id as 24-char hex, priority, description, expression, actions[], created_at)
- [ ] `GET /v3/routes` — list with skip/limit pagination
- [ ] `GET /v3/routes/{id}` — get single
- [ ] `POST /v3/routes` — create with expression + actions
- [ ] `PUT /v3/routes/{id}` — update
- [ ] `DELETE /v3/routes/{id}` — delete

### Expression Parser
- [ ] Parse `match_recipient("pattern")`, `match_header("header", "pattern")`, `catch_all()`
- [ ] Support `and` operator for combining expressions

### Action Handling
- [ ] `forward("url")` — HTTP POST to URL with event payload
- [ ] `forward("email")` — store as forwarded (no actual send)
- [ ] `store(notify="url")` — store message + optional notification
- [ ] `stop()` — halt further route evaluation

### Inbound Simulation
- [ ] `POST /mock/inbound/{domain}` — simulate receiving an inbound email, evaluate routes, trigger actions

---

## Phase 11: IPs & IP Pools (stub)

Static IP data and pool CRUD. Low complexity.

> Plan doc: [ips-and-pools.md](./ips-and-pools.md)

### IP Management
- [ ] Model: `IP` with pre-seeded default IPs
- [ ] `GET /v3/ips` — list all IPs
- [ ] `GET /v3/ips/{ip}` — get single IP
- [ ] `GET /v3/domains/{name}/ips` — list IPs assigned to domain
- [ ] `POST /v3/domains/{name}/ips` — assign IP to domain
- [ ] `DELETE /v3/domains/{name}/ips/{ip}` — unassign IP (also handle `/v3/domains/{name}/pool/{ip}` path)

### IP Pool CRUD
- [ ] Model: `IPPool` (name, description, ips[])
- [ ] `GET /v1/ip_pools` (also accept `/v3/ip_pools`) — list pools
- [ ] `POST /v1/ip_pools` — create pool
- [ ] `GET /v1/ip_pools/{pool_id}` — get pool
- [ ] `PUT /v1/ip_pools/{pool_id}` — update pool
- [ ] `DELETE /v1/ip_pools/{pool_id}` — delete pool (with `ip_pool` replacement param)

---

## Phase 12: Subaccounts

Multi-tenancy with header-based scoping.

> Plan doc: [subaccounts.md](./subaccounts.md)

### Subaccount CRUD
- [ ] Model: `Subaccount` (id as 24-char hex, name, status: open/disabled/closed)
- [ ] `GET /v5/accounts/subaccounts` — list with pagination and filters
- [ ] `POST /v5/accounts/subaccounts` — create (accept name in both query param and form data)
- [ ] `GET /v5/accounts/subaccounts/{id}` — get single
- [ ] `POST /v5/accounts/subaccounts/{id}/disable` — disable
- [ ] `POST /v5/accounts/subaccounts/{id}/enable` — enable

### Sending Limits
- [ ] `GET /v5/accounts/subaccounts/{id}/limit/custom/monthly` — get limit
- [ ] `POST /v5/accounts/subaccounts/{id}/limit/custom/monthly` — set limit
- [ ] `DELETE /v5/accounts/subaccounts/{id}/limit/custom/monthly` — remove limit

### Feature Flags
- [ ] `PUT /v5/accounts/subaccounts/{id}/features` — update features (`application/x-www-form-urlencoded` with JSON-stringified values)

### Resource Isolation
- [ ] Verify `X-Mailgun-On-Behalf-Of` middleware (from Phase 0) correctly scopes all resource operations
- [ ] Test subaccount isolation across domains, messages, events, suppressions

---

## Phase 13: Metrics & Analytics

Aggregate statistics derived from stored events.

> Plan doc: [metrics-and-analytics.md](./metrics-and-analytics.md)

### Legacy v3 Stats
- [ ] `GET /v3/stats/total` — account-level stats
- [ ] `GET /v3/stats/filter` — filtered account stats
- [ ] `GET /v3/stats/total/domains` — per-domain breakdown

### v1 Metrics API
- [ ] `POST /v1/analytics/metrics` — multi-dimensional query (dimensions: time, domain, tag, subaccount, etc.)
- [ ] Support up to 3 dimensions, 10 metrics per query
- [ ] Pagination with `cursor` and configurable `limit` (defaults differ by dimension)
- [ ] `include_aggregates` flag for rollup totals
- [ ] Rate metrics computed as strings (e.g., `"98.5%"`)

### Usage Metrics
- [ ] `POST /v1/analytics/usage/metrics` — usage data (messages sent, stored, etc.)

### Bounce Classification (stub)
- [ ] `POST /v2/bounce-classification/metrics` — bounce breakdown by classification

### Domain Aggregates
- [ ] `GET /v3/domains/{domain}/stats/aggregates/providers` — per-provider breakdown
- [ ] `GET /v3/domains/{domain}/stats/aggregates/devices` — per-device breakdown
- [ ] `GET /v3/domains/{domain}/stats/aggregates/countries` — per-country breakdown

---

## Phase 14: Web UI — Foundation

> Plan doc: [web-ui.md](./web-ui.md)

### App Shell & Routing
- [ ] Set up Vue Router with navigation for all pages
- [ ] Layout: sidebar navigation + main content area
- [ ] API client module (axios/fetch wrapper for `/mock/*` and standard Mailgun endpoints)
- [ ] Shared components: data tables with pagination, detail panels, status badges, toast notifications

### Dashboard
- [ ] `GET /mock/dashboard` — backend endpoint returning summary counts (messages, events, domains, etc.)
- [ ] Dashboard page: summary cards, recent messages list, recent events list

---

## Phase 15: Web UI — Messages & Events

### Messages View
- [ ] Messages list page: sortable table with from, to, subject, date, status
- [ ] `GET /mock/messages` — paginated message list endpoint (distinct from Mailgun's storage API)
- [ ] `GET /mock/messages/{id}` — full message detail endpoint
- [ ] Message detail page: headers, plain text body, HTML body (rendered in sandboxed iframe), attachments list
- [ ] `DELETE /mock/messages/{id}` — delete single message
- [ ] `POST /mock/messages/clear` — clear all messages

### Events View
- [ ] Events list page: filterable log with color-coded event types
- [ ] Filters: event type, recipient, date range
- [ ] Event detail panel: full event JSON

---

## Phase 16: Web UI — Management Pages

### Domains Page
- [ ] Domain list with status badges (active/unverified/disabled)
- [ ] Domain detail: DNS records, tracking settings, connection settings

### Suppressions Page
- [ ] Tabbed view: Bounces, Complaints, Unsubscribes, Allowlist
- [ ] Add/delete/import operations per suppression type

### Templates Page
- [ ] Template list with version counts
- [ ] Template detail: version list, version content preview, active version indicator

### Mailing Lists Page
- [ ] List of mailing lists with member counts
- [ ] Member management: add, edit, remove, bulk import

### Webhooks Page
- [ ] Webhook configuration: registered URLs per event type
- [ ] Delivery log: recent delivery attempts with status codes

### Routes Page
- [ ] Route list sorted by priority
- [ ] Route detail: expression, actions, created date

---

## Phase 17: Web UI — Testing & Real-time

### Testing Tools
- [ ] Event trigger panel: select message → trigger deliver/fail/open/click/unsubscribe/complaint
- [ ] Inbound simulation form: compose mock inbound email, submit to `/mock/inbound/{domain}`

### Mock Configuration Page
- [ ] Settings form for all mock config options (auto-delivery mode, verification mode, webhook retry, auth mode)
- [ ] Data reset buttons (reset all, reset messages, reset per-domain)

### Real-time Updates
- [ ] `GET /mock/ws` — WebSocket endpoint broadcasting events, messages, webhook deliveries
- [ ] Connect WebSocket in Vue app; auto-update message list, event log, webhook delivery log
- [ ] Connection status indicator in UI

---

## Notes

- **Field casing inconsistencies** are intentional and must match real Mailgun behavior — see [scratchpad.md](./scratchpad.md) for details per resource type.
- **API version discrepancies** (v3 vs v4 domains, v1/v3 IP pools, singular/plural tag paths) must all be handled — see [scratchpad.md](./scratchpad.md).
- **Timestamp formats** vary by resource: RFC 2822 (events), RFC 1123 (subaccounts), ISO 8601 (API keys), Unix epoch (flexible input).
- The Web UI phases (14–17) can be worked on in parallel with later backend phases once the API surface they depend on is available.
