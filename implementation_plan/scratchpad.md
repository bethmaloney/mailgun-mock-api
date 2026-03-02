# Scratchpad

Work items, notes, and things to explore in future iterations.

---

## Cross-cutting concerns

- ~~**SMTP submission support:** Deferred — v1 is HTTP API only.~~
- **Shared pagination utility:** Three pagination patterns exist across the API (cursor/URL-based, skip/limit offset, token-based). Build once and reuse across all list endpoints (suppressions, templates, tags, mailing lists, etc.).
- **Global mock configuration system:** Settings referenced across multiple plan docs (event generation, domain verification, webhook retry mode, auth mode, storage limits). The web-ui.md doc consolidates these into a unified `/mock/config` endpoint. During implementation, use a single config object passed to all subsystems.
- **`X-Mailgun-On-Behalf-Of` as cross-cutting concern:** This header affects ALL endpoints, not just subaccounts. The mock's middleware/auth layer needs to intercept it early and set the subaccount context for the entire request.
- **Field casing inconsistencies across resources:** The mock must preserve Mailgun's inconsistent field naming per resource type:
  - `createdAt` (camelCase) — templates, versions, allowlists
  - `created_at` (snake_case) — suppressions (bounces, complaints, unsubscribes)
  - `first-seen`/`last-seen` (hyphenated) — tags (legacy v3 API)
- **Shared stats computation:** Account-level, domain-level, and tag-level stats all use the same `StatsResponse` schema. Implement as a single reusable function with scope parameters.

## API version discrepancies

- **v3 vs v4 domain endpoints:** Domain list/create/get/update are v4 endpoints; delete, tracking, credentials, and DKIM management are v3 endpoints. The mock needs to handle both.
- **IP Pool version prefix:** OpenAPI spec uses `/v3/ip_pools`, SDKs use `/v1/ip_pools`. Accept both.
- **Tag path discrepancy:** OpenAPI spec defines `/v3/{domain}/tag` (singular) with `tag` as query param; SDKs use `/v3/{domain}/tags/{tag}` (plural) with tag in path. Support both.
- **v1 analytics uses POST for listing:** `/v1/analytics/tags` uses POST (not GET) with a JSON body for pagination/filters.

## Messages & storage

- **Storage key format:** Need to determine a format for mock storage keys. Real Mailgun uses opaque keys that encode storage region info.
- **Message retention:** The mock should have a configurable retention period (or just keep everything). This should be a global mock configuration option.
- **Attachment storage:** Store actual attachment bytes so users can retrieve content via the stored messages API.
- **Mailing list sends and `%recipient.varname%`:** When sending to a list address, member `vars` are available as `%recipient.varname%` template variables. The message sending pipeline needs to handle list expansion and per-recipient variable substitution.
- **`%mailing_list_unsubscribe_url%` variable:** The mock should generate a functional unsubscribe URL pointing back to the mock that marks the member as `subscribed: false`.

## Domains

- **DKIM key management endpoints:** `/v4/domains/{authority_name}/keys` and `/v1/dkim/keys` are production-only (real key generation/rotation). Stub with success responses in v1.
- **DKIM auto-rotation:** `/v1/dkim_management/domains/{name}/rotation` and `/v1/dkim_management/domains/{name}/rotate`. Production-only — stub in v1.
- **Domain state transitions:** Domains can be `active`, `unverified`, or `disabled`. The `disabled` state includes `code`, `reason`, `permanently`, and `until` fields. Support state transitions but don't enforce disable reasons.

## Events & logs

- **Logs API (v1/analytics/logs):** Newer POST-based analytics endpoint. Not used by major client libraries yet. Stub in v1.
- **Mock event trigger endpoints:** Non-standard endpoints (`/mock/events/{domain}/deliver/{message_id}`, etc.) for manually triggering event types. Include in v1.
- **Campaign tracking:** Events include a `campaigns` array field (legacy/deprecated). Accept campaign data but don't need a separate campaigns API.
- **Event log retention:** Keep all events by default with an optional configurable max. Global mock configuration option.

## Webhooks

- **Webhook test endpoint:** Node.js SDK references `PUT /v3/domains/{domain}/webhooks/{id}/test`. Support as a convenience — POST a test payload and return the response code.
- **Legacy webhook format:** Only support Webhooks 2.0 (JSON with `signature` + `event-data`). Skip 1.0 format.
- **Account-level webhook fields:** Account-level payloads include `domain-name` and `account-id` in `event-data` that domain-level webhooks don't. Consider how `account-id` maps to subaccount IDs.

## Suppressions

- **Unsubscribe `tag`/`tags` field inconsistency:** Form-data POST uses `tag` (singular string), JSON batch POST uses `tags` (plural array). Handle both field names.
- **Allowlist field naming:** Allowlist records use `createdAt` and `value`/`type`; other suppression types use `created_at` and `address`. Preserve these inconsistencies.
- **Complaint `count` field:** Go SDK and API docs show complaints have a `count` field (repeated complaints from same address), but OpenAPI spec omits it. Include it.

## Templates

- **Handlebars custom `equal` helper:** Mailgun's `equal` helper is non-standard — register as a custom helper alongside the standard `if`, `unless`, `each`, `with`.
- **MJML field in versions:** Always include `mjml` field in version responses (empty string `""` when not using MJML).
- **Template `headers` field encoding:** Sent as JSON-encoded string in `multipart/form-data` requests; returned as JSON object in responses. Parse on input, serialize on output.
- **Version `engine` field:** Defaults to `"handlebars"` if not provided.
- **Template name as pagination pivot:** Templates paginate by `name`; versions paginate by `tag`.

