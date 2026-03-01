# Mailing Lists

Mailing lists allow grouping recipients under a single email address. Sending to the list address expands to individual deliveries to each subscribed member. The mock must support full CRUD for both lists and members, including bulk operations.

## API Endpoints

### Mailing List CRUD

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v3/lists` | Create a mailing list |
| GET | `/v3/lists/{list_address}` | Get a mailing list by address |
| PUT | `/v3/lists/{list_address}` | Update a mailing list |
| DELETE | `/v3/lists/{list_address}` | Delete a mailing list (and all members) |
| GET | `/v3/lists/pages` | List all mailing lists (cursor-based pagination) |
| GET | `/v3/lists` | List all mailing lists (legacy offset-based pagination) |

### Member CRUD

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v3/lists/{list_address}/members` | Add a single member |
| GET | `/v3/lists/{list_address}/members/{member_address}` | Get a single member |
| PUT | `/v3/lists/{list_address}/members/{member_address}` | Update a member |
| DELETE | `/v3/lists/{list_address}/members/{member_address}` | Delete a member |
| GET | `/v3/lists/{list_address}/members/pages` | List members (cursor-based pagination) |
| GET | `/v3/lists/{list_address}/members` | List members (legacy offset-based pagination) |

### Bulk Operations

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v3/lists/{list_address}/members.json` | Bulk add members via JSON (up to 1000) |
| POST | `/v3/lists/{list_address}/members.csv` | Bulk add members via CSV (up to 1000) |

### Validation (Stub Only)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v3/lists/{list_address}/validate` | Start list validation job |
| GET | `/v3/lists/{list_address}/validate` | Get validation results |
| DELETE | `/v3/lists/{list_address}/validate` | Cancel validation job |

Validation is a paid Mailgun feature (email verification for list members). The mock should accept these calls and return a canned success response but does not need real validation logic.

---

## Schemas

### MailingList Object

