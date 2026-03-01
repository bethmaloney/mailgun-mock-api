# Scratchpad

Work items, notes, and things to explore in future iterations.

## Discovered during Messages research

- **SMTP sending:** The overview mentions "Accept messages via API/SMTP" but the Messages plan only covers HTTP API. SMTP ingestion (port 587/465) is a separate concern — consider whether the mock should support SMTP submission or just HTTP API. Add to a future iteration if needed.
- ~~**Template rendering:** Messages can reference templates by name (`template` field) and pass variables (`t:variables`). The Templates plan doc needs to cover how template rendering integrates with message sending (variable substitution, version resolution).~~ ✅ Covered in templates.md — "Template Rendering (Integration with Message Sending)" section
- ~~**Event generation from messages:** When a message is accepted, events (accepted, delivered, failed, etc.) need to be generated. The Events & Logs plan doc should define how message sending triggers event creation.~~ ✅ Covered in events-and-logs.md
- ~~**Webhook delivery from messages:** Accepted/delivered events should trigger webhook delivery if webhooks are configured. The Webhooks plan doc should cover this integration.~~ ✅ Covered in webhooks.md — "Integration Points" section defines the full event→webhook pipeline
- ~~**Suppression checking:** On send, Mailgun checks suppressions (bounces, complaints, unsubscribes) and may reject delivery. The Suppressions plan doc should cover how this integrates with sending.~~ ✅ Covered in suppressions.md — "Integration with Message Sending Pipeline" section
- **Storage key format:** Need to determine a good format for mock storage keys. Real Mailgun uses opaque keys that encode storage region info.
- **Message retention:** The mock should have a configurable message retention period (or just keep everything). Real Mailgun retains based on plan/domain settings.
- **Attachment storage:** Decide whether the mock stores actual attachment bytes or just metadata. For testing purposes, storing metadata (filename, size, content-type) may be sufficient, but some users may want to retrieve attachment content.

## Discovered during Domains research

- ~~**Domain-scoped webhooks:** The OpenAPI spec has webhook endpoints under `/v3/domains/{domain}/webhooks` (v3) and `/v4/domains/{domain}/webhooks` (v4). The Webhooks plan doc should cover both the v3 per-event-type model and the v4 URL+event_types model. These were documented in domains.md only as references — full webhook behavior belongs in webhooks.md.~~ ✅ Covered in webhooks.md — both v3 and v4 domain webhook APIs fully documented
- **Domain-scoped sending queues:** GET `/v3/domains/{name}/sending_queues` is already covered in messages.md. No duplication needed.
- **DKIM key management endpoints:** The OpenAPI spec includes `/v4/domains/{authority_name}/keys` for listing/activating/deactivating DKIM keys, and `/v1/dkim/keys` for legacy key management. These are production-only concerns (actual key generation, rotation). The mock should accept these calls and return success, but doesn't need real key material. Could be added as stubs if needed.
- **DKIM auto-rotation:** `/v1/dkim_management/domains/{name}/rotation` and `/v1/dkim_management/domains/{name}/rotate` endpoints exist for automatic key rotation. Production-only — stub if needed.
- **Domain IP management:** Endpoints exist for assigning/removing IPs from domains (`/v3/domains/{name}/ips/{ip}`). Covered under IPs & IP Pools area — not needed in domains doc.
- **Dynamic IP pools enrollment:** `/v3/domains/{name}/dynamic_pools` and bulk enrollment at `/v3/domains/all/dynamic_pools/enroll`. Production-only concern, skip for mock.
- **Domain state transitions:** Domains can be `active`, `unverified`, or `disabled`. The `disabled` state includes a nested object with `code`, `reason`, `permanently`, and `until` fields. The mock should support state transitions but doesn't need to enforce disable reasons.
- **v3 vs v4 API versions:** Domain list/create/get/update are v4 endpoints, while delete, tracking, credentials, and DKIM management are v3 endpoints. The mock needs to handle both API versions correctly.

## Discovered during Events & Logs research

