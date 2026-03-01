# Templates

Template storage, versioning, and rendering for the mock Mailgun service.

## Overview

Mailgun's Templates API lets users store reusable email templates server-side, manage versions, and reference them by name when sending messages. The mock needs to support the full CRUD lifecycle for templates and versions, plus template resolution during message sending.

**Key concepts:**
- Templates are scoped to a domain
- Each template can have up to **40 versions**, identified by a string `tag`
- Exactly one version is **active** at a time — sending uses the active version unless `t:version` overrides it
- Template content uses **Handlebars** (default) or **Go `text/template`** engine
- Templates can store headers (`From`, `Subject`, `Reply-To`) that get injected at send time
- Max **100 templates per domain**, max **100 KB per template content**

## API Endpoints

### Template CRUD

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v3/{domain}/templates` | Create a template (optionally with initial version) |
| `GET` | `/v3/{domain}/templates` | List all templates (paginated) |
| `GET` | `/v3/{domain}/templates/{name}` | Get a single template |
| `PUT` | `/v3/{domain}/templates/{name}` | Update template description |
| `DELETE` | `/v3/{domain}/templates/{name}` | Delete a template and all its versions |
| `DELETE` | `/v3/{domain}/templates` | Delete all templates for the domain |

### Version CRUD

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v3/{domain}/templates/{name}/versions` | Create a new version |
| `GET` | `/v3/{domain}/templates/{name}/versions` | List all versions (paginated) |
| `GET` | `/v3/{domain}/templates/{name}/versions/{tag}` | Get a specific version |
| `PUT` | `/v3/{domain}/templates/{name}/versions/{tag}` | Update a version |
| `DELETE` | `/v3/{domain}/templates/{name}/versions/{tag}` | Delete a version |
| `PUT` | `/v3/{domain}/templates/{name}/versions/{tag}/copy/{new_tag}` | Copy a version to a new tag |

---

## Data Models

### Template

```typescript
{
  name: string,          // template name (lowercased, utf-8 supported)
  description: string,   // human-readable description
  createdAt: string,     // RFC 2822 date, e.g. "Wed, 29 Aug 2018 23:31:13 UTC"
  createdBy: string,     // optional user-supplied metadata (who created it)
  id: string,            // UUID, e.g. "46565d87-68b6-4edb-8b3c-34554af4bb77"
  version?: Version,     // active version (included when ?active=yes)
  versions?: Version[],  // list of versions (only in list-versions response)
}
```

### Version

```typescript
{
  tag: string,           // version identifier, e.g. "v1", "initial" (lowercased by API)
  template: string,      // the template content (HTML/Handlebars/Go markup)
  engine: string,        // "handlebars" (default) or "go"
  mjml: string,          // MJML source (empty string if not used)
  createdAt: string,     // RFC 2822 date
  comment: string,       // version description/changelog
  active: boolean,       // whether this is the active version
  id: string,            // UUID
  headers?: {            // optional stored headers
    From?: string,
    Subject?: string,
    "Reply-To"?: string
  }
}
```

**Notes:**
- Version tags are **lowercased** by the API (sending `"V1"` stores as `"v1"`)
- The `mjml` field appears in all version responses (empty string when not using MJML)
- The `template` content field is **omitted** in list-versions responses (only metadata returned); use the get-single-version endpoint to retrieve content
- The `headers` field is only present when headers have been set

---

## Endpoint Details

### POST `/v3/{domain}/templates` — Create Template

