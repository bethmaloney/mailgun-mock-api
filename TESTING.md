# Mailgun Mock API — Testing Checklist

Test plan covering all implemented areas. Tests should use both the **Mailgun Go SDK** (`mailgun-go/v5`) and direct **HTTP calls** (validated against `mailgun.yaml` OpenAPI spec).

Endpoints are ordered so that dependencies are created before they are needed (e.g. domains before messages, messages before events).

---

## 1. Credentials & API Keys

Set up auth before anything else.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 1.1 | List API keys | GET | `/v1/keys` | `ListAPIKeys` |
| 1.2 | Create API key | POST | `/v1/keys` | `CreateAPIKey` |
| 1.3 | Delete API key | DELETE | `/v1/keys/{key_id}` | `DeleteAPIKey` |
| 1.4 | Regenerate public key | POST | `/v1/keys/public` | `RegeneratePublicAPIKey` |
| 1.5 | List SMTP credentials | GET | `/v3/domains/{domain}/credentials` | `ListCredentials` |
| 1.6 | Create SMTP credential | POST | `/v3/domains/{domain}/credentials` | `CreateCredential` |
| 1.7 | Update SMTP credential password | PUT | `/v3/domains/{domain}/credentials/{login}` | `ChangeCredentialPassword` |
| 1.8 | Delete SMTP credential | DELETE | `/v3/domains/{domain}/credentials/{login}` | `DeleteCredential` |

---

## 2. Domains

Domains must exist before sending messages or configuring other resources.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 2.1 | Create domain | POST | `/v4/domains` | `CreateDomain` |
| 2.2 | List domains | GET | `/v4/domains` | `ListDomains` |
| 2.3 | Get domain | GET | `/v4/domains/{name}` | `GetDomain` |
| 2.4 | Update domain | PUT | `/v4/domains/{name}` | `UpdateDomain` |
| 2.5 | Verify domain | PUT | `/v4/domains/{name}/verify` | `VerifyDomain` |
| 2.6 | Delete domain | DELETE | `/v3/domains/{name}` | `DeleteDomain` |

### 2a. Domain Connection Settings

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 2a.1 | Get connection settings | GET | `/v3/domains/{domain}/connection` | `GetDomainConnection` |
| 2a.2 | Update connection settings | PUT | `/v3/domains/{domain}/connection` | `UpdateDomainConnection` |

### 2b. Domain Tracking

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 2b.1 | Get tracking settings | GET | `/v3/domains/{domain}/tracking` | `GetDomainTracking` |
| 2b.2 | Update open tracking | PUT | `/v3/domains/{domain}/tracking/open` | `UpdateOpenTracking` |
| 2b.3 | Update click tracking | PUT | `/v3/domains/{domain}/tracking/click` | `UpdateClickTracking` |
| 2b.4 | Update unsubscribe tracking | PUT | `/v3/domains/{domain}/tracking/unsubscribe` | `UpdateUnsubscribeTracking` |

### 2c. Domain DKIM

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 2c.1 | Update DKIM authority | PUT | `/v3/domains/{domain}/dkim_authority` | `UpdateDomainDkimAuthority` |
| 2c.2 | Update DKIM selector | PUT | `/v3/domains/{domain}/dkim_selector` | `UpdateDomainDkimSelector` |
| 2c.3 | List domain DKIM keys | GET | `/v1/dkim/keys` | `ListDomainKeys` |
| 2c.4 | Create domain DKIM key | POST | `/v1/dkim/keys` | `CreateDomainKey` |
| 2c.5 | Activate DKIM key | PUT | `/v4/domains/{domain}/keys/{selector}/activate` | `ActivateDomainKey` |
| 2c.6 | Deactivate DKIM key | PUT | `/v4/domains/{domain}/keys/{selector}/deactivate` | `DeactivateDomainKey` |
| 2c.7 | Delete DKIM key | DELETE | `/v1/dkim/keys/{key_id}` | `DeleteDomainKey` |

---

## 3. Templates

Create templates before sending templated messages.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 3.1 | Create template | POST | `/v3/{domain}/templates` | `CreateTemplate` |
| 3.2 | List templates | GET | `/v3/{domain}/templates` | `ListTemplates` |
| 3.3 | Get template | GET | `/v3/{domain}/templates/{name}` | `GetTemplate` |
| 3.4 | Update template | PUT | `/v3/{domain}/templates/{name}` | `UpdateTemplate` |
| 3.5 | Delete template | DELETE | `/v3/{domain}/templates/{name}` | `DeleteTemplate` |

