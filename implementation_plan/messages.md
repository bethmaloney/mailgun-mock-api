# Email Sending (Messages)

Accept messages via API, validate payload shape, store for inspection, and generate events. The mock does NOT actually deliver email — it stores everything for later retrieval and inspection.

## Endpoints

### 1. POST `/v3/{domain_name}/messages` — Send an email

The primary sending endpoint. Accepts `multipart/form-data` with message components (from, to, subject, body, attachments, options). Mailgun assembles a MIME message and queues it.

**Auth:** HTTP Basic (`api:<key>`)

#### Path Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `domain_name` | string | yes | Sending domain |

#### Request Body (`multipart/form-data`)

**Required fields:** `from`, `to`, `subject` (except when using a template with pre-set headers)

**Core fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | yes* | Sender address. Supports `"Name <email>"` format. *Not required if template provides it. |
| `to` | string[] | yes | Recipient(s). Comma-separated or repeated param. Supports `"Name <email>"`. Duplicates auto-removed. |
| `cc` | string[] | no | Carbon copy recipients |
| `bcc` | string[] | no | Blind carbon copy recipients |
| `subject` | string | yes* | Message subject. *Not required if template provides it. |
| `text` | string | no | Plain text body |
| `html` | string | no | HTML body |
| `amp-html` | string | no | AMP HTML body |
| `attachment` | binary[] | no | File attachments (multipart uploads). Each must have a filename. |
| `inline` | binary[] | no | Inline image attachments (referenced via `cid:` in HTML) |

At least one of `text`, `html`, `amp-html`, or `template` must be provided.

**Template fields (`t:` prefix):**

| Field | Type | Description |
|-------|------|-------------|
| `template` | string | Name of stored template to render |
| `t:version` | string | Specific template version tag |
| `t:text` | enum: `"yes"` | Auto-generate plain text from template HTML |
| `t:variables` | string (JSON) | Template variable values as JSON dict |

**Sending options (`o:` prefix):**

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `o:tag` | string[] | — | Tags for categorization. Max 10 per message. |
| `o:testmode` | string | `"yes"` | Process/validate but don't deliver |
| `o:deliverytime` | string | RFC-2822 date | Schedule delivery (max 3-7 days ahead) |
| `o:deliver-within` | string | `NhNm` | Max delivery window (min 5m, max 24h) |
| `o:deliverytime-optimize-period` | string | `Nh` | Send Time Optimization (24h-72h) |
| `o:time-zone-localize` | string | `HH:mm` or `hh:mmaa` | Timezone Optimization |
| `o:tracking` | string | `yes\|no\|true\|false\|htmlonly` | Toggle click + open tracking |
| `o:tracking-clicks` | string | `yes\|no\|true\|false\|htmlonly` | Toggle click tracking |
| `o:tracking-opens` | string | `yes\|no\|true\|false` | Toggle open tracking |
| `o:tracking-pixel-location-top` | string | `yes\|no\|true\|false\|htmlonly` | Move tracking pixel to top |
| `o:dkim` | string | `yes\|no\|true\|false` | Toggle DKIM per-message |
| `o:secondary-dkim` | string | `domain/selector` | Secondary DKIM signing key |
| `o:secondary-dkim-public` | string | `domain/selector` | Alias for secondary DKIM |
| `o:require-tls` | string | `yes\|no\|true\|false` | Require TLS for delivery |
| `o:skip-verification` | string | `yes\|no\|true\|false` | Skip TLS cert verification |
| `o:sending-ip` | string | — | Specific sending IP |
| `o:sending-ip-pool` | string | — | IP Pool ID |
| `o:archive-to` | string | URL | HTTP POST copy to URL |
| `o:suppress-headers` | string | header names or `"all"` | Remove X-Mailgun headers |

**Custom headers (`h:` prefix):** Any field prefixed with `h:` becomes a MIME header (e.g., `h:Reply-To`, `h:X-Custom`).

**Custom variables (`v:` prefix):** Any field prefixed with `v:` is attached as metadata, available in events/webhooks. Max 4KB in events (truncated).

**Batch sending:**

| Field | Type | Description |
|-------|------|-------------|
| `recipient-variables` | string (JSON) | Per-recipient variables. Max 1,000 recipients. |

When `recipient-variables` is provided, each `to` recipient gets an individual email and only sees their own address. Body can use `%recipient.varname%` syntax. Without recipient-variables, all `to` recipients see each other's addresses.

#### Success Response (200)

```json
{
  "id": "<20230101120000.abc123def456@yourdomain.com>",
  "message": "Queued. Thank you."
}
```

The `id` follows RFC-2392 format: `<timestamp.random@domain>`.

#### Error Responses

| Status | Body | When |
|--------|------|------|
| 400 | `{"message": "..."}` | Missing/invalid params (e.g., `"from parameter is missing"`, `"to parameter is not a valid address"`, `"Need at least one of 'text', 'html', 'amp-html' or 'template' parameters specified"`) |
| 401 | `"Forbidden"` | Invalid/missing API key |
| 429 | `{"message": "..."}` | Rate limit exceeded |
| 500 | `{"message": "Internal Server Error"}` | Server error |

