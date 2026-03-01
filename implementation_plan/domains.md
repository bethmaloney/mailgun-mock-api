# Domains

Domain CRUD, auto-verify or controllable verification status, DNS records, tracking settings, and SMTP credentials. Domains are the central organizing entity in Mailgun — messages are sent from domains, events belong to domains, and most resources are scoped to a domain.

## Endpoints

### 1. GET `/v4/domains` — List all domains

Returns a paginated list of domains for the account.

**Auth:** HTTP Basic (`api:<key>`)

#### Query Parameters

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `limit` | integer | 100 | Max items per page (1–1000) |
| `skip` | integer | 0 | Number of items to skip |
| `state` | string | — | Filter by state: `active`, `unverified`, `disabled` |
| `sort` | string | — | Sort order (e.g., `name`, `-name` for desc) |
| `authority` | string | — | Filter by DKIM authority domain |
| `search` | string | — | Search by domain name substring |
| `include_subaccounts` | boolean | false | Include subaccount domains |

#### Success Response (200)

```json
{
  "total_count": 2,
  "items": [
    {
      "id": "domain-id-123",
      "name": "example.com",
      "state": "active",
      "type": "custom",
      "created_at": "Mon, 02 Jan 2023 12:00:00 UTC",
      "smtp_login": "postmaster@example.com",
      "spam_action": "disabled",
      "wildcard": false,
      "require_tls": false,
      "skip_verification": false,
      "is_disabled": false,
      "web_prefix": "email",
      "web_scheme": "https",
      "use_automatic_sender_security": true,
      "message_ttl": 259200
    }
  ]
}
```

---

### 2. POST `/v4/domains` — Create a new domain

Registers a new sending domain. Returns the domain object and DNS records that need to be configured.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Domain name (e.g., `mg.example.com`) |
| `smtp_password` | string | no | Initial SMTP password for postmaster |
| `spam_action` | string | no | `disabled` (default), `tag`, or `block` |
| `wildcard` | boolean | no | Accept mail for subdomains |
| `force_dkim_authority` | boolean | no | Force DKIM authority to this domain |
| `dkim_key_size` | integer | no | DKIM key size: `1024` or `2048` (default) |
| `ips` | string | no | Comma-separated dedicated IPs to assign |
| `pool_id` | string | no | IP Pool ID to assign |
| `web_scheme` | string | no | `http` or `https` (for tracking URLs) |
| `web_prefix` | string | no | Tracking subdomain prefix (default: `email`) |
| `mailfrom_host` | string | no | Custom MAIL FROM host |
| `require_tls` | boolean | no | Require TLS for delivery |
| `skip_verification` | boolean | no | Skip TLS cert verification |
| `use_automatic_sender_security` | boolean | no | Auto-manage DKIM (default: `true`) |
| `message_ttl` | integer | no | Message retention in seconds (default: `259200` = 3 days) |

#### Success Response (200)

```json
{
  "message": "Domain has been created",
  "domain": {
    "id": "domain-id-123",
    "name": "mg.example.com",
    "state": "unverified",
    "type": "custom",
    "created_at": "Mon, 02 Jan 2023 12:00:00 UTC",
    "smtp_login": "postmaster@mg.example.com",
    "smtp_password": "the-password",
    "spam_action": "disabled",
    "wildcard": false,
    "require_tls": false,
    "skip_verification": false,
    "is_disabled": false,
    "web_prefix": "email",
    "web_scheme": "https",
    "use_automatic_sender_security": true,
    "message_ttl": 259200
  },
  "receiving_dns_records": [
    {
      "record_type": "MX",
      "priority": "10",
      "valid": "unknown",
      "is_active": false,
      "name": "mg.example.com",
      "value": "mxa.mailgun.org",
      "cached": []
    },
    {
      "record_type": "MX",
      "priority": "10",
      "valid": "unknown",
      "is_active": false,
      "name": "mg.example.com",
      "value": "mxb.mailgun.org",
      "cached": []
    }
  ],
  "sending_dns_records": [
    {
      "record_type": "TXT",
      "valid": "unknown",
      "is_active": false,
      "name": "mg.example.com",
      "value": "v=spf1 include:mailgun.org ~all",
      "cached": []
    },
    {
      "record_type": "TXT",
      "valid": "unknown",
      "is_active": false,
      "name": "pic._domainkey.mg.example.com",
      "value": "k=rsa; p=MIGfMA0GCS...",
      "cached": []
    },
    {
      "record_type": "CNAME",
      "valid": "unknown",
      "is_active": false,
      "name": "email.mg.example.com",
      "value": "mailgun.org",
      "cached": []
    }
  ]
}
```