### 3a. Template Versions

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 3a.1 | Create version | POST | `/v3/{domain}/templates/{name}/versions` | `AddTemplateVersion` |
| 3a.2 | List versions | GET | `/v3/{domain}/templates/{name}/versions` | `ListTemplateVersions` |
| 3a.3 | Get version | GET | `/v3/{domain}/templates/{name}/versions/{tag}` | `GetTemplateVersion` |
| 3a.4 | Update version | PUT | `/v3/{domain}/templates/{name}/versions/{tag}` | `UpdateTemplateVersion` |
| 3a.5 | Delete version | DELETE | `/v3/{domain}/templates/{name}/versions/{tag}` | `DeleteTemplateVersion` |

---

## 4. Mailing Lists

Create lists and members before sending to list addresses.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 4.1 | Create mailing list | POST | `/v3/lists` | `CreateMailingList` |
| 4.2 | List mailing lists (offset) | GET | `/v3/lists` | `ListMailingLists` |
| 4.3 | List mailing lists (cursor) | GET | `/v3/lists/pages` | `ListMailingLists` |
| 4.4 | Get mailing list | GET | `/v3/lists/{address}` | `GetMailingList` |
| 4.5 | Update mailing list | PUT | `/v3/lists/{address}` | `UpdateMailingList` |
| 4.6 | Delete mailing list | DELETE | `/v3/lists/{address}` | `DeleteMailingList` |

### 4a. Mailing List Members

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 4a.1 | Add member | POST | `/v3/lists/{address}/members` | `CreateMember` |
| 4a.2 | List members (offset) | GET | `/v3/lists/{address}/members` | `ListMembers` |
| 4a.3 | List members (cursor) | GET | `/v3/lists/{address}/members/pages` | `ListMembers` |
| 4a.4 | Get member | GET | `/v3/lists/{address}/members/{member}` | `GetMember` |
| 4a.5 | Update member | PUT | `/v3/lists/{address}/members/{member}` | `UpdateMember` |
| 4a.6 | Delete member | DELETE | `/v3/lists/{address}/members/{member}` | `DeleteMember` |
| 4a.7 | Bulk add members (JSON) | POST | `/v3/lists/{address}/members.json` | `CreateMemberList` |

---

## 5. Routes

Set up inbound routing rules before simulating inbound email.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 5.1 | Create route | POST | `/v3/routes` | `CreateRoute` |
| 5.2 | List routes | GET | `/v3/routes` | `ListRoutes` |
| 5.3 | Get route | GET | `/v3/routes/{id}` | `GetRoute` |
| 5.4 | Update route | PUT | `/v3/routes/{id}` | `UpdateRoute` |
| 5.5 | Delete route | DELETE | `/v3/routes/{id}` | `DeleteRoute` |

---

## 6. Webhooks

Register webhooks before sending messages so event deliveries can be observed.

### 6a. Domain Webhooks (v3 — per-event-type)

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 6a.1 | Create webhook | POST | `/v3/domains/{domain}/webhooks` | `CreateWebhook` |
| 6a.2 | List webhooks | GET | `/v3/domains/{domain}/webhooks` | `ListWebhooks` |
| 6a.3 | Get webhook | GET | `/v3/domains/{domain}/webhooks/{name}` | `GetWebhook` |
| 6a.4 | Update webhook | PUT | `/v3/domains/{domain}/webhooks/{name}` | `UpdateWebhook` |
| 6a.5 | Delete webhook | DELETE | `/v3/domains/{domain}/webhooks/{name}` | `DeleteWebhook` |

### 6b. Account-Level Webhooks (v1)

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 6b.1 | Create account webhook | POST | `/v1/webhooks` | — |
| 6b.2 | List account webhooks | GET | `/v1/webhooks` | — |
| 6b.3 | Get account webhook | GET | `/v1/webhooks/{id}` | — |
| 6b.4 | Update account webhook | PUT | `/v1/webhooks/{id}` | — |
| 6b.5 | Delete account webhook | DELETE | `/v1/webhooks/{id}` | — |

### 6c. Webhook Signing Key

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 6c.1 | Get signing key | GET | `/v5/accounts/http_signing_key` | — |
| 6c.2 | Rotate signing key | POST | `/v5/accounts/http_signing_key` | — |

---

## 7. Email Sending (Messages)