---

### 2. POST `/v3/{domain_name}/messages.mime` — Send MIME email

Same as above but the caller builds the MIME message themselves.

**Required fields:** `to`, `message`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | string[] | yes | Recipients |
| `message` | binary | yes | Pre-built MIME message (file upload) |

All `o:`, `h:`, `v:`, `t:`, and `recipient-variables` fields are supported. The `from`, `subject`, `cc`, `bcc`, `text`, `html`, `attachment`, and `inline` fields are NOT used — they come from the MIME body.

Response shape is identical to the standard send endpoint.

---

### 3. GET `/v3/domains/{domain_name}/messages/{storage_key}` — Retrieve stored email

Retrieve a previously sent message by its storage key (found in event data).

#### Path Parameters

| Name | Type | Description |
|------|------|-------------|
| `domain_name` | string | Domain the message was sent from |
| `storage_key` | string | Storage key from event's `storage.key` field |

#### Success Response (200)

```json
{
  "Content-Transfer-Encoding": "7bit",
  "Content-Type": "multipart/form-data; boundary=...",
  "From": "sender@domain.com",
  "Message-Id": "<id@domain.com>",
  "Mime-Version": "1.0",
  "Subject": "Hello",
  "To": "recipient@example.com",
  "X-Mailgun-Tag": "tag-name",
  "sender": "sender@domain.com",
  "recipients": "recipient@example.com",
  "body-html": "<html>...</html>",
  "body-plain": "plain text",
  "stripped-html": "<html>...</html>",
  "stripped-text": "plain text",
  "stripped-signature": "signature block",
  "message-headers": [
    ["Mime-Version", "1.0"],
    ["Subject", "Hello"],
    ["From", "sender@domain.com"],
    ["To", "recipient@example.com"]
  ],
  "X-Mailgun-Template-Name": "template-name",
  "X-Mailgun-Template-Variables": "{\"key\":\"value\"}"
}
```

#### Error Responses

| Status | Body |
|--------|------|
| 400 | `{"message": "..."}` |
| 404 | `{"message": "Message not found"}` |

---

### 4. POST `/v3/domains/{domain_name}/messages/{storage_key}` — Resend email

Resend a previously stored message to new recipients.

#### Request Body (`multipart/form-data`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | string | yes | New recipient(s), comma-separated |

#### Success Response (200)

```json
{
  "id": "<new-message-id@domain>",
  "message": "Queued. Thank you."
}
```

---

### 5. GET `/v3/domains/{name}/sending_queues` — Queue status

Returns sending queue status for a domain.

#### Success Response (200)

```json
{
  "regular": {
    "is_disabled": false,
    "disabled": { "until": "RFC-822 date", "reason": "description" }
  },
  "scheduled": {
    "is_disabled": false,
    "disabled": { "until": "RFC-822 date", "reason": "description" }
  }
}
```

#### Error Responses

| Status | Body |
|--------|------|
| 401 | `{"message": "Invalid private key"}` |
| 404 | `{"message": "Domain not found"}` |

---

### 6. DELETE `/v3/{domain_name}/envelopes` — Clear queue

Deletes all scheduled and undelivered mail from the domain queue.

#### Success Response (200)

```json
{ "message": "done" }
```

---

## Validation Rules

The mock should validate these constraints (derived from API spec + Go SDK):

1. **Required fields:** `from`, `to`, `subject` must be present (unless template provides from/subject)
2. **Body required:** At least one of `text`, `html`, `amp-html`, or `template` must be provided
3. **Recipient count:** Total (to + cc + bcc) must be > 0 and <= 1,000
4. **Email format:** `from`, `to`, `cc`, `bcc` must be valid email addresses or `"Name <email>"` format
5. **Tags limit:** Max 10 tags per message
6. **Send options size:** Combined `o:`, `h:`, `v:`, `t:` parameters must be <= 16KB
7. **Duplicate filtering:** Duplicate recipients in to/cc/bcc are silently removed
8. **Domain validation:** Domain must exist (return 404 if not)
9. **Recipient variables limit:** Max 1,000 recipients when using `recipient-variables`
10. **Boolean params:** Accept both `"yes"`/`"no"` and `"true"`/`"false"` strings
11. **Scheduling:** `o:deliverytime` must be RFC-2822 format, max 3-7 days in future
12. **STO period:** `o:deliverytime-optimize-period` must be 24h-72h, incompatible with multiple recipients

## Mock Behavior

### What the mock does

1. **Accept and store:** Accept valid messages, assign a message ID (`<timestamp.random@domain>`), store all fields for inspection
2. **Generate events:** On successful accept, create an `accepted` event. Optionally auto-generate `delivered` event after a brief delay (configurable)
3. **Return standard response:** Always return `{"id": "<...>", "message": "Queued. Thank you."}` on success
4. **Store for retrieval:** Generate a `storage_key` and make the message retrievable via GET endpoint
5. **Validate inputs:** Enforce the validation rules above, return appropriate 400 errors
6. **Support test mode:** When `o:testmode=yes`, process and validate but mark as test (still stored, still generates events)
7. **Resend support:** Allow re-sending stored messages to new recipients
8. **Queue status:** Return static queue status (always enabled by default)