**Request** (`multipart/form-data` or `application/x-www-form-urlencoded`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Template name. Supports utf-8, will be lowercased. |
| `description` | string | no | Template description |
| `createdBy` | string | no | Who created the template (metadata) |
| `template` | string | no | Template content. If provided, creates the initial version automatically. |
| `tag` | string | no | Version tag for the initial version. Defaults to `"initial"` if `template` is provided and `tag` is omitted. |
| `comment` | string | no | Comment for the initial version (only valid when `template` is provided) |
| `engine` | string | no | `"handlebars"` (default) or `"go"` |
| `headers` | string | no | JSON-encoded object with `From`, `Subject`, and/or `Reply-To` headers |

**Response** (200):
```json
{
  "message": "template has been stored",
  "template": {
    "name": "test_template",
    "description": "A test template",
    "createdAt": "Wed, 29 Aug 2018 23:31:13 UTC",
    "createdBy": "",
    "id": "46565d87-68b6-4edb-8b3c-34554af4bb77",
    "version": {
      "tag": "v0",
      "template": "<div class=\"entry\"><h1>{{title}}</h1><div class=\"body\">{{body}}</div></div>",
      "engine": "handlebars",
      "mjml": "",
      "createdAt": "Wed, 29 Aug 2018 23:31:13 UTC",
      "comment": "",
      "active": true,
      "id": "someId2"
    }
  }
}
```

If no `template` content was provided, the `version` field is `null` (template created without any versions).

### GET `/v3/{domain}/templates` — List Templates

**Query parameters:**

| Field | Type | Description |
|-------|------|-------------|
| `page` | string | `"first"`, `"last"`, `"next"`, `"previous"`. Defaults to `"first"`. |
| `limit` | integer | Number of templates to return. Default and max: 100. |
| `p` | string | Pivot/cursor for pagination (template name). |

**Response** (200):
```json
{
  "items": [
    {
      "name": "test_template",
      "description": "A test template",
      "createdAt": "Wed, 29 Aug 2018 23:31:13 UTC",
      "createdBy": "",
      "id": "46565d87-..."
    }
  ],
  "paging": {
    "first": "https://api.mailgun.net/v3/{domain}/templates?page=first&limit=100",
    "last": "https://api.mailgun.net/v3/{domain}/templates?page=last&limit=100",
    "next": "https://api.mailgun.net/v3/{domain}/templates?page=next&p=test_template&limit=100",
    "previous": "https://api.mailgun.net/v3/{domain}/templates?page=previous&p=test_template&limit=100"
  }
}
```

**Note:** The `version` field in each item is **not** populated by default. The `?active=yes` query parameter can be used with the list endpoint to include the active version content for each template, though this is undocumented in the OpenAPI spec — observed in client library tests.

### GET `/v3/{domain}/templates/{name}` — Get Template

**Query parameters:**

| Field | Type | Description |
|-------|------|-------------|
| `active` | string | If `"yes"`, includes the active version's content in the response |

**Response** (200) without `?active=yes`:
```json
{
  "template": {
    "name": "test_template",
    "description": "A test template",
    "createdAt": "Mon, 20 Dec 2021 14:47:51 UTC",
    "createdBy": "",
    "id": "someId"
  }
}
```

**Response** (200) with `?active=yes`:
```json
{
  "template": {
    "name": "test_template",
    "description": "A test template",
    "createdAt": "Mon, 20 Dec 2021 14:47:51 UTC",
    "createdBy": "",
    "id": "someId",
    "version": {
      "tag": "v0",
      "template": "{{fname}} {{lname}}",
      "engine": "handlebars",
      "mjml": "",
      "createdAt": "Mon, 20 Dec 2021 14:47:51 UTC",
      "comment": "",
      "active": true,
      "id": "versionId",
      "headers": { "Subject": "{{subject}}" }
    }
  }
}
```

### PUT `/v3/{domain}/templates/{name}` — Update Template

Updates **only the description**. Does not modify version content.

**Request** (`multipart/form-data`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | yes | Updated description |

**Response** (200):
```json
{
  "message": "template has been updated",
  "template": {
    "name": "test_template"
  }
}
```

### DELETE `/v3/{domain}/templates/{name}` — Delete Template

Deletes the template and **all** its versions.

**Response** (200):
```json
{
  "message": "template has been deleted",
  "template": {
    "name": "test_template"
  }
}
```

### DELETE `/v3/{domain}/templates` — Delete All Templates

Deletes all templates and all their versions for the domain.

**Response** (200):
```json
{
  "message": "templates have been deleted"
}
```

---

### POST `/v3/{domain}/templates/{name}/versions` — Create Version

**Request** (`multipart/form-data`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `template` | string | yes | Template content |
| `tag` | string | yes | Version tag (must be unique per template, will be lowercased) |
| `comment` | string | no | Version comment |
| `active` | string | no | `"yes"` to make this the active version |
| `engine` | string | no | `"handlebars"` (default) or `"go"` |
| `headers` | string | no | JSON-encoded headers object |

**Behavior:**
- If the template has no existing versions, the first version automatically becomes active regardless of the `active` flag
- Setting `active=yes` deactivates the previously active version
- Max 40 versions per template

**Response** (200):
```json
{
  "message": "new version of the template has been stored",
  "template": {
    "name": "test_template",
    "version": {
      "tag": "v1",
      "template": "...",
      "engine": "handlebars",
      "mjml": "",
      "createdAt": "Wed, 29 Aug 2018 23:45:00 UTC",
      "comment": "Updated layout",
      "active": true,
      "id": "versionId"
    }
  }
}
```

### GET `/v3/{domain}/templates/{name}/versions` — List Versions

**Query parameters:**

| Field | Type | Description |
|-------|------|-------------|
| `page` | string | `"first"`, `"last"`, `"next"`, `"previous"`. Defaults to `"first"`. |
| `limit` | integer | Number of versions to return. Default and max: 100. |
| `p` | string | Pivot/cursor for pagination (version tag). |

**Response** (200):
```json
{
  "template": {
    "name": "test_template",
    "description": "A test template",
    "createdAt": "Wed, 22 Dec 2021 09:13:27 UTC",
    "createdBy": "",
    "id": "someId",
    "versions": [
      {
        "tag": "v2",
        "engine": "handlebars",
        "mjml": "",
        "createdAt": "Wed, 22 Dec 2021 09:15:00 UTC",
        "comment": "updated layout",
        "active": true,
        "id": "someId1"
      },
      {
        "tag": "v5",
        "engine": "handlebars",
        "mjml": "",
        "createdAt": "Wed, 22 Dec 2021 09:20:00 UTC",
        "comment": "experimental",
        "active": false,
        "id": "someId2"
      }
    ]
  },
  "paging": {
    "first": "https://api.mailgun.net/v3/{domain}/templates/{name}/versions?limit=100",
    "last": "https://api.mailgun.net/v3/{domain}/templates/{name}/versions?page=last&limit=100",
    "next": "https://api.mailgun.net/v3/{domain}/templates/{name}/versions?page=next&p=v5&limit=100",
    "previous": "https://api.mailgun.net/v3/{domain}/templates/{name}/versions?page=previous&p=v2&limit=100"
  }
}
```

**Note:** The `versions` array contains metadata only (no `template` content field). Use the get-single-version endpoint to retrieve the content.

### GET `/v3/{domain}/templates/{name}/versions/{tag}` — Get Version

**Response** (200):
```json
{
  "template": {
    "name": "test_template",
    "version": {
      "tag": "v0",
      "template": "<div class=\"entry\"><h1>{{title}}</h1><div class=\"body\">{{body}}</div></div>",
      "engine": "handlebars",
      "mjml": "",
      "createdAt": "Wed, 29 Aug 2018 23:31:13 UTC",
      "comment": "Initial version",
      "active": true,
      "id": "versionId",
      "headers": { "Subject": "Welcome {{name}}!" }
    }
  }
}
```

### PUT `/v3/{domain}/templates/{name}/versions/{tag}` — Update Version

**Request** (`multipart/form-data`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `template` | string | no | Updated template content |
| `comment` | string | no | Updated comment |
| `active` | string | no | `"yes"` to make this the active version |
| `headers` | string | no | Updated JSON-encoded headers |

**Response** (200):
```json
{
  "message": "version has been updated",
  "template": {
    "name": "test_template",
    "version": {
      "tag": "v0"
    }
  }
}
```

### DELETE `/v3/{domain}/templates/{name}/versions/{tag}` — Delete Version

**Response** (200):
```json
{
  "message": "version has been deleted",
  "template": {
    "name": "test_template",
    "version": {
      "tag": "v0"
    }
  }
}
```

### PUT `/v3/{domain}/templates/{name}/versions/{tag}/copy/{new_tag}` — Copy Version

Copies an existing version's content and headers to a new version tag.

**Query parameters:**

| Field | Type | Description |
|-------|------|-------------|
| `comment` | string | Optional comment for the new version |

**Behavior:**
- If `new_tag` already exists, the existing version is **overwritten**
- The new version inherits the source version's content, engine, and headers

**Response** (200):
```json
{
  "message": "version has been copied",
  "version": {
    "tag": "v3",
    "template": "...",
    "engine": "handlebars",
    "mjml": "",
    "createdAt": "...",
    "comment": "Copied from v2",
    "active": false,
    "id": "newVersionId"
  },
  "template": {
    "tag": "v3"
  }
}
```

**Note:** The response has both `version` (new) and `template` (deprecated alias) fields. The `template` field is deprecated — use `version`.

---

## Template Rendering (Integration with Message Sending)

When a message is sent with `template` field set, the mock must:

1. **Resolve the template** by name within the sending domain
2. **Select the version:**
   - If `t:version` is specified, use that version tag
   - Otherwise, use the active version
3. **Gather variables** from `t:variables` (JSON dict)
4. **Render the template** content with the specified engine and variables
5. **Apply stored headers** — `From`, `Subject`, `Reply-To` from the version's `headers` field are injected into the message, but **message-level headers override template headers**
6. **Set HTML body** to the rendered output
7. If `t:text=yes`, generate a plain text version of the rendered HTML

### Template Engines

| Engine | Value | Description |
|--------|-------|-------------|
| Handlebars | `"handlebars"` | Default. Uses `{{variable}}` syntax. |
| Go | `"go"` | Go `text/template` engine. Uses `{{.Variable}}` syntax. |

### Handlebars Block Helpers (supported by Mailgun)

| Helper | Description |
|--------|-------------|
| `{{#if var}}...{{/if}}` | Conditional: renders block if `var` is truthy |
| `{{#unless var}}...{{/unless}}` | Inverse conditional: renders block if `var` is falsy |
| `{{#each array}}...{{/each}}` | Iteration: loops over array. `../` accesses parent context. |
| `{{#with obj}}...{{/with}}` | Context shift: changes evaluation context within block |
| `{{#equal var value}}...{{/equal}}` | Equality check: renders if `var` equals `value` (string comparison) |

### Mock Rendering Strategy

For the mock, template rendering should:
- **Support basic Handlebars variable substitution** (`{{var}}` → value) — this covers the majority of real-world usage
- **Optionally support block helpers** (`if`, `each`, `with`, `unless`, `equal`) — nice to have for realistic testing
- **Skip Go template engine** — very rarely used; accept `engine: "go"` but render with Handlebars or return unrendered content
- **Store the template name and version in the message record** for inspection

### Message-Level Template Fields

These fields on the message sending endpoint interact with templates (already documented in [messages.md](./messages.md)):

| Field | Description |
|-------|-------------|
| `template` | Name of the stored template |
| `t:version` | Specific version tag to use |
| `t:text` | `"yes"` to auto-generate plain text from rendered HTML |
| `t:variables` | JSON dict of template variables |

---

## Pagination

Templates and versions use the same cursor-based pagination as other Mailgun list endpoints:

```json
{
  "paging": {
    "first": "https://...?page=first&limit=100",
    "last": "https://...?page=last&limit=100",
    "next": "https://...?page=next&p={last_item_key}&limit=100",
    "previous": "https://...?page=previous&p={first_item_key}&limit=100"
  }
}
```

- **Pivot (`p`)**: Template name for template lists, version tag for version lists
- **Default limit**: 100 (both templates and versions per the OpenAPI spec)
- Uses the shared `PagingResponse` schema (same as suppressions, events, etc.)

---

## Error Cases

| Scenario | Status | Response |
|----------|--------|----------|
| Template not found | 404 | `{"message": "template not found"}` |
| Version not found | 404 | `{"message": "version not found"}` |
| Template name already exists | 400 | `{"message": "template with name 'X' already exists"}` |
| Version tag already exists | 400 | `{"message": "version with tag 'X' already exists"}` |
| Missing required field (`name`) | 400 | `{"message": "name is required"}` |
| Missing required field (`template`, `tag`) on version create | 400 | `{"message": "template is required"}` |
| Max versions exceeded (40) | 400 | `{"message": "maximum number of versions reached"}` |
| Max templates exceeded (100) | 400 | `{"message": "maximum number of templates reached"}` |
| Template too large (>100 KB) | 400 | `{"message": "template content too large"}` |
| Sending with nonexistent template | 400 | `{"message": "template 'X' not found"}` |
| Sending with nonexistent version | 400 | `{"message": "version 'X' not found for template 'Y'"}` |

---

## Mock Behavior Notes

### What the mock should do

1. **Full CRUD** for templates and versions — store in memory, return correct response shapes
2. **Template name lowercasing** — names are lowercased on storage
3. **Version tag lowercasing** — tags are lowercased (confirmed: sending `"V1"` stores as `"v1"`)
4. **Active version tracking** — exactly one active version at a time; setting a new active deactivates the old one
5. **First version auto-active** — the first version created for a template automatically becomes active
6. **Enforce limits** — 100 templates per domain, 40 versions per template
7. **Template resolution during sending** — when a message uses `template` field, resolve the template and version, store the rendered content (or at minimum store the template reference in the message record)
8. **Basic Handlebars rendering** — use a Handlebars library to render `{{variable}}` substitutions from `t:variables`
9. **Header injection from templates** — apply stored `From`, `Subject`, `Reply-To` headers, but let message-level headers take precedence
10. **Version copy** — duplicate a version's content/headers to a new tag name
11. **Delete all** — bulk delete endpoint for all templates in a domain

### What the mock can skip

- **MJML rendering** — accept and store the `mjml` field but don't compile MJML to HTML; return it as-is
- **Go template engine** — accept `engine: "go"` but render with Handlebars or return raw content
- **Template content size validation** — the 100 KB limit can be relaxed; enforce only if easy
- **Advanced Handlebars features** — partials, custom helpers beyond the documented set

---

## Internal Storage Schema

```typescript
interface StoredTemplate {
  id: string;                  // generated UUID
  domain: string;              // owning domain
  name: string;                // lowercased template name
  description: string;
  createdBy: string;
  createdAt: Date;
  versions: StoredVersion[];   // all versions
}

interface StoredVersion {
  id: string;                  // generated UUID
  tag: string;                 // lowercased version tag
  template: string;            // template content (HTML/Handlebars)
  engine: string;              // "handlebars" or "go"
  mjml: string;                // MJML source or ""
  comment: string;
  active: boolean;
  createdAt: Date;
  headers?: {
    From?: string;
    Subject?: string;
    "Reply-To"?: string;
  };
}
```

---

## Test Scenarios

1. **Create template with initial version** — POST with `name` + `template` + `tag` → template created with active version
2. **Create template without version** — POST with only `name` → template created, `version` is null
3. **Get template without active flag** — GET without `?active=yes` → no `version` field
4. **Get template with active flag** — GET with `?active=yes` → includes active version content
5. **Update template description** — PUT with `description` → only description changes
6. **Delete template** — DELETE → removes template and all versions
7. **Delete all templates** — DELETE on collection → removes everything for domain
8. **Create version** — POST to versions endpoint → new version stored
9. **First version auto-active** — create version on empty template → becomes active
10. **Set version active** — create/update with `active=yes` → old active deactivated
11. **List versions** — GET versions → paginated list with metadata (no content)
12. **Get specific version** — GET version by tag → includes template content
13. **Update version** — PUT with new content/comment/active → version updated
14. **Delete version** — DELETE by tag → version removed
15. **Copy version** — PUT copy endpoint → new version with copied content
16. **Copy overwrite** — copy to existing tag → overwrites target
17. **Tag lowercasing** — create with `"V1"` → stored/returned as `"v1"`
18. **Name lowercasing** — create with `"MyTemplate"` → stored/returned as `"mytemplate"`
19. **Send with template** — POST message with `template` field → template resolved, content rendered
20. **Send with template version** — POST message with `template` + `t:version` → specific version used
21. **Send with template variables** — POST message with `t:variables` → variables substituted
22. **Send with missing template** — POST message referencing nonexistent template → 400 error
23. **Template header override** — template has `From` header, message also sets `from` → message-level wins
24. **Pagination** — list templates/versions → correct `paging` URLs with pivots
25. **Max versions limit** — create 41st version → 400 error

---

## References

- **OpenAPI spec**: `mailgun.yaml` — lines ~11296-11974 (endpoints), ~20236-20543 (schemas)
- **API docs**: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/templates
- **Template user manual**: https://documentation.mailgun.com/docs/mailgun/user-manual/sending-messages/send-templates
- **Node.js SDK**: https://github.com/mailgun/mailgun.js — `lib/Classes/Domains/domainsTemplates.ts`, `lib/Types/Domains/DomainTemplates.ts`
- **Ruby SDK**: https://github.com/mailgun/mailgun-ruby — `lib/mailgun/templates/templates.rb`
- **Python SDK**: https://github.com/mailgun/mailgun-python — `mailgun/handlers/templates_handler.py`
- **Help center**: https://help.mailgun.com/hc/en-us/articles/360021380793-Email-Templates