Core functionality — send messages after domains, templates, and webhooks are in place.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 7.1 | Send plain-text message | POST | `/v3/{domain}/messages` | `Send` (via `NewMessage`) |
| 7.2 | Send HTML message | POST | `/v3/{domain}/messages` | `Send` (with `SetHtml`) |
| 7.3 | Send with attachments | POST | `/v3/{domain}/messages` | `Send` (with `AddAttachment`) |
| 7.4 | Send with inline images | POST | `/v3/{domain}/messages` | `Send` (with `AddInline`) |
| 7.5 | Send with tags | POST | `/v3/{domain}/messages` | `Send` (with `AddTag`) |
| 7.6 | Send with template | POST | `/v3/{domain}/messages` | `Send` (with `SetTemplate`) |
| 7.7 | Send with custom variables | POST | `/v3/{domain}/messages` | `Send` (with `AddVariable`) |
| 7.8 | Send with recipient variables | POST | `/v3/{domain}/messages` | `Send` (with `AddRecipientAndVariables`) |
| 7.9 | Send with scheduled delivery | POST | `/v3/{domain}/messages` | `Send` (with `SetDeliveryTime`) |
| 7.10 | Send MIME message | POST | `/v3/{domain}/messages.mime` | `Send` (via `NewMIMEMessage`) |
| 7.11 | Send in test mode | POST | `/v3/{domain}/messages` | `Send` (with `EnableTestMode`) |
| 7.12 | Send with tracking overrides | POST | `/v3/{domain}/messages` | `Send` (with `SetTracking*`) |
| 7.13 | Send with require-TLS | POST | `/v3/{domain}/messages` | `Send` (with `SetRequireTLS`) |

### 7a. Stored Messages

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 7a.1 | Get stored message | GET | `/v3/domains/{domain}/messages/{key}` | `GetStoredMessage` |
| 7a.2 | Get stored message (raw MIME) | GET | `/v3/domains/{domain}/messages/{key}` | `GetStoredMessageRaw` |
| 7a.3 | Resend stored message | POST | `/v3/domains/{domain}/messages/{key}` | `ReSend` |

### 7b. Sending Queues

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 7b.1 | Get queue status | GET | `/v3/{domain}/sending_queues` | — |
| 7b.2 | Clear queue | DELETE | `/v3/{domain}/envelopes` | — |

---

## 8. Events & Logs

Query events generated from sent messages.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 8.1 | List events (with filters) | GET | `/v3/{domain}/events` | `ListEvents` |
| 8.2 | Poll events | GET | `/v3/{domain}/events` (polling) | `PollEvents` |
| 8.3 | Filter by event type | GET | `/v3/{domain}/events?event=delivered` | `ListEvents` (with filter) |
| 8.4 | Filter by recipient | GET | `/v3/{domain}/events?recipient=...` | `ListEvents` (with filter) |
| 8.5 | Filter by time range | GET | `/v3/{domain}/events?begin=...&end=...` | `ListEvents` (with filter) |
| 8.6 | Paginate events | GET | `/v3/{domain}/events/{page_token}` | Iterator `.Next()` |
| 8.7 | Query logs API | POST | `/v1/analytics/logs` | — |

### Event types to verify

- `accepted` — message accepted by mock
- `delivered` — simulated delivery
- `failed` (permanent / temporary) — simulated failure
- `rejected` — rejected message
- `opened` — simulated open
- `clicked` — simulated click
- `unsubscribed` — simulated unsubscribe
- `complained` — simulated spam complaint
- `stored` — message stored for retrieval

---

## 9. Tags

Verify tags created from sent messages.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 9.1 | List tags | GET | `/v3/{domain}/tags` | `ListTags` |
| 9.2 | Get tag | GET | `/v3/{domain}/tags/{tag}` | `GetTag` |
| 9.3 | Update tag description | PUT | `/v3/{domain}/tags/{tag}` | — |
| 9.4 | Delete tag | DELETE | `/v3/{domain}/tags/{tag}` | `DeleteTag` |
| 9.5 | Get tag stats | GET | `/v3/{domain}/tags/{tag}/stats` | — |
| 9.6 | Get tag aggregates (countries) | GET | `/v3/{domain}/tags/{tag}/stats/aggregates/countries` | — |
| 9.7 | Get tag aggregates (providers) | GET | `/v3/{domain}/tags/{tag}/stats/aggregates/providers` | — |
| 9.8 | Get tag aggregates (devices) | GET | `/v3/{domain}/tags/{tag}/stats/aggregates/devices` | — |
| 9.9 | Get tag limits | GET | `/v3/domains/{domain}/limits/tag` | `GetTagLimits` |

---