- **Logs API (v1/analytics/logs):** The newer POST-based analytics endpoint supports complex filtering, metric aggregation, and unique event deduplication. It's not used by major client libraries yet. Stubbing it is sufficient for the mock — documented in events-and-logs.md as a stub target.
- ~~**Webhook delivery from events:** Events should trigger webhook delivery if webhooks are configured for those event types. The Webhooks plan doc needs to define: when an event is generated → check for matching webhook subscriptions → POST event payload to webhook URL. This is the core integration point between events and webhooks.~~ ✅ Covered in webhooks.md — "Webhook Delivery Pipeline" and "Integration Points" sections
- ~~**Suppression integration with events:** The events doc defines that suppressed recipients should generate `failed` events with appropriate `reason` values (`suppress-bounce`, `suppress-complaint`, `suppress-unsubscribe`). The Suppressions plan doc should document the lookup API that the message/event pipeline calls into.~~ ✅ Covered in suppressions.md — suppression check flow and event generation documented
- **Mock event trigger endpoints:** The events plan proposes mock-specific endpoints (`/mock/events/{domain}/deliver/{message_id}`, etc.) for manually triggering event types. These are non-standard Mailgun endpoints — they should be documented in the Web UI plan as part of the mock's testing/debugging tools.
- **Campaign tracking:** Events include a `campaigns` array field, but the Mailgun Campaigns API appears to be legacy/deprecated. The mock should accept campaign data on events but doesn't need a separate campaigns management area.
- **Event log retention:** Real Mailgun retains events for 1-30 days depending on plan. The mock should keep all events by default with an optional configurable max. This should be a global mock configuration option.

## Discovered during Webhooks research

- **Webhook test endpoint:** The Node.js SDK references `PUT /v3/domains/{domain}/webhooks/{id}/test` for testing that a webhook URL is reachable. This isn't well-documented in the API reference but exists in the SDK. The mock could support this as a convenience (POST a test payload to the URL and return the response code).
- **Legacy webhook format (pre-2018):** Mailgun originally sent webhooks as `application/x-www-form-urlencoded` or `multipart/form-data` (Webhooks 1.0). The current format (Webhooks 2.0) is JSON with `signature` + `event-data`. The mock only needs to support 2.0 format.
- **Global mock configuration:** Webhook retry mode (`immediate` vs `realistic`), signing key, and delivery logging are mock-wide settings. These should be part of a unified mock configuration system — needs to be defined as a cross-cutting concern, possibly in a separate config doc or in the Web UI plan.
- **Webhook delivery to localhost:** In local dev, webhook URLs will typically be `http://localhost:...`. The mock must accept HTTP URLs (not just HTTPS) unlike production Mailgun. This is already noted in the constraints table.
- **Account-level webhook `domain-name` field:** Account-level webhook payloads include `domain-name` and `account-id` fields in `event-data` that domain-level webhooks don't. The Subaccounts plan doc should consider how `account-id` maps to subaccount IDs.
- ~~**Suppression → webhook chain:** When a suppressed recipient generates a `failed` event with reason `suppress-*`, this should trigger `permanent_fail` webhooks. The Suppressions plan doc should define the lookup interface that the event pipeline calls.~~ ✅ Covered in suppressions.md — suppressed messages generate permanent_fail events which trigger webhooks per existing webhook pipeline

## Discovered during Suppressions research

- **IP Allowlist (v2):** There is a separate account-level API at `/v2/ip_whitelist` for managing which IPs can access the Mailgun API (CRUD for IP addresses with descriptions). This is a security/access-control feature, not a sending suppression. Stub in the Credentials & Keys plan doc if needed.
- **Unsubscribe tracking settings:** The `PUT /v3/domains/{name}/tracking/unsubscribe` endpoint configures auto-inserted unsubscribe link footers (html_footer, text_footer, active flag). Already documented in domains.md — referenced in suppressions.md for completeness.
- **Unsubscribe tag/tags field inconsistency:** Form-data POST uses `tag` (singular string), JSON batch POST uses `tags` (plural array). The mock must handle both field names correctly depending on content type.
- **Allowlist field naming inconsistency:** Allowlist records use `createdAt` (camelCase) and `value`/`type` fields, while other suppression types use `created_at` (snake_case) and `address`. The mock must preserve these inconsistencies to match the real API.
- **Complaint `count` field:** The Go SDK and API docs show complaints have a `count` field tracking repeated complaints from the same address, but the OpenAPI spec omits it. The mock should include it.
- **Shared pagination model:** All four suppression types plus other Mailgun list endpoints use the same cursor-based `paging` structure with `first`/`next`/`previous`/`last` URL fields. A shared pagination utility should be built once and reused across all list endpoints.

