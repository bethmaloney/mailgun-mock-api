# Scratchpad

Work items, notes, and things to explore in future iterations.

## Discovered during Messages research

- **SMTP sending:** The overview mentions "Accept messages via API/SMTP" but the Messages plan only covers HTTP API. SMTP ingestion (port 587/465) is a separate concern — consider whether the mock should support SMTP submission or just HTTP API. Add to a future iteration if needed.
- **Template rendering:** Messages can reference templates by name (`template` field) and pass variables (`t:variables`). The Templates plan doc needs to cover how template rendering integrates with message sending (variable substitution, version resolution).
- **Event generation from messages:** When a message is accepted, events (accepted, delivered, failed, etc.) need to be generated. The Events & Logs plan doc should define how message sending triggers event creation.
- **Webhook delivery from messages:** Accepted/delivered events should trigger webhook delivery if webhooks are configured. The Webhooks plan doc should cover this integration.
- **Suppression checking:** On send, Mailgun checks suppressions (bounces, complaints, unsubscribes) and may reject delivery. The Suppressions plan doc should cover how this integrates with sending.
- **Storage key format:** Need to determine a good format for mock storage keys. Real Mailgun uses opaque keys that encode storage region info.
- **Message retention:** The mock should have a configurable message retention period (or just keep everything). Real Mailgun retains based on plan/domain settings.
- **Attachment storage:** Decide whether the mock stores actual attachment bytes or just metadata. For testing purposes, storing metadata (filename, size, content-type) may be sufficient, but some users may want to retrieve attachment content.

## Discovered during Domains research

- **Domain-scoped webhooks:** The OpenAPI spec has webhook endpoints under `/v3/domains/{domain}/webhooks` (v3) and `/v4/domains/{domain}/webhooks` (v4). The Webhooks plan doc should cover both the v3 per-event-type model and the v4 URL+event_types model. These were documented in domains.md only as references — full webhook behavior belongs in webhooks.md.
- **Domain-scoped sending queues:** GET `/v3/domains/{name}/sending_queues` is already covered in messages.md. No duplication needed.
- **DKIM key management endpoints:** The OpenAPI spec includes `/v4/domains/{authority_name}/keys` for listing/activating/deactivating DKIM keys, and `/v1/dkim/keys` for legacy key management. These are production-only concerns (actual key generation, rotation). The mock should accept these calls and return success, but doesn't need real key material. Could be added as stubs if needed.
- **DKIM auto-rotation:** `/v1/dkim_management/domains/{name}/rotation` and `/v1/dkim_management/domains/{name}/rotate` endpoints exist for automatic key rotation. Production-only — stub if needed.
- **Domain IP management:** Endpoints exist for assigning/removing IPs from domains (`/v3/domains/{name}/ips/{ip}`). Covered under IPs & IP Pools area — not needed in domains doc.
- **Dynamic IP pools enrollment:** `/v3/domains/{name}/dynamic_pools` and bulk enrollment at `/v3/domains/all/dynamic_pools/enroll`. Production-only concern, skip for mock.
- **Domain state transitions:** Domains can be `active`, `unverified`, or `disabled`. The `disabled` state includes a nested object with `code`, `reason`, `permanently`, and `until` fields. The mock should support state transitions but doesn't need to enforce disable reasons.
- **v3 vs v4 API versions:** Domain list/create/get/update are v4 endpoints, while delete, tracking, credentials, and DKIM management are v3 endpoints. The mock needs to handle both API versions correctly.