## 10. Suppressions

Test after sending messages so there are addresses to suppress.

### 10a. Bounces

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 10a.1 | Add bounce | POST | `/v3/{domain}/bounces` | `AddBounce` |
| 10a.2 | List bounces | GET | `/v3/{domain}/bounces` | `ListBounces` |
| 10a.3 | Get bounce | GET | `/v3/{domain}/bounces/{address}` | `GetBounce` |
| 10a.4 | Delete bounce | DELETE | `/v3/{domain}/bounces/{address}` | `DeleteBounce` |
| 10a.5 | Delete all bounces | DELETE | `/v3/{domain}/bounces` | `DeleteBounceList` |
| 10a.6 | Bulk import bounces | POST | `/v3/{domain}/bounces` (JSON array) | `AddBounces` |

### 10b. Complaints

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 10b.1 | Add complaint | POST | `/v3/{domain}/complaints` | `CreateComplaint` |
| 10b.2 | List complaints | GET | `/v3/{domain}/complaints` | `ListComplaints` |
| 10b.3 | Get complaint | GET | `/v3/{domain}/complaints/{address}` | `GetComplaint` |
| 10b.4 | Delete complaint | DELETE | `/v3/{domain}/complaints/{address}` | `DeleteComplaint` |
| 10b.5 | Bulk import complaints | POST | `/v3/{domain}/complaints` (JSON array) | `CreateComplaints` |

### 10c. Unsubscribes

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 10c.1 | Add unsubscribe | POST | `/v3/{domain}/unsubscribes` | `CreateUnsubscribe` |
| 10c.2 | List unsubscribes | GET | `/v3/{domain}/unsubscribes` | `ListUnsubscribes` |
| 10c.3 | Get unsubscribe | GET | `/v3/{domain}/unsubscribes/{address}` | `GetUnsubscribe` |
| 10c.4 | Delete unsubscribe | DELETE | `/v3/{domain}/unsubscribes/{address}` | `DeleteUnsubscribe` |
| 10c.5 | Delete unsubscribe (by tag) | DELETE | `/v3/{domain}/unsubscribes/{address}?tag={tag}` | `DeleteUnsubscribeWithTag` |
| 10c.6 | Bulk import unsubscribes | POST | `/v3/{domain}/unsubscribes` (JSON array) | `CreateUnsubscribes` |

### 10d. Allowlist (Whitelists)

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 10d.1 | Add to allowlist | POST | `/v3/{domain}/whitelists` | — |
| 10d.2 | List allowlist | GET | `/v3/{domain}/whitelists` | — |
| 10d.3 | Get allowlist entry | GET | `/v3/{domain}/whitelists/{value}` | — |
| 10d.4 | Delete from allowlist | DELETE | `/v3/{domain}/whitelists/{value}` | — |

---

## 11. IPs & IP Pools

Stub area — verify endpoints accept calls and return plausible data.

### 11a. Account IPs

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 11a.1 | List IPs | GET | `/v3/ips` | `ListIPs` |
| 11a.2 | Get IP | GET | `/v3/ips/{ip}` | `GetIP` |

### 11b. Domain IPs

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 11b.1 | List domain IPs | GET | `/v3/domains/{domain}/ips` | `ListDomainIPs` |
| 11b.2 | Assign IP to domain | POST | `/v3/domains/{domain}/ips` | `AddDomainIP` |
| 11b.3 | Remove IP from domain | DELETE | `/v3/domains/{domain}/ips/{ip}` | `DeleteDomainIP` |

### 11c. IP Pools

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 11c.1 | List IP pools | GET | `/v1/ip_pools` | — |
| 11c.2 | Create IP pool | POST | `/v1/ip_pools` | — |
| 11c.3 | Get IP pool | GET | `/v1/ip_pools/{pool_id}` | — |
| 11c.4 | Update IP pool | PATCH | `/v1/ip_pools/{pool_id}` | — |
| 11c.5 | Delete IP pool | DELETE | `/v1/ip_pools/{pool_id}` | — |

---

## 12. Subaccounts

Stub area — verify basic CRUD and on-behalf-of header.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 12.1 | Create subaccount | POST | `/v5/accounts/subaccounts` | `CreateSubaccount` |
| 12.2 | List subaccounts | GET | `/v5/accounts/subaccounts` | `ListSubaccounts` |
| 12.3 | Get subaccount | GET | `/v5/accounts/subaccounts/{id}` | `GetSubaccount` |
| 12.4 | Enable subaccount | POST | `/v5/accounts/subaccounts/{id}/enable` | `EnableSubaccount` |
| 12.5 | Disable subaccount | POST | `/v5/accounts/subaccounts/{id}/disable` | `DisableSubaccount` |
| 12.6 | Verify X-Mailgun-On-Behalf-Of header | — | (all endpoints) | `SetOnBehalfOfSubaccount` |