```json
{
  "address": "developers@example.com",
  "name": "Developers",
  "description": "Mailgun developers list",
  "access_level": "readonly",
  "reply_preference": "list",
  "created_at": "Tue, 09 Aug 2011 20:50:27 -0000",
  "members_count": 2
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | Yes (create) | — | Email address of the list (e.g., `devs@example.com`). Acts as the list identifier. |
| `name` | string | No | `""` | Display name. |
| `description` | string | No | `""` | Description text. |
| `access_level` | string enum | No | `"readonly"` | Who can post: `readonly`, `members`, `everyone`. |
| `reply_preference` | string enum | No | `"list"` | Where replies go: `list` or `sender`. Can be `null` in responses. |
| `created_at` | string | Response only | — | RFC 2822 timestamp (e.g., `"Tue, 09 Aug 2011 20:50:27 -0000"`). |
| `members_count` | integer | Response only | — | Number of members in the list. |

**Access level behavior:**
- `readonly` — Only list admins can send to the list. Replies forced to `sender`.
- `members` — Only subscribed members may send to the list.
- `everyone` — Anyone may send, including non-subscribers.

**Reply preference behavior:**
- `list` — Replies go to the mailing list address.
- `sender` — Replies go to the original sender's address.
- For `readonly` lists, `reply_preference` is forced to `sender` regardless of what is set.

### ListMember Object

```json
{
  "address": "alice@example.com",
  "name": "Alice",
  "subscribed": true,
  "vars": {
    "gender": "female",
    "age": 27
  }
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | Yes | — | Email address of the member. |
| `name` | string | No | `""` | Display name. |
| `subscribed` | boolean | No | `true` | Subscription status. `false` = excluded from mailings but record kept. |
| `vars` | object | No | `{}` | Arbitrary key-value data for personalization/segmentation. |

### MailingList in Event/Log Payloads

When events reference mailing list sends, the event `mailing-list` field uses this shape:

```json
{
  "address": "developers@example.com",
  "list-id": "some-id",
  "sid": "some-sid"
}
```

This is a read-only field in event data. The mock can include it in events generated from list sends.

---

## Endpoint Details

### POST `/v3/lists` — Create Mailing List

**Content-Type:** `multipart/form-data`

**Request fields:**

| Field | Type | Required |
|-------|------|----------|
| `address` | string | Yes |
| `name` | string | No |
| `description` | string | No |
| `access_level` | string | No (default: `readonly`) |
| `reply_preference` | string | No (default: `list`) |

**Response (201):**

```json
{
  "message": "Mailing list has been created",
  "list": {
    "address": "developers@example.com",
    "name": "Developers",
    "description": "Describe the mailing list",
    "access_level": "readonly",
    "reply_preference": "list",
    "created_at": "Tue, 09 Aug 2011 20:50:27 -0000",
    "members_count": 0
  }
}
```

**Error responses:**
- `400` — Invalid `access_level` or `reply_preference`: `{"message": "Invalid access level 'fake'. It can be any of: 'readonly', 'members', 'everyone'."}`
- `400` — Invalid `reply_preference`: `{"message": "Invalid reply preference 'wrong'. It can be any of: 'sender', 'list'"}`
- `429` — Rate limited: `{"message": "Too Many Requests"}`

### GET `/v3/lists/{list_address}` — Get Mailing List

**Response (200):**

```json
{
  "list": {
    "address": "developers@example.com",
    "name": "Developers",
    "description": "Describe the mailing list",
    "access_level": "readonly",
    "reply_preference": "list",
    "created_at": "Tue, 09 Aug 2011 20:50:27 -0000",
    "members_count": 2
  }
}
```

**Error:** `404` — `{"message": "Mailing list developers@example.com not found"}`

### PUT `/v3/lists/{list_address}` — Update Mailing List

**Content-Type:** `multipart/form-data`

Only included fields are updated; omitted fields remain unchanged.

**Request fields:**

| Field | Type | Description |
|-------|------|-------------|
| `address` | string | New list address (to rename). |
| `name` | string | New display name. |
| `description` | string | New description. |
| `access_level` | string | New access level. |
| `reply_preference` | string | New reply preference. |
| `list-id` | string | Sets the List-Id email header value. |

**Response (200):**

```json
{
  "message": "Mailing list has been updated",
  "list": { /* updated MailingList object */ }
}
```

**Error:** `404` — `{"message": "Mailing list developers@example.com not found"}`

### DELETE `/v3/lists/{list_address}` — Delete Mailing List

Deletes the list and all its members.

**Response (200):**

```json
{
  "address": "developers@example.com",
  "message": "Mailing list has been removed"
}
```

**Error:** `404` — `{"message": "Mailing list developers@example.com not found"}`

### GET `/v3/lists/pages` — List Mailing Lists (Cursor-Based)

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 100 | Max results per page. Max value: 100. |
| `address` | string | — | Pivot address for pagination cursor. |
| `page` | string | — | Page direction: `first`, `last`, `next`, `prev`. |

**Response (200):**

```json
{
  "items": [ /* array of MailingList objects */ ],
  "paging": {
    "first": "https://api.mailgun.net/v3/lists/pages?page=first",
    "last": "https://api.mailgun.net/v3/lists/pages?page=last",
    "next": "https://api.mailgun.net/v3/lists/pages?page=next&address=devs@example.com",
    "previous": "https://api.mailgun.net/v3/lists/pages?page=prev&address=devs@example.com"
  }
}
```

### GET `/v3/lists` — List Mailing Lists (Legacy Offset-Based)

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | integer | 100 | Max results. |
| `skip` | integer | 0 | Number of items to skip. |
| `address` | string | — | Filter by specific address. |

**Response (200):**

```json
{
  "total_count": 1,
  "items": [ /* array of MailingList objects */ ]
}
```

### POST `/v3/lists/{list_address}/members` — Add Single Member

**Content-Type:** `multipart/form-data`

**Request fields:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | Yes | — | Member email address. |
| `name` | string | No | `""` | Display name. |
| `vars` | string (JSON) | No | `{}` | JSON-encoded dict of custom variables. |
| `subscribed` | boolean/string | No | `true` | Accepts `true`/`false` or `"yes"`/`"no"`. |
| `upsert` | boolean/string | No | `false` | If `true`/`"yes"`, updates existing member instead of erroring. |

**Response (200):**

```json
{
  "message": "Mailing list member has been created",
  "member": {
    "address": "alice@example.com",
    "name": "Alice",
    "subscribed": true,
    "vars": {"gender": "female", "age": 27}
  }
}
```

**Error:** `400` — `{"message": "Address already exists 'alice@example.com'"}` (when `upsert` is false)

### GET `/v3/lists/{list_address}/members/{member_address}` — Get Member

**Response (200):**

```json
{
  "member": {
    "address": "alice@example.com",
    "name": "Alice",
    "subscribed": true,
    "vars": {"gender": "female", "age": 27}
  }
}
```

**Note:** The OpenAPI spec shows the response as a bare `ListMemberResponse` without the `member` wrapper, but SDKs and docs consistently expect `{ "member": { ... } }`. The mock should use the wrapped form.

### PUT `/v3/lists/{list_address}/members/{member_address}` — Update Member

**Content-Type:** `multipart/form-data`

Only included fields are updated.

**Request fields:**

| Field | Type | Description |
|-------|------|-------------|
| `address` | string | New email address (to change it). |
| `name` | string | New display name. |
| `vars` | string (JSON) | JSON-encoded dict of custom variables. |
| `subscribed` | boolean/string | New subscription status. |

**Response (200):**

```json
{
  "message": "Mailing list member has been updated",
  "member": { /* updated ListMember object */ }
}
```

### DELETE `/v3/lists/{list_address}/members/{member_address}` — Delete Member

**Response (200):**

```json
{
  "member": {
    "address": "alice@example.com"
  },
  "message": "Mailing list member has been deleted"
}
```

**Error:** `404` — `{"message": "Member dev@example.com of mailing list alice@example.com not found"}`

### GET `/v3/lists/{list_address}/members/pages` — List Members (Cursor-Based)

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `subscribed` | boolean | — | Filter by subscription status. |
| `limit` | integer | 100 | Max results per page. Max: 100. |
| `address` | string | — | Pivot address for cursor. |
| `page` | string | — | Page direction: `first`, `last`, `next`, `prev`. |

**Response (200):**

```json
{
  "items": [ /* array of ListMember objects */ ],
  "paging": {
    "first": "https://...",
    "last": "https://...",
    "next": "https://...",
    "previous": "https://..."
  }
}
```

### GET `/v3/lists/{list_address}/members` — List Members (Legacy Offset-Based)

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `address` | string | — | Filter by specific member address. |
| `subscribed` | boolean | — | Filter by subscription status. |
| `limit` | integer | 100 | Max results. Max: 100. |
| `skip` | integer | 0 | Number to skip. |

**Response (200):**

```json
{
  "total_count": 1,
  "items": [ /* array of ListMember objects */ ]
}
```

### POST `/v3/lists/{list_address}/members.json` — Bulk Add Members (JSON)

**Content-Type:** `multipart/form-data`

**Request fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `members` | string (JSON array) | Yes | JSON-encoded array of member objects or email strings. Max 1000. |
| `upsert` | boolean/string | No (default: `false`) | If true, update existing members instead of erroring. |

The `members` field accepts two formats:
1. Array of email strings: `["bob@example.com", "alice@example.com"]`
2. Array of member objects: `[{"address": "bob@example.com", "name": "Bob", "subscribed": true, "vars": {"age": 30}}]`

**Response (200):**

```json
{
  "list": { /* MailingList object with updated members_count */ },
  "message": "Mailing list has been updated",
  "task-id": "4321"
}
```

**Async behavior:** If the request contains more than 100 entries, the operation is processed asynchronously. The response returns immediately with a `task-id`. For ≤100 entries, it's processed synchronously but still returns `task-id`.

**Error:** `404` — `{"message": "Mailing list 'devs@mg.net' not found"}`

### POST `/v3/lists/{list_address}/members.csv` — Bulk Add Members (CSV)

**Content-Type:** `multipart/form-data`

**Request fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `members` | file (CSV) | Yes | CSV file with member data. Max 5 MB. |
| `subscribed` | boolean | No | Default subscription status for all uploaded members. |
| `upsert` | boolean | No (default: `false`) | If true, update existing members. |

**Response (200):** Same shape as the JSON bulk endpoint (returns `list`, `message`, `task-id`).

**Error:** `400` — `{"message": "CSV file is too big, max allowed size is 5 MB"}`

---

## Pagination

Mailing lists use **two pagination styles** that the mock must support:

### Cursor-Based (preferred)

Used by `/v3/lists/pages` and `/v3/lists/{address}/members/pages`.

- Uses a `paging` object with `first`, `last`, `next`, `previous` URL fields.
- The `address` query parameter acts as the cursor pivot.
- The `page` query parameter selects direction (`first`, `last`, `next`, `prev`).
- This is the same paging model used by suppressions and other Mailgun list endpoints. A shared pagination utility should be reused.

### Offset-Based (legacy)

Used by `/v3/lists` and `/v3/lists/{address}/members`.

- Uses `skip`/`limit` query parameters.
- Returns `total_count` alongside `items`.
- Still actively used by the Ruby SDK.

---

## Mock Behavior Notes

### What to implement

1. **Full CRUD** for both mailing lists and members.
2. **Both pagination styles** — cursor-based and offset-based.
3. **Bulk JSON add** (`/members.json`) — parse the JSON array from the `members` form field, apply upsert logic.
4. **Bulk CSV add** (`/members.csv`) — parse CSV content, extract member records.
5. **`members_count` tracking** — automatically increment/decrement as members are added/removed.
6. **`vars` handling** — accept as JSON string in requests, store as object, return as object.
7. **`subscribed` field** — accept both boolean (`true`/`false`) and string (`"yes"`/`"no"`) inputs, always return boolean.
8. **`upsert` flag** — when `true`, update existing member; when `false`, return 400 for duplicates.
9. **Access level validation** — reject invalid values with appropriate error messages.
10. **Reply preference validation** — reject invalid values; enforce `sender` for `readonly` lists.

### What to stub/simplify

1. **Async bulk processing** — The mock can process all bulk operations synchronously and always return a `task-id` (even though it's instant).
2. **List validation** (`/validate`) — Accept calls, return canned success. No real email verification.
3. **Rate limiting (429)** — The mock doesn't need to enforce rate limits. Just don't return 429.
4. **CSV file size limit** — Can skip enforcing the 5 MB limit.
5. **MX record checks** — The mock doesn't verify domain MX records for list posting access.

### Integration with Message Sending

When a message is sent to a list address via `POST /v3/{domain}/messages`:
1. Look up the mailing list by the `to` address.
2. Expand to all subscribed members (`subscribed: true`).
3. Member `vars` are available for template variable substitution (via `%recipient.varname%` syntax).
4. The `%mailing_list_unsubscribe_url%` variable should generate an unsubscribe link for each recipient.
5. Each expanded delivery generates its own events (accepted, delivered, etc.) with the `mailing-list` field populated.

### Field Casing

Mailing lists use `snake_case` consistently:
- `access_level`, `reply_preference`, `created_at`, `members_count`, `total_count`
- This matches suppressions (`created_at`) but differs from templates/allowlists (`createdAt` camelCase).
- The `task-id` field in bulk responses uses a **hyphenated** key — this must be preserved exactly.

### Content-Type

All write endpoints (POST, PUT) accept `multipart/form-data`, not JSON request bodies. The `vars` and `members` fields are JSON-encoded strings within the form data.

---

## Client Library Patterns

### Node.js (`mailgun.js`)

```javascript
// List CRUD
const lists = await mg.lists.list();              // GET /v3/lists/pages
const list = await mg.lists.get('devs@mg.net');    // GET /v3/lists/{address}
await mg.lists.create({ address: 'devs@mg.net' }); // POST /v3/lists
await mg.lists.update('devs@mg.net', { name: 'Devs' }); // PUT /v3/lists/{address}
await mg.lists.destroy('devs@mg.net');             // DELETE /v3/lists/{address}

// Members
await mg.lists.members.listMembers('devs@mg.net');
await mg.lists.members.createMember('devs@mg.net', { address: 'a@b.com', subscribed: true });
await mg.lists.members.createMembers('devs@mg.net', { members: [...], upsert: 'yes' }); // bulk JSON
await mg.lists.members.updateMember('devs@mg.net', 'a@b.com', { name: 'Alice' });
await mg.lists.members.destroyMember('devs@mg.net', 'a@b.com');
```

- Converts `subscribed: true/false` to `"yes"/"no"` before sending.
- Auto-stringifies `vars` object to JSON.
- Uses cursor-based pagination endpoints (`/pages`).

### Ruby (`mailgun-ruby`)

```ruby
# Uses generic HTTP methods — no dedicated list classes
mg_client.post "lists", { address: 'devs@mg.net', name: 'Devs' }
mg_client.get "lists/devs@mg.net"
mg_client.get "lists/devs@mg.net/members", { limit: 50, skip: 0 }
mg_client.post "lists/devs@mg.net/members", { address: 'a@b.com' }
```

- Uses legacy offset-based pagination (`skip`/`limit`).
- Includes a unique `OptInHandler` class for double opt-in flows (HMAC-based hash generation/validation). The mock doesn't need to replicate this — it's client-side logic.

### Python (`mailgun-python`)

```python
client.lists.create(domain=domain, data={"address": "devs@mg.net"})
client.lists_pages.get(domain=domain)
client.lists_members.create(domain=domain, address="devs@mg.net", data={...})
client.lists_members.create(domain=domain, address="devs@mg.net", data={...}, multiple=True)  # bulk
```

- Uses `_pages` suffix for paginated endpoints.
- `multiple=True` kwarg switches to `/members.json` endpoint.
- `vars` must be passed as a JSON string, not a Python dict.

---

## Test Scenarios

1. **List CRUD lifecycle** — Create → Get → Update → Delete a mailing list.
2. **Member CRUD lifecycle** — Add → Get → Update → Delete a member.
3. **Bulk add (JSON)** — Add multiple members via `/members.json`, verify `members_count` updates.
4. **Bulk add (CSV)** — Add members via CSV upload.
5. **Upsert behavior** — Add a member, then add again with `upsert: false` (expect 400), then with `upsert: true` (expect update).
6. **Subscription filtering** — Add subscribed and unsubscribed members, filter with `?subscribed=true`.
7. **Both pagination styles** — Verify cursor-based (`/pages`) and offset-based (`skip`/`limit`) pagination work.
8. **Access level validation** — Reject invalid `access_level` values with appropriate 400 error.
9. **Reply preference enforcement** — Setting `reply_preference: list` on a `readonly` list should be forced to `sender`.
10. **Member vars** — Store vars as JSON string in request, return as object in response.
11. **List expansion on send** — Send a message to a list address, verify it generates events for each subscribed member.
12. **Delete list cascades** — Deleting a list removes all its members.
13. **Subscribed vs deleted** — `subscribed: false` keeps the record; DELETE removes it entirely.

---

## References

- **OpenAPI spec:** `mailgun.yaml` — endpoints at lines 10170–11295, schemas at lines 20078–20230
- **API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/mailing-lists
- **User manual:** https://documentation.mailgun.com/docs/mailgun/user-manual/sending-messages/mailing-lists
- **Node.js SDK:** https://github.com/mailgun/mailgun.js — `lib/Classes/MailingLists/`
- **Ruby SDK:** https://github.com/mailgun/mailgun-ruby — generic client, `docs/Snippets.md`
- **Python SDK:** https://github.com/mailgun/mailgun-python — `mailgun/handlers/mailinglists_handler.py`