## Discovered during Templates research

- **Handlebars library choice:** The mock needs a Handlebars rendering library for template variable substitution. Mailgun uses a "customized version of Handlebars" with specific block helpers (`if`, `unless`, `each`, `with`, `equal`). The `equal` helper is non-standard Handlebars — it will need to be registered as a custom helper. A JS Handlebars library (e.g., `handlebars` npm package) would handle the standard helpers; `equal` needs a custom registration.
- **MJML field in versions:** Every version response includes an `mjml` field (empty string when not using MJML). The OpenAPI spec marks it as required in the Version schema. The mock should always include this field in version responses, set to `""`.
- **Template `headers` field is JSON-encoded string in requests:** When creating/updating templates/versions, the `headers` field is sent as a JSON-encoded string in `multipart/form-data`. The mock must parse this JSON string and store the resulting object. In responses, `headers` is returned as a JSON object (not a string).
- **Version `engine` field defaults:** If `engine` is not provided when creating a template or version, it defaults to `"handlebars"`. The OpenAPI spec marks `engine` as required in the Version response schema.
- **Template name used as pivot:** Pagination for templates uses the template `name` as the pivot (`p` parameter), while version pagination uses the version `tag`. This is consistent with the shared pagination model but the pivot field varies by resource type.
- **`createdAt` casing in templates:** Templates and versions use `createdAt` (camelCase), consistent with allowlists but different from suppressions (`created_at` snake_case). The mock must use the correct casing per resource type.

## Discovered during Tags research

- **All tag endpoints are deprecated in the OpenAPI spec:** Every `/v3/{domain}/tag*` endpoint is marked `deprecated: true`. However, all three client SDKs (Node, Ruby, Python) still actively use these endpoints. The v1 Analytics Tags API is the intended replacement, but SDK adoption is incomplete. The mock must support both.
- **OpenAPI spec vs SDK path discrepancy:** The OpenAPI spec defines tag CRUD at `/v3/{domain}/tag` (singular) with `tag` as a query parameter, but all SDKs use `/v3/{domain}/tags/{tag}` (plural) with tag in the path. The mock should support both forms.
- **Tag per-message limit inconsistency:** Legacy documentation says 3 tags per message, but the Ruby SDK enforces `MAX_TAGS = 10`. The mock should accept up to 10 per message.
- **Stats response shape is shared:** The `StatsResponse` schema (time-series with event buckets) is used by both tag stats (`/v3/{domain}/tags/{tag}/stats`) and domain stats (`/v3/{domain}/stats/total`). The Metrics & Analytics plan doc should reference this shared schema rather than re-defining it.
- **`first-seen`/`last-seen` use hyphenated keys:** Tags in the legacy v3 API use hyphenated keys (`first-seen`, `last-seen`), which is yet another casing convention (distinct from `createdAt` camelCase in templates/allowlists and `created_at` snake_case in suppressions). The mock must track and apply the correct casing per resource type.
- **v1 API uses POST for listing:** The new `/v1/analytics/tags` uses POST (not GET) for listing tags, with a JSON body containing pagination/filter parameters. This is unusual and should be noted in the Metrics & Analytics plan doc if that area covers the v1 analytics API more broadly.

## Discovered during Mailing Lists research

