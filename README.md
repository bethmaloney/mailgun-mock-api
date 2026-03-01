# Mailgun Mock API

A mock Mailgun service for local development and testing. Accepts real Mailgun API calls, stores data for inspection, and simulates events — without sending real email.

Point your Mailgun client at this service instead of `api.mailgun.net` and everything Just Works, except no emails actually leave your machine.

## Why

- **Local development** — no Mailgun account needed, no accidental sends
- **CI/CD testing** — assert on email content, recipients, and events without network calls
- **Drop-in replacement** — uses the same API shape as real Mailgun, compatible with official SDKs

## Planned Features

| Area | Description |
|------|-------------|
| Messages | Accept messages via API, validate payload, store for inspection |
| Domains | Domain CRUD, controllable verification status, DNS records |
| Events & Logs | Generate realistic events for sent messages, event polling |
| Webhooks | Register webhooks, deliver event payloads, simulate events |
| Suppressions | Bounces, complaints, unsubscribes, allowlist — full CRUD |
| Templates | Template CRUD, versioning, variable rendering |
| Tags | Store tags on messages, return alongside stats |
| Mailing Lists | List and member CRUD, bulk operations |
| Routes | Inbound route management |
| Web UI | Inspect messages, view events, manage suppressions |

See [`implementation_plan/overview.md`](implementation_plan/overview.md) for the full breakdown.

## Status

**Early stage** — currently documenting the Mailgun API and planning the implementation. No runnable code yet.

## License

MIT