## Tags

- **All tag endpoints deprecated but still used:** Every `/v3/{domain}/tag*` endpoint is `deprecated: true`, but all SDKs still use them. Support both legacy v3 and v1 Analytics Tags API.
- **Tag per-message limit:** Legacy docs say 3, Ruby SDK enforces 10. Accept up to 10.
- **v3 stats `event` param is repeatable:** `?event=delivered&event=accepted` — the mock's query parser must handle repeated parameters.

## Mailing lists

- **List validation endpoints:** `POST/GET/DELETE /v3/lists/{address}/validate` — paid feature, stub in v1.
- **`list-id` field on update:** PUT endpoint accepts `list-id` parameter for the List-Id email header.
- **`reply_preference` can be null:** Handle null values in responses.
- **GET member response wrapping:** Use `{ "member": { ... } }` form (not bare), matching real-world behavior over OpenAPI spec.

## Routes

- **Inbound message simulation:** `POST /mock/inbound/{domain}` to simulate receiving inbound email and triggering route evaluation. Include in v1.
- **Route expression parser:** Handle `match_recipient()`, `match_header()`, `catch_all()`, and `and` operator. Capture group references (`\1`, `\g<name>`) are a stretch goal.
- **`forward()` to HTTP URL:** Use the same payload shape as webhook events — reasonable simplification.
- **Routes are account-scoped:** Only CRUD resource that is account-level rather than domain-scoped. Route operations should work with any valid API key.

## IPs & IP Pools

- **v5 subaccount DIPP delegation:** `/v5/accounts/subaccounts/ip_pools/...` endpoints. Complex async operations — skip unless Subaccounts plan requires it.
- **Alternate domain IP path:** Both `/v3/domains/{name}/ips/{ip}` and `/v3/domains/{name}/pool/{ip}` for the same delete operation. Handle both.
- **`o:ip-pool` message parameter:** Already documented in ips-and-pools.md; messages.md should reference it.
- **Asynchronous pool operations:** Return `"started"` (not `"success"`) for API compatibility, but process changes synchronously.

## Credentials & keys

- **Key rotation grace period:** Real Mailgun gives 48-hour grace period. The mock should delete immediately for simplicity.
- **API key format:** `key-` prefix + ~48 hex chars. Public keys use `pubkey-` prefix.
- **`spec` path parameter inconsistency:** PUT/DELETE use `{spec}` as local-part (e.g., `alice`), but DELETE response returns the full email in `spec`.
- **SMTP credentials not auto-created:** Domain creation should NOT auto-create SMTP credentials.
- **Request body encoding:** All credential/key operations use `multipart/form-data`, not JSON.

## Subaccounts

- **Account-level sending limits:** `/v5/accounts/limit/custom/monthly` endpoints. Stub in v1 alongside subaccount limits.
- **Resource isolation model:** Soft isolation — store `subaccount_id` on resources, filter in list endpoints. No separate namespaces per subaccount.
- **`include_subaccounts` query parameter:** Appears on domain list, events, and analytics endpoints. Filter accordingly.
- **`name` parameter encoding:** OpenAPI says query param for create, SDKs send form data. Accept both.
- **Python SDK has no subaccount support:** Python apps will use direct HTTP calls.
- **Feature update encoding:** `application/x-www-form-urlencoded` where each key contains JSON-stringified `{"enabled": true/false}`. Unusual pattern to parse.
- **`closed` vs `disabled` status:** Three states: `open`, `disabled`, `closed`. Support all three; `closed` unlikely in testing.

## Metrics & analytics

- **Skip rate limiting:** Real Mailgun enforces 500 req/10s on metrics API.
- **Rate metrics are strings:** `delivered_rate`, `opened_rate`, etc. are string values, not numbers.
- **Pagination defaults differ by dimension:** `time` dimension: default 1500 (max 1500). Others: default 10 (max 1000).
- **`include_aggregates` flag:** Include top-level `aggregates.metrics` rollup totals when true.
- **Skip deprecated bounce classification GET endpoints:** Use `POST /v2/bounce-classification/metrics` only.

## Web UI & mock-specific

- **Data reset endpoints:** `POST /mock/reset` and `POST /mock/reset/messages` for CI/CD. Include per-domain reset (`POST /mock/reset/{domain}`) in v1.
- **WebSocket design:** `/mock/ws` broadcasts all activity. Consider whether server-side filtering is needed or if client-side filtering suffices.
- **UI framework choice:** Decide during implementation. Lightweight (Vue, Svelte) preferred over heavy (React with full toolchain).
- **HTML email rendering security:** Iframe sandbox must prevent script execution and external resource loading.

## Low priority / skip

- **Dynamic IP pools enrollment:** Production-only, skip.
- **Domain-scoped sending queues:** Already covered in messages.md.
- **Domain IP management:** Covered in IPs & IP Pools area.
- **Ruby OptInHandler:** Client-side only, no mock work needed.
- **Ruby SDK missing list credentials:** Some apps won't use the list endpoint.
- **IP Allowlist (v2):** `/v2/ip_whitelist` — security/access-control, stub in Credentials & Keys if needed.
- **Unsubscribe tracking settings:** Already documented in domains.md.
- **Deprecated bounce classification GET endpoints:** Skip entirely.