- **List validation endpoints:** `POST/GET/DELETE /v3/lists/{address}/validate` exist for email verification of list members. This is a paid Mailgun feature. Documented as stub-only in mailing-lists.md. All three SDKs support it (Node has dedicated methods, Python uses `validate=True` kwarg).
- **Ruby OptInHandler:** The Ruby SDK includes a client-side `OptInHandler` class for double opt-in flows using SHA1 HMAC hash generation/validation. This is purely client-side logic, not an API endpoint — the mock doesn't need to implement anything for it.
- **`list-id` field on update:** The PUT update endpoint accepts a `list-id` parameter that sets the List-Id email header value. This is exposed in the OpenAPI spec but not prominently documented.
- **`reply_preference` can be null in responses:** The Node.js SDK types define `reply_preference` as `null | string`. The mock should handle null values.
- **OpenAPI spec inconsistency for GET member:** The OpenAPI spec returns a bare `ListMemberResponse` for `GET /v3/lists/{address}/members/{member}`, but SDKs and docs expect the response wrapped in `{ "member": { ... } }`. The mock should use the wrapped form to match real-world behavior.
- **Mailing list sends and `%recipient.varname%`:** When sending to a list address, member `vars` are available as `%recipient.varname%` template variables. This is a cross-cutting concern with the Messages plan — the message sending pipeline needs to know about list expansion and per-recipient variable substitution.
- **`%mailing_list_unsubscribe_url%` variable:** Mailgun auto-generates unsubscribe URLs for list sends. The mock should generate a functional URL pointing back to the mock that marks the member as `subscribed: false`.
- **Two pagination styles in use:** The Node.js and Python SDKs use cursor-based pagination (`/pages` endpoints) while the Ruby SDK uses offset-based (`skip`/`limit`). Both must be supported. This reinforces the need for a shared pagination utility noted in the Suppressions research.

## Discovered during Routes research

- **Inbound message simulation endpoint:** The mock needs a non-standard endpoint (e.g. `POST /mock/inbound/{domain}`) to simulate receiving inbound email and triggering route evaluation. Real Mailgun receives email via MX records, which the mock can't replicate. This should be documented in the Web UI plan as a mock-specific testing tool.
- **Ruby SDK update response shape:** The Ruby SDK integration tests show the update response may return route fields at the top level (flat) rather than nested under `"route"`. The mock should return the nested form per the OpenAPI spec, but this is a potential SDK compatibility edge case.
- **Route expression parser:** The mock needs a simple expression parser that can handle `match_recipient("pattern")`, `match_header("header", "pattern")`, `catch_all()`, and the `and` operator. Python-style regex (used in expressions) maps well to JavaScript regex for basic patterns. Capture group references in actions (`\1`, `\g<name>`) are a stretch goal — not essential for mock testing.
- **`forward()` to HTTP URL:** When a route forwards to an HTTP URL, Mailgun POSTs the parsed email as either `multipart/form-data` (default) or JSON. The exact payload shape for route forwards differs slightly from webhook payloads. For the mock, using the same payload shape as webhook events is a reasonable simplification.
- **Routes are account-scoped:** Routes are the only CRUD resource in the core API that is account-level rather than domain-scoped. The mock's storage and authentication model needs to account for this — route operations should work with any valid API key, not just domain-specific keys.

## Discovered during IPs & IP Pools research