#### Error Responses

| Status | Body | When |
|--------|------|------|
| 400 | `{"message": "..."}` | Invalid domain name, duplicate domain |
| 401 | `"Forbidden"` | Invalid/missing API key |

---

### 3. GET `/v4/domains/{name}` — Get domain details

Returns a single domain with its DNS record status.

**Auth:** HTTP Basic (`api:<key>`)

#### Path Parameters

| Name | Type | Description |
|------|------|-------------|
| `name` | string | Domain name |

#### Success Response (200)

```json
{
  "domain": { /* same shape as in create response */ },
  "receiving_dns_records": [ /* Record[] */ ],
  "sending_dns_records": [ /* Record[] */ ]
}
```

#### Error Responses

| Status | Body |
|--------|------|
| 404 | `{"message": "Domain not found"}` |

---

### 4. PUT `/v4/domains/{name}` — Update domain settings

Update mutable domain configuration.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Description |
|-------|------|-------------|
| `spam_action` | string | `disabled`, `tag`, or `block` |
| `wildcard` | boolean | Accept mail for subdomains |
| `web_scheme` | string | `http` or `https` |
| `web_prefix` | string | Tracking subdomain prefix |
| `mailfrom_host` | string | Custom MAIL FROM host |
| `require_tls` | boolean | Require TLS |
| `skip_verification` | boolean | Skip TLS cert verification |
| `smtp_password` | string | Update SMTP password |
| `use_automatic_sender_security` | boolean | Auto-manage DKIM |
| `message_ttl` | integer | Message retention in seconds |
| `archive_to` | string | URL for message archival |

#### Success Response (200)

```json
{
  "message": "Domain has been updated",
  "domain": { /* updated domain object */ },
  "receiving_dns_records": [ /* Record[] */ ],
  "sending_dns_records": [ /* Record[] */ ]
}
```

---

### 5. DELETE `/v3/domains/{name}` — Delete a domain

Permanently removes a domain. Note: this endpoint is v3, not v4.

**Auth:** HTTP Basic (`api:<key>`)

#### Success Response (200)

```json
{
  "message": "Domain has been deleted"
}
```

#### Error Responses

| Status | Body |
|--------|------|
| 404 | `{"message": "Domain not found"}` |

---

### 6. PUT `/v4/domains/{name}/verify` — Verify domain DNS

Triggers a DNS verification check. In production, Mailgun queries DNS to verify SPF, DKIM, MX, and CNAME records. Returns updated domain and record status.

**Auth:** HTTP Basic (`api:<key>`)

#### Success Response (200)

```json
{
  "message": "Domain DNS records have been updated",
  "domain": {
    "name": "mg.example.com",
    "state": "active",
    /* ... rest of domain fields */
  },
  "receiving_dns_records": [
    {
      "record_type": "MX",
      "priority": "10",
      "valid": "valid",
      "is_active": true,
      "name": "mg.example.com",
      "value": "mxa.mailgun.org",
      "cached": ["mxa.mailgun.org"]
    }
  ],
  "sending_dns_records": [
    {
      "record_type": "TXT",
      "valid": "valid",
      "is_active": true,
      "name": "mg.example.com",
      "value": "v=spf1 include:mailgun.org ~all",
      "cached": ["v=spf1 include:mailgun.org ~all"]
    }
  ]
}
```

---

### 7. GET `/v3/domains/{name}/tracking` — Get tracking settings

Returns open, click, and unsubscribe tracking configuration for a domain.

**Auth:** HTTP Basic (`api:<key>`)

#### Success Response (200)

```json
{
  "tracking": {
    "open": {
      "active": true,
      "place_at_the_top": false
    },
    "click": {
      "active": true
    },
    "unsubscribe": {
      "active": true,
      "html_footer": "\n<br>\n<p><a href=\"%unsubscribe_url%\">unsubscribe</a></p>\n",
      "text_footer": "\n\nTo unsubscribe click: <%unsubscribe_url%>\n\n"
    }
  }
}
```

---

### 8. PUT `/v3/domains/{name}/tracking/open` — Update open tracking

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Description |
|-------|------|-------------|
| `active` | boolean | Enable/disable open tracking |
| `place_at_the_top` | boolean | Place tracking pixel at top of email body |