### What the mock skips

- Actual email delivery (SMTP/MX resolution)
- DKIM signing / TLS enforcement
- IP pool routing
- Send Time Optimization / Timezone Optimization (accept params, ignore them)
- Rate limiting (accept the params, don't enforce real limits)
- `o:archive-to` HTTP POST delivery
- MIME parsing for the `.mime` endpoint (store the raw MIME as-is)
- Tracking pixel injection
- `stripped-html`/`stripped-text`/`stripped-signature` extraction (return same as body)

### Storage model

Each stored message should capture:

```
{
  id: string,              // RFC-2392 message ID
  storage_key: string,     // key for retrieval
  domain: string,          // sending domain
  from: string,            // sender
  to: string[],            // recipients
  cc: string[],            // CC recipients
  bcc: string[],           // BCC recipients
  subject: string,         // subject
  text: string | null,     // plain text body
  html: string | null,     // HTML body
  amp_html: string | null, // AMP body
  template: string | null, // template name
  template_version: string | null,
  template_variables: object | null,
  tags: string[],          // o:tag values
  headers: object,         // h: custom headers
  variables: object,       // v: custom variables
  recipient_variables: object | null,
  options: {               // o: options
    testmode: boolean,
    tracking: string | null,
    tracking_clicks: string | null,
    tracking_opens: string | null,
    deliverytime: string | null,
    require_tls: boolean,
    dkim: string | null,
    ...
  },
  attachments: [{          // file metadata (no need to store actual bytes in most cases)
    filename: string,
    content_type: string,
    size: number,
  }],
  inline: [{               // inline attachments
    filename: string,
    content_type: string,
    size: number,
  }],
  mime: string | null,     // raw MIME body (for .mime endpoint)
  created_at: string,      // ISO-8601 timestamp
}
```

## SDK Compatibility Notes

Client libraries send messages like this — the mock must handle these patterns:

### Node.js (mailgun.js)
```javascript
mg.messages.create('domain.com', {
  from: "Name <email>",
  to: ["recipient@example.com"],
  subject: "Hello",
  text: "body",
  html: "<h1>body</h1>",
  attachment: [{ filename: 'doc.pdf', data: buffer }],
  'o:tag': ['tag1', 'tag2'],
  'h:Reply-To': 'reply@example.com',
  'v:user-id': '123',
  'recipient-variables': JSON.stringify({ 'bob@example.com': { name: 'Bob' } })
});
```

The Node SDK converts booleans to `"yes"`/`"no"` strings and auto-selects the `.mime` endpoint if a `message` field is present.

### Python (mailgun-python)
```python
client.messages.create(
    domain="domain.com",
    data={
        "from": "sender@domain.com",
        "to": "recipient@example.com",
        "subject": "Hello",
        "text": "body",
        "o:tag": "my-tag"
    },
    files=[("attachment", ("file.txt", open("file.txt","rb").read()))]
)
```

## Test Scenarios

1. **Basic send:** POST with from/to/subject/text → 200 with message ID
2. **HTML send:** POST with html body → stored with html content
3. **Missing required fields:** Omit `from` → 400, omit `to` → 400, omit body params → 400
4. **Template send:** POST with `template` name → 200 (template resolution handled separately)
5. **Attachments:** POST with file attachments → stored with attachment metadata
6. **Inline images:** POST with inline attachments → stored
7. **Tags:** POST with `o:tag` values → stored, accessible in events
8. **Custom headers/variables:** POST with `h:` and `v:` prefixed fields → stored
9. **Batch send:** POST with `recipient-variables` → stored, each recipient gets separate event
10. **Test mode:** POST with `o:testmode=yes` → 200, stored but marked as test
11. **Scheduled send:** POST with `o:deliverytime` → accepted, stored with schedule time
12. **Retrieve message:** GET with storage key → full message data
13. **Resend message:** POST resend to new recipient → new message ID
14. **Invalid domain:** POST to non-existent domain → 404
15. **MIME send:** POST to `.mime` endpoint with raw MIME → stored
16. **Queue status:** GET sending_queues → queue info
17. **Duplicate recipients:** POST with duplicate emails → auto-deduped
18. **Too many recipients:** POST with > 1,000 recipients → 400
19. **Auth failure:** POST without valid auth → 401

## References

- **OpenAPI spec:** `mailgun.yaml` lines 508-829 (endpoints), 14108-14900 (schemas)
- **API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/messages
- **Batch sending guide:** https://documentation.mailgun.com/docs/mailgun/user-manual/sending-messages/batch-sending
- **Node SDK:** https://github.com/mailgun/mailgun.js
- **Python SDK:** https://github.com/mailgun/mailgun-python
- **Go SDK (validation logic):** https://github.com/mailgun/mailgun-go/blob/master/messages.go
