# Scratchpad

Work items, notes, and things to explore in future iterations.

## Discovered during Messages research

- **SMTP sending:** The overview mentions "Accept messages via API/SMTP" but the Messages plan only covers HTTP API. SMTP ingestion (port 587/465) is a separate concern — consider whether the mock should support SMTP submission or just HTTP API. Add to a future iteration if needed.
- **Template rendering:** Messages can reference templates by name (`template` field) and pass variables (`t:variables`). The Templates plan doc needs to cover how template rendering integrates with message sending (variable substitution, version resolution).
- ~~**Event generation from messages:** When a message is accepted, events (accepted, delivered, failed, etc.) need to be generated. The Events & Logs plan doc should define how message sending triggers event creation.~~ ✅ Covered in events-and-logs.md
- ~~**Webhook delivery from messages:** Accepted/delivered events should trigger webhook delivery if webhooks are configured. The Webhooks plan doc should cover this integration.~~ ✅ Covered in webhooks.md — "Integration Points" section defines the full event→webhook pipeline
- **Suppression checking:** On send, Mailgun checks suppressions (bounces, complaints, unsubscribes) and may reject delivery. The Suppressions plan doc should cover how this integrates with sending.
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
- **Suppression integration with events:** The events doc defines that suppressed recipients should generate `failed` events with appropriate `reason` values (`suppress-bounce`, `suppress-complaint`, `suppress-unsubscribe`). The Suppressions plan doc should document the lookup API that the message/event pipeline calls into.
- **Mock event trigger endpoints:** The events plan proposes mock-specific endpoints (`/mock/events/{domain}/deliver/{message_id}`, etc.) for manually triggering event types. These are non-standard Mailgun endpoints — they should be documented in the Web UI plan as part of the mock's testing/debugging tools.
- **Campaign tracking:** Events include a `campaigns` array field, but the Mailgun Campaigns API appears to be legacy/deprecated. The mock should accept campaign data on events but doesn't need a separate campaigns management area.
- **Event log retention:** Real Mailgun retains events for 1-30 days depending on plan. The mock should keep all events by default with an optional configurable max. This should be a global mock configuration option.

## Discovered during Webhooks research

- **Webhook test endpoint:** The Node.js SDK references `PUT /v3/domains/{domain}/webhooks/{id}/test` for testing that a webhook URL is reachable. This isn't well-documented in the API reference but exists in the SDK. The mock could support this as a convenience (POST a test payload to the URL and return the response code).
- **Legacy webhook format (pre-2018):** Mailgun originally sent webhooks as `application/x-www-form-urlencoded` or `multipart/form-data` (Webhooks 1.0). The current format (Webhooks 2.0) is JSON with `signature` + `event-data`. The mock only needs to support 2.0 format.
- **Global mock configuration:** Webhook retry mode (`immediate` vs `realistic`), signing key, and delivery logging are mock-wide settings. These should be part of a unified mock configuration system — needs to be defined as a cross-cutting concern, possibly in a separate config doc or in the Web UI plan.
- **Webhook delivery to localhost:** In local dev, webhook URLs will typically be `http://localhost:...`. The mock must accept HTTP URLs (not just HTTPS) unlike production Mailgun. This is already noted in the constraints table.
- **Account-level webhook `domain-name` field:** Account-level webhook payloads include `domain-name` and `account-id` fields in `event-data` that domain-level webhooks don't. The Subaccounts plan doc should consider how `account-id` maps to subaccount IDs.
- **Suppression → webhook chain:** When a suppressed recipient generates a `failed` event with reason `suppress-*`, this should trigger `permanent_fail` webhooks. The Suppressions plan doc should define the lookup interface that the event pipeline calls.