#### Success Response (200)

```json
{
  "message": "Domain tracking settings have been updated",
  "open": {
    "active": true,
    "place_at_the_top": false
  }
}
```

---

### 9. PUT `/v3/domains/{name}/tracking/click` — Update click tracking

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Description |
|-------|------|-------------|
| `active` | string | `true`, `false`, or `htmlonly` |

#### Success Response (200)

```json
{
  "message": "Domain tracking settings have been updated",
  "click": {
    "active": true
  }
}
```

---

### 10. PUT `/v3/domains/{name}/tracking/unsubscribe` — Update unsubscribe tracking

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Description |
|-------|------|-------------|
| `active` | boolean | Enable/disable unsubscribe tracking |
| `html_footer` | string | HTML unsubscribe footer template |
| `text_footer` | string | Plain text unsubscribe footer template |

#### Success Response (200)

```json
{
  "message": "Domain tracking settings have been updated",
  "unsubscribe": {
    "active": true,
    "html_footer": "<a href=\"%unsubscribe_url%\">unsubscribe</a>",
    "text_footer": "To unsubscribe: <%unsubscribe_url%>"
  }
}
```

---

### 11. GET `/v3/domains/{domain_name}/credentials` — List SMTP credentials

Returns paginated SMTP credentials for a domain.

**Auth:** HTTP Basic (`api:<key>`)

#### Query Parameters

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `limit` | integer | 100 | Max items per page |
| `skip` | integer | 0 | Items to skip |

#### Success Response (200)

```json
{
  "total_count": 1,
  "items": [
    {
      "login": "postmaster@mg.example.com",
      "mailbox": "postmaster@mg.example.com",
      "size_bytes": null,
      "created_at": "Mon, 02 Jan 2023 12:00:00 UTC"
    }
  ]
}
```

---

### 12. POST `/v3/domains/{domain_name}/credentials` — Create SMTP credential

Creates a new SMTP credential for the domain.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `login` | string | yes | Username (becomes `login@domain`) |
| `password` | string | yes | SMTP password (min 5, max 32 chars) |

#### Success Response (200)

```json
{
  "message": "'postmaster@mg.example.com' has been created"
}
```

---

### 13. PUT `/v3/domains/{domain_name}/credentials/{login}` — Update credential password

Updates the password for an existing SMTP credential.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `password` | string | yes | New SMTP password (min 5, max 32 chars) |

#### Success Response (200)

```json
{
  "message": "Password has been updated"
}
```

---

### 14. DELETE `/v3/domains/{domain_name}/credentials/{login}` — Delete credential

Deletes a specific SMTP credential.

**Auth:** HTTP Basic (`api:<key>`)

#### Success Response (200)

```json
{
  "message": "Credentials have been deleted",
  "spec": "postmaster@mg.example.com"
}
```

---

### 15. PUT `/v3/domains/{name}/dkim_authority` — Update DKIM authority

Changes DKIM signing authority between the subdomain and root domain.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `self` | boolean | yes | `true` = this domain signs its own DKIM; `false` = delegate to root |

#### Success Response (200)

```json
{
  "message": "Domain DKIM authority has been changed",
  "changed": true,
  "sending_dns_records": [ /* Record[] */ ]
}
```

---

### 16. PUT `/v3/domains/{name}/dkim_selector` — Update DKIM selector

Changes the DKIM selector prefix used in DNS records.

**Auth:** HTTP Basic (`api:<key>`)

#### Request Body (`multipart/form-data`)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dkim_selector` | string | yes | New DKIM selector prefix (e.g., `s1`, `mx`) |

#### Success Response (200)

```json
{
  "message": "Domain DKIM selector has been updated"
}
```

---

## Schemas

### Domain Object