- ~~**Domain-IP assignment endpoints:** `GET/POST /v3/domains/{domain}/ips` and `DELETE /v3/domains/{domain}/ips/{ip}` are documented in ips-and-pools.md. The domains.md plan already references these as cross-cutting.~~ ✅ Covered in ips-and-pools.md
- **IP Pool version prefix discrepancy:** The OpenAPI spec documents pools at `/v3/ip_pools`, but Node.js and Python SDKs use `/v1/ip_pools`. The mock must accept both path prefixes. This is another instance of the v1/v3 version inconsistency pattern seen elsewhere (tags, analytics).
- ~~**IP Allowlist (`/v2/ip_whitelist`):** Account-level API for restricting which IPs can access the Mailgun API. CRUD with `ip_address` and `description` fields. This is a security/access-control feature — document in Credentials & Keys plan doc. (Also noted in Suppressions scratchpad.)~~ ✅ Covered in credentials-and-keys.md
- **v5 subaccount DIPP delegation:** Endpoints at `/v5/accounts/subaccounts/ip_pools/...` and `/v5/accounts/subaccounts/{id}/ip` for delegating pools and linking IPs to subaccounts. Complex async operations. Skip unless Subaccounts plan requires it.
- **Alternate domain IP path:** The OpenAPI spec defines both `/v3/domains/{name}/ips/{ip}` and `/v3/domains/{name}/pool/{ip}` as alternate paths for the same delete operation. The mock should handle both.
- **`o:ip-pool` message parameter:** Messages API accepts `o:ip-pool` to select a sending pool. Already documented in ips-and-pools.md integration section — the messages.md plan should reference this if it doesn't already.
- **Asynchronous pool operations:** Pool delete, IP add/remove to pools, and domain-pool link changes all return `"started"` (not `"success"`) because real Mailgun processes them asynchronously. The mock should return `"started"` for API compatibility but process changes synchronously.

## Discovered during Credentials & Keys research

- **Key rotation grace period:** When rotating an API key, the old key continues to work for 48 hours before expiring. The mock could optionally simulate this, but for simplicity, key deletion should be immediate. Note for future if apps test key rotation flows.
- **API key format:** Current keys use the prefix `key-` followed by ~48 hexadecimal characters. The mock should generate keys in this format for consistency.
- **Public key prefix:** Public validation keys use `pubkey-` prefix. The mock should maintain this convention.
- **`spec` path parameter inconsistency:** The PUT/DELETE credential endpoints use `{spec}` as the path parameter name, which represents the login local-part (e.g., `alice`), not the full email. But the DELETE response returns the full email in the `spec` field. The mock must handle this inconsistency.
- **Ruby SDK missing list credentials:** The Ruby SDK has no `list_smtp_credentials` method — it only supports create/update/delete. This means some apps may not use the list endpoint at all.
- **SMTP credentials no longer auto-created:** SMTP credentials are NOT auto-generated when creating a domain via the Domains API. They must be explicitly created. The mock's domain creation should NOT auto-create credentials.
- ~~**`X-Mailgun-On-Behalf-Of` header:** JS and Ruby SDKs support this header for subaccount context switching. The Subaccounts plan doc should define how this interacts with key validation.~~ ✅ Covered in subaccounts.md — header mechanism fully documented including SDK patterns and mock implementation guidance
- **Request body encoding for credentials:** All credential/key create/update operations use `multipart/form-data`, NOT JSON. The mock must accept form-data for these endpoints (consistent with most Mailgun write endpoints).

## Discovered during Subaccounts research

- **Account-level sending limits:** The `/v5/accounts/limit/custom/monthly` endpoints (GET/PUT/DELETE) and `/v5/accounts/limit/custom/enable` provide account-level (not subaccount-scoped) sending limit management. These use the same `CustomMessageLimitResponse` schema as the subaccount limit endpoints. The mock could stub these alongside the subaccount limits.
- **`X-Mailgun-On-Behalf-Of` as cross-cutting concern:** This header affects ALL endpoints, not just subaccounts. The mock's middleware/auth layer needs to intercept this header early and set the subaccount context for the entire request. This is an architectural concern that should be considered during implementation.
- **Subaccount resource isolation model:** The mock needs to decide whether to enforce strict isolation (subaccount A can't see subaccount B's domains) or soft isolation (store `subaccount_id` on resources, filter in list endpoints). Soft isolation is simpler and sufficient for testing.
- **`include_subaccounts` query parameter:** Multiple list/analytics endpoints accept `include_subaccounts=true` to include data from all subaccounts. This appears on domain list (`/v4/domains`), events (`/v3/{domain}/events`), and all analytics/metrics endpoints. The Metrics & Analytics plan doc should cover this parameter.
- **Subaccount `name` parameter encoding discrepancy:** The OpenAPI spec defines `name` as a query parameter for create, but all SDKs send it as form data. The mock should accept both.
- **Python SDK has no subaccount support:** The Python SDK only has partial IP pool routing related to subaccounts, not actual subaccount CRUD. This means Python-based apps testing subaccounts will use direct HTTP calls.
- **Feature update request encoding:** The features endpoint uses `application/x-www-form-urlencoded` where each feature key contains a JSON-stringified `{"enabled": true/false}` value. This is an unusual encoding pattern the mock must parse correctly.
- **`closed` vs `disabled` status:** The SubaccountStatus enum includes three values: `open`, `disabled`, and `closed`. The list endpoint has separate `enabled` and `closed` filter parameters, suggesting `closed` is a distinct terminal state different from `disabled`. The mock should support all three states but `closed` is unlikely to be used in testing.

