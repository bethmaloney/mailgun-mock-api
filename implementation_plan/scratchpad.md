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