```
{
  id: string,                           // Unique ID
  name: string,                         // Domain FQDN
  state: "active" | "unverified" | "disabled",
  type: "custom" | "sandbox",          // sandbox = Mailgun test domain
  created_at: string,                   // RFC-1123 date (e.g., "Mon, 02 Jan 2023 12:00:00 UTC")
  smtp_login: string,                   // Default SMTP login (postmaster@domain)
  smtp_password: string | null,         // SMTP password (only returned on create)
  spam_action: "disabled" | "tag" | "block",
  wildcard: boolean,                    // Accept mail for subdomains
  require_tls: boolean,                 // Require TLS for delivery
  skip_verification: boolean,           // Skip TLS cert verification
  is_disabled: boolean,                 // Whether domain is disabled
  web_prefix: string,                   // Tracking subdomain (default: "email")
  web_scheme: "http" | "https",         // Tracking URL scheme
  use_automatic_sender_security: boolean, // Auto DKIM management
  message_ttl: integer,                 // Message retention (seconds)
  tracking_host: string | null,         // Custom tracking host
  archive_to: string | null,            // Archival URL
  subaccount_id: string | null,         // Associated subaccount
  disabled: {                           // Disabled details (nullable)
    code: string,
    note: string,
    permanently: boolean,
    reason: string,
    until: string                       // RFC-1123 date
  } | null
}
```

### DNS Record Object

```
{
  record_type: "MX" | "TXT" | "CNAME" | "A",
  name: string,                         // DNS record name
  value: string,                        // Expected record value
  priority: string | null,              // MX priority (e.g., "10")
  valid: "valid" | "invalid" | "unknown",
  is_active: boolean,                   // Whether record is active/verified
  cached: string[]                      // What Mailgun found in DNS
}
```

### Tracking Settings Object

```
{
  open: {
    active: boolean,
    place_at_the_top: boolean
  },
  click: {
    active: boolean                     // Also accepts "htmlonly" on write
  },
  unsubscribe: {
    active: boolean,
    html_footer: string,                // HTML template with %unsubscribe_url%
    text_footer: string                 // Text template with <%unsubscribe_url%>
  }
}
```

### SMTP Credential Object

```
{
  login: string,                        // Full email (user@domain)
  mailbox: string,                      // Same as login
  size_bytes: integer | null,           // Mailbox size (always null for mock)
  created_at: string                    // RFC-1123 date
}
```

## Mock Behavior

### What the mock does

1. **Domain CRUD:** Full create, read, update, delete lifecycle. Domains are stored in memory and scoped by API key (or global if auth is disabled).
2. **Auto-verify mode (default):** When a domain is created, the mock immediately sets `state: "active"` and marks all DNS records as `valid: "valid"`, `is_active: true`. This is the default for frictionless dev/test.
3. **Manual verification mode (configurable):** Optionally, domains start as `state: "unverified"` with DNS records `valid: "unknown"`. Calling PUT `/verify` transitions them to `active`. This mode is useful for testing verification flows.
4. **DNS records:** On domain creation, the mock generates realistic DNS records (SPF TXT, DKIM TXT, MX, tracking CNAME) with mock values. These records are static — the mock doesn't actually query DNS.
5. **Tracking settings:** Full CRUD for open, click, and unsubscribe tracking configuration. Stored per-domain. Default to enabled on domain creation.
6. **SMTP credentials:** Full CRUD for credentials. Stored per-domain. A default `postmaster@{domain}` credential is created with the domain.
7. **DKIM management:** Accept DKIM authority and selector update calls. Store the values but don't perform actual key operations. Return realistic responses.
8. **Sandbox domains:** Support a single pre-seeded sandbox domain (`sandbox*.mailgun.org`) for testing without domain setup.
9. **Pagination:** Support `limit` and `skip` on list endpoints. Return `total_count` for proper pagination.
10. **Search/filter:** Support `state` filter and `search` substring matching on list endpoint.

### What the mock skips

