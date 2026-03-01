# Mailgun Mock API — Implementation Plan

A mock Mailgun service for dev/testing. Accepts real Mailgun API calls, stores data for inspection, and simulates events — without sending real email.

## Core Areas

| # | Area | Plan | Description |
|---|------|------|-------------|
| 1 | Email Sending (Messages) | [messages.md](./messages.md) | Accept messages via API/SMTP, validate payload shape, store for inspection |
| 2 | Domains | [domains.md](./domains.md) | Domain CRUD, auto-verify or controllable verification status, DNS records |
| 3 | Events & Logs | [events-and-logs.md](./events-and-logs.md) | Generate realistic events for sent messages, support event polling |
| 4 | Webhooks | [webhooks.md](./webhooks.md) | Register webhooks, deliver event payloads, simulate/trigger events |
| 5 | Suppressions | [suppressions.md](./suppressions.md) | Bounces, complaints, unsubscribes, allowlist — full CRUD |
| 6 | Templates | [templates.md](./templates.md) | Template CRUD, versioning, variable rendering |
| 7 | Tags | [tags.md](./tags.md) | Store tags on messages, return alongside stats |
| 8 | Mailing Lists | [mailing-lists.md](./mailing-lists.md) | List and member CRUD, bulk operations |
| 9 | Routes (Receiving) | [routes.md](./routes.md) | Inbound route management, stored message retrieval |

## Stub Areas (minimal — accept calls, return 200)

| # | Area | Plan | Description |
|---|------|------|-------------|
| 10 | IPs & IP Pools | [ips-and-pools.md](./ips-and-pools.md) | Return static IP/pool data if app assigns pools to domains |
| 11 | Credentials & Keys | [credentials-and-keys.md](./credentials-and-keys.md) | Basic API key gating, accept any key or configurable keys |
| 12 | Subaccounts | [subaccounts.md](./subaccounts.md) | Basic CRUD if app uses multi-tenancy |
| 13 | Metrics & Analytics | [metrics-and-analytics.md](./metrics-and-analytics.md) | Return stats derived from messages sent to the mock |

## UI / Control Panel

| # | Area | Plan | Description |
|---|------|------|-------------|
| 14 | Web UI | [web-ui.md](./web-ui.md) | Inspect captured messages, view events, trigger webhook events, manage suppressions |

## Skipped Areas (not useful in mock/test context)

- Inbox Placement & Deliverability (Mailgun Optimize)
- Pre-Send Testing (Mailgun Inspect)
- IP Warmup
- Dynamic IP Pools
- Email Validation (stub if needed)
- Security & Access Control (2FA, SSO)
- Billing / Account Settings