## Discovered during Metrics & Analytics research

- **v3 stats `event` param is repeatable:** The `event` query parameter on `/v3/stats/total` can be specified multiple times (e.g., `?event=delivered&event=accepted`). The Node.js SDK explicitly supports passing an array for `event`. The mock's query parameter parser must handle repeated parameters.
- **v1 metrics rate limit:** Real Mailgun enforces 500 requests per 10 seconds on the metrics API. The mock should skip this rate limiting.
- **v1 metrics rates are strings:** Rate metrics (e.g., `delivered_rate`, `opened_rate`) are returned as string values, not numbers. The Go SDK types confirm these are strings. The mock must serialize rates as strings in JSON.
- **v1 metrics pagination defaults differ by dimension:** When `time` is a dimension, the default limit is 1500 (max 1500). For all other dimensions, default is 10 (max 1000). The mock should respect these defaults.
- **`include_aggregates` flag:** When true, the metrics response includes a top-level `aggregates.metrics` object containing rollup totals across all items. The mock should compute this from the item-level metrics.
- **Deprecated v1 bounce classification GET endpoints:** `GET /v1/bounce-classification/stats`, `GET /v1/bounce-classification/domains`, `GET /v1/bounce-classification/domains/{domain}/entities`, `GET /v1/bounce-classification/domains/{domain}/entities/{entity-id}/rules` — all deprecated in favor of `POST /v2/bounce-classification/metrics`. The mock can skip these entirely.
- **Shared stats computation architecture:** Account-level stats, domain-level stats, and tag-level stats all use the same `StatsResponse` schema and the same computation logic (filter events → bucket by resolution → count by type). This should be implemented as a single reusable function with scope parameters (account / domain / tag).

## Discovered during Web UI research

- **SMTP submission support:** The overview mentions "Accept messages via API/SMTP" but no plan doc covers SMTP ingestion (port 587/465). This is a significant feature gap — many developers test with SMTP clients, not just the HTTP API. Consider adding an SMTP plan doc or a section in messages.md.
- **Shared pagination utility:** Three pagination patterns exist across the API (cursor/URL-based, skip/limit offset, token-based). A shared pagination utility should be built once and reused. Noted previously in Suppressions research but reinforced by the OpenAPI spec analysis — the utility needs to support all three patterns.
- **Global mock configuration system:** Settings are referenced across multiple plan docs (event generation, domain verification, webhook retry mode, auth mode, storage limits). The web-ui.md doc consolidates these into a unified `/mock/config` endpoint. During implementation, these settings should be a single config object passed to all subsystems.
- **Data reset endpoints:** `POST /mock/reset` and `POST /mock/reset/messages` are essential for CI/CD workflows — tests need to start from a clean state. Consider also per-domain reset (`POST /mock/reset/{domain}`).
- **WebSocket design:** The `/mock/ws` WebSocket endpoint broadcasts all mock activity. During implementation, consider whether filtering (e.g., subscribe to events for a specific domain only) is needed, or if client-side filtering is sufficient for the expected message volume.
- **UI framework choice:** No framework is prescribed in the plan. This decision should be made during implementation based on team preferences. Lightweight options (Vue, Svelte) are recommended over heavier ones (React with full toolchain) since this is a dev tool, not a production app.
- **HTML email rendering security:** The message detail view renders HTML emails in an iframe sandbox. The sandbox must prevent script execution and external resource loading to avoid security issues from captured emails containing malicious content.
