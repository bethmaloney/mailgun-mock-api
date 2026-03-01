# Reverse Engineering Task

Your goal is to document how the Mailgun API and frontend works, so it can be implemented from scratch as a mock service using the specifications you produce.

## Your Goal

Select ONE feature from `implementation_plan/overview.md` that hasn't been documented yet, research it thoroughly, and document your findings.

## Process

1. **Read the overview**: Open `implementation_plan/overview.md` and find the first plan doc that hasn't been completely filled in yet (just has a heading)
2. **Read the scratchpad**: Check `implementation_plan/scratchpad.md` for any notes or work items from previous iterations
3. **Research the Mailgun API**: Use sub-agents to perform research:
   - Look up the real Mailgun API documentation for that area
   - Find endpoint definitions, request/response shapes, and behavior
   - Look at existing client libraries or examples for real-world usage patterns
4. **Write the plan doc**: Fill in the area's plan doc with:
   - The Mailgun API endpoints to mock (method, path, description)
   - Request/response schemas
   - Behavior notes (what the mock should do vs real Mailgun)
   - Any test scenarios worth supporting
   - Relevant source locations, files, and references used
5. **Update tracking files**:
   - Edit `implementation_plan/overview.md` to mark the doc as done or partial if additional work is required
   - Update `implementation_plan/scratchpad.md` with things to explore in future iterations
6. **Commit**: Create a commit with a descriptive message

## IMPORTANT

- Work on just ONE area per iteration
- Use sub-agents for parallel research when it helps (fetching multiple doc pages, checking client libraries, etc.)
- Document relevant locations, files, references, and URLs
- Research the real Mailgun API to ensure accuracy
- Focus on what's useful for a mock — skip fields/behaviors that only matter in production
- **If you discover a new area or dependency** while researching, add it to `implementation_plan/scratchpad.md` for a future iteration. Do NOT work on it now.

## Reference Links

### OpenAPI Spec
- **Local OpenAPI spec**: `mailgun.yaml` — the full Mailgun OpenAPI definition is available in the repo root. Use this as the primary source of truth for endpoint paths, parameters, request/response schemas, and status codes.

### API Documentation
- **API Overview**: https://documentation.mailgun.com/docs/mailgun/api-reference/api-overview
- **API Reference (all endpoints)**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun
- **Messages**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/messages
- **Domains**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/domains
- **Events**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/events
- **Stats**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/stats
- **Webhooks**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/webhooks
- **Routes**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/routes
- **Templates**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/templates
- **Tags**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/tags
- **Mailing Lists**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/mailing-lists
- **IPs**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/ips
- **IP Pools**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/ip-pools
- **Metrics**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/metrics
- **Subaccounts**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/subaccounts
- **Bounces**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/bounces
- **Complaints**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/complaints
- **Unsubscribes**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/unsubscribe
- **Credentials**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/credentials
- **Keys**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/keys

### Help Center / UI Behavior
- **Control Panel Overview**: https://help.mailgun.com/hc/en-us/articles/360021388013-The-Mailgun-Control-Panel
- **Reporting Dashboard**: https://help.mailgun.com/hc/en-us/articles/4402703701019-Reporting-Dashboard
- **Suppressions**: https://help.mailgun.com/hc/en-us/articles/360012287493-Suppressions-Bounces-Complaints-Unsubscribes-Allowlists
- **Routes**: https://help.mailgun.com/hc/en-us/articles/360011355893-Routes
- **Unsubscribe Handling**: https://help.mailgun.com/hc/en-us/articles/203306610-Unsubscribe-Handling-Links
- **API Keys & SMTP Credentials**: https://help.mailgun.com/hc/en-us/articles/203380100-Where-can-I-find-my-API-keys-and-SMTP-credentials
- **API Key Roles (RBAC)**: https://help.mailgun.com/hc/en-us/articles/26016288026907-API-Key-Roles

### Guides & User Manual
- **Getting Started (Sending)**: https://documentation.mailgun.com/docs/mailgun/api-reference/intro/
- **User Manual**: https://documentation.mailgun.com/en/latest/user_manual.html
- **FAQ Sending**: https://documentation.mailgun.com/docs/mailgun/faq/sending
- **FAQ Receiving**: https://documentation.mailgun.com/docs/mailgun/faq/receiving
- **API Key Management**: https://documentation.mailgun.com/docs/mailgun/user-manual/api-key-mgmt/rbac-mgmt

### Client Libraries (real-world usage patterns)
- **mailgun-js (Node)**: https://github.com/mailgun/mailgun.js
- **mailgun-ruby**: https://github.com/mailgun/mailgun-ruby
- **mailgun-python**: https://github.com/mailgun/mailgun-python

## Completion Signal

When ALL plan docs in the overview have been filled in and no items remain in the scratchpad, perform a FINAL check for any missing features/docs/specs. Update the scratchpad with any new findings.

If there are no new findings THEN output exactly:

```
ALL PLAN DOCS COMPLETE
```

This signals the automation to stop.