---

## 13. Metrics & Analytics

Query stats after messages have been sent and events generated.

| # | Endpoint / Operation | Method | Path | SDK Method |
|---|---|---|---|---|
| 13.1 | Get domain stats | GET | `/v3/{domain}/stats/total` | `GetStats` |
| 13.2 | Get account stats | GET | `/v3/stats/total` | `GetStats` |
| 13.3 | Query metrics API | POST | `/v1/analytics/metrics` | `ListMetrics` |
| 13.4 | Query usage metrics | POST | `/v1/analytics/usage/metrics` | — |
| 13.5 | Get domain aggregates (providers) | GET | `/v3/{domain}/aggregates/providers` | — |
| 13.6 | Get domain aggregates (countries) | GET | `/v3/{domain}/aggregates/countries` | — |
| 13.7 | Get domain aggregates (devices) | GET | `/v3/{domain}/aggregates/devices` | — |

---

## 14. Mock-Specific Endpoints

Control-plane endpoints unique to the mock server.

| # | Endpoint / Operation | Method | Path | Notes |
|---|---|---|---|---|
| 14.1 | Dashboard summary | GET | `/mock/dashboard` | Message/event counts |
| 14.2 | List captured messages | GET | `/mock/messages` | Browse stored messages |
| 14.3 | Get captured message detail | GET | `/mock/messages/{id}` | Full message content |
| 14.4 | Delete captured message | DELETE | `/mock/messages/{id}` | — |
| 14.5 | Trigger deliver event | POST | `/mock/events/{domain}/deliver/{message_id}` | Simulate delivery |
| 14.6 | Trigger fail event | POST | `/mock/events/{domain}/fail/{message_id}` | Simulate failure |
| 14.7 | Trigger open event | POST | `/mock/events/{domain}/open/{message_id}` | Simulate open |
| 14.8 | Trigger click event | POST | `/mock/events/{domain}/click/{message_id}` | Simulate click |
| 14.9 | Trigger unsubscribe event | POST | `/mock/events/{domain}/unsubscribe/{message_id}` | Simulate unsub |
| 14.10 | Trigger complain event | POST | `/mock/events/{domain}/complain/{message_id}` | Simulate complaint |
| 14.11 | Webhook delivery log | GET | `/mock/webhooks/deliveries` | Inspect webhook deliveries |
| 14.12 | Simulate inbound email | POST | `/mock/inbound/{domain}` | Test routes |
| 14.13 | Get mock config | GET | `/mock/config` | Current settings |
| 14.14 | Update mock config | PUT | `/mock/config` | Change behavior |
| 14.15 | Reset all data | POST | `/mock/reset` | Clear everything |
| 14.16 | WebSocket live updates | WS | `/mock/ws` | Real-time push |

---

## Suggested Test Flow

A recommended order for an end-to-end integration test run:

1. **Reset** — `POST /mock/reset` to start clean
2. **Config** — `GET/PUT /mock/config` to set desired mock behavior
3. **Credentials** — Create API keys / SMTP credentials
4. **Domains** — Create and verify a domain, configure tracking
5. **Templates** — Create a template with versions
6. **Mailing Lists** — Create a list, add members
7. **Routes** — Create inbound routing rules
8. **Webhooks** — Register webhooks for event types
9. **Send Messages** — Send various message types (plain, HTML, template, MIME, with tags, attachments)
10. **Stored Messages** — Retrieve and resend stored messages
11. **Events** — Query events, filter by type/recipient/time, paginate
12. **Tags** — List tags created from sent messages, query tag stats
13. **Trigger Events** — Use mock endpoints to simulate deliver/open/click/fail/complain/unsubscribe
14. **Webhooks Verify** — Check webhook delivery log for delivered payloads
15. **Suppressions** — Add/list/delete bounces, complaints, unsubscribes, allowlist entries
16. **Metrics** — Query stats and analytics after events exist
17. **IPs & Pools** — Verify stub endpoints return data
18. **Subaccounts** — CRUD and on-behalf-of header
19. **Simulate Inbound** — POST to `/mock/inbound/{domain}`, verify route matching
20. **Cleanup** — Delete domain, verify cascade cleanup