- Real DNS lookups and verification
- DKIM key generation (return static mock keys)
- DKIM key rotation (accept calls, return success)
- IP assignment and pool management (accept params, ignore)
- Dynamic IP pool enrollment
- Sending queue management (covered in messages.md)
- Domain disable/enable enforcement (accept state changes, but don't enforce restrictions)
- `mailfrom_host` DNS configuration
- Actual SMTP server for credentials (credentials are stored but not functional for SMTP)

### Storage model

Each domain should capture:

```
{
  id: string,                    // Generated UUID
  name: string,                  // Domain FQDN
  state: string,                 // "active" | "unverified" | "disabled"
  type: string,                  // "custom" | "sandbox"
  created_at: string,            // RFC-1123 timestamp
  smtp_login: string,            // postmaster@{domain}
  smtp_password: string | null,
  spam_action: string,           // "disabled" | "tag" | "block"
  wildcard: boolean,
  require_tls: boolean,
  skip_verification: boolean,
  is_disabled: boolean,
  web_prefix: string,
  web_scheme: string,
  use_automatic_sender_security: boolean,
  message_ttl: integer,
  tracking_host: string | null,
  archive_to: string | null,
  dkim_selector: string,         // default "pic" or custom
  dkim_authority_self: boolean,   // whether domain is its own DKIM authority
  tracking: {                    // tracking settings
    open: { active: boolean, place_at_the_top: boolean },
    click: { active: boolean },
    unsubscribe: { active: boolean, html_footer: string, text_footer: string }
  },
  sending_dns_records: Record[],
  receiving_dns_records: Record[],
  credentials: Credential[],     // SMTP credentials
}
```

### Default DNS records template

On domain creation, generate these records (substituting `{domain}` and `{selector}`):

**Sending records:**
1. SPF: `TXT` record on `{domain}` → `v=spf1 include:mailgun.org ~all`
2. DKIM: `TXT` record on `{selector}._domainkey.{domain}` → `k=rsa; p=<mock-public-key>`
3. Tracking CNAME: `CNAME` record on `{web_prefix}.{domain}` → `mailgun.org`

**Receiving records:**
1. MX: `MX` record on `{domain}` → `mxa.mailgun.org` (priority 10)
2. MX: `MX` record on `{domain}` → `mxb.mailgun.org` (priority 10)

## SDK Compatibility Notes

### Node.js (mailgun.js)
```javascript
// Create domain
mg.domains.create({ name: 'mg.example.com', smtp_password: 'secret' });

// List domains
mg.domains.list();

// Get domain
mg.domains.get('mg.example.com');

// Update domain
mg.domains.update('mg.example.com', { web_scheme: 'https' });

// Delete domain
mg.domains.destroy('mg.example.com');

// Verify domain
mg.domains.verify('mg.example.com');

// Tracking settings
mg.domains.getTracking('mg.example.com');
mg.domains.updateTracking('mg.example.com', 'open', { active: true });

// SMTP credentials
mg.domains.getCredentials('mg.example.com');
mg.domains.createCredential('mg.example.com', { login: 'alice', password: 'pass' });
mg.domains.deleteCredential('mg.example.com', 'alice@mg.example.com');
```

### Python (mailgun-python)
```python
# Create domain
client.domains.create(data={"name": "mg.example.com"})

# List domains
client.domains.list()

# Get domain
client.domains.get("mg.example.com")

# Verify domain
client.domains.verify("mg.example.com")
```

## Test Scenarios

1. **Create domain:** POST with `name` → 200, returns domain + DNS records
2. **Create duplicate:** POST with existing domain name → 400
3. **List domains:** GET → 200, returns paginated list with `total_count`
4. **List with filters:** GET with `state=active` → only active domains; `search=example` → matching domains
5. **Get domain:** GET by name → 200, includes domain + DNS records
6. **Get nonexistent:** GET unknown domain → 404
7. **Update domain:** PUT with changed fields → 200, fields updated
8. **Delete domain:** DELETE → 200, domain removed from list
9. **Delete nonexistent:** DELETE unknown domain → 404
10. **Verify domain (auto mode):** PUT verify → domain already active, records valid
11. **Verify domain (manual mode):** Create → state=unverified; PUT verify → state=active
12. **Get tracking:** GET tracking → 200, returns open/click/unsubscribe settings
13. **Update tracking:** PUT tracking/open → 200, setting changed
14. **Unsubscribe footer:** PUT tracking/unsubscribe with custom footers → stored and returned
15. **List credentials:** GET credentials → 200, includes default postmaster credential
16. **Create credential:** POST with login/password → 200, credential created
17. **Update credential password:** PUT with new password → 200
18. **Delete credential:** DELETE → 200, credential removed
19. **DKIM authority:** PUT dkim_authority → 200, sending records updated
20. **DKIM selector:** PUT dkim_selector → 200, DKIM record name updated
21. **Pagination:** List domains with limit=1 → returns 1 item, total_count shows total
22. **Auth failure:** Any endpoint without valid auth → 401

## References

- **OpenAPI spec:** `mailgun.yaml` — domain endpoints under `/v3/domains` and `/v4/domains` paths; schemas under `github.com-mailgun-domains-client-golang-Domain`, `github.com-mailgun-domains-client-golang-Record`
- **API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/domains
- **Credentials docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/credentials
- **Node SDK:** https://github.com/mailgun/mailgun.js
- **Python SDK:** https://github.com/mailgun/mailgun-python
- **Help center (API keys/credentials):** https://help.mailgun.com/hc/en-us/articles/203380100
