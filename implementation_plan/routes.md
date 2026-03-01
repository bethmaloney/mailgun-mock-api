# Routes (Receiving)

Routes define rules for handling inbound email. When an incoming message matches a route's filter expression, Mailgun executes the route's actions (forward to a URL/email, store for later retrieval, or stop processing). Routes are **account-level** (global), not per-domain. They require an account-level API key — domain sending keys cannot manage routes.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v3/routes` | Create a route |
| GET | `/v3/routes` | List all routes (offset-based pagination) |
| GET | `/v3/routes/{id}` | Get a single route |
| PUT | `/v3/routes/{id}` | Update a route (partial update) |
| DELETE | `/v3/routes/{id}` | Delete a route |
| GET | `/v3/routes/match` | Test if an address matches a route |

---

## Schemas

### Route Object

```json
{
  "id": "4f3bad2335335426750048c6",
  "priority": 0,
  "description": "Sample route",
  "expression": "match_recipient(\".*@samples.mailgun.org\")",
  "actions": [
    "forward(\"http://myhost.com/messages/\")",
    "stop()"
  ],
  "created_at": "Wed, 15 Feb 2012 13:03:31 GMT"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (24-character hex string, e.g. `"4f3bad2335335426750048c6"`). |
| `priority` | integer | Lower number = higher priority. Routes with equal priority are evaluated in chronological order. Default: `0`. |
| `description` | string | Arbitrary human-readable description. |
| `expression` | string | Filter expression (see Expressions section below). |
| `actions` | string[] | Array of action strings (see Actions section below). |
| `created_at` | string | RFC 2822 timestamp (e.g. `"Wed, 15 Feb 2012 13:03:31 GMT"`). |

**Request/response field asymmetry:** The request field for actions is `action` (singular), while the response field is `actions` (plural). Both are arrays of strings.

---

## Endpoint Details

### POST `/v3/routes` — Create a Route

**Content-Type:** `application/x-www-form-urlencoded`

**Request fields:**

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `priority` | integer | No | `0` | Lower = higher priority. |
| `description` | string | No | `""` | Arbitrary description. |
| `expression` | string | Yes | — | Filter expression. |
| `action` | string[] | Yes | — | Action(s) to execute. Pass multiple `action` form fields for multiple actions. |

**Response (200):**

```json
{
  "message": "Route has been created",
  "route": {
    "id": "4f3bad2335335426750048c6",
    "priority": 0,
    "description": "Sample route",
    "expression": "match_recipient(\".*@samples.mailgun.org\")",
    "actions": [
      "forward(\"http://myhost.com/messages/\")",
      "stop()"
    ],
    "created_at": "Wed, 15 Feb 2012 13:03:31 GMT"
  }
}
```

### GET `/v3/routes` — List All Routes

**Query parameters:**

| Parameter | Type | Default | Max | Description |
|-----------|------|---------|-----|-------------|
| `skip` | integer | `0` | — | Number of records to skip. |
| `limit` | integer | `100` | `1000` | Max records to return. |

**Response (200):**

```json
{
  "total_count": 1,
  "items": [
    {
      "id": "4f3bad2335335426750048c6",
      "priority": 0,
      "description": "Sample route",
      "expression": "match_recipient(\".*@samples.mailgun.org\")",
      "actions": [
        "forward(\"http://myhost.com/messages/\")",
        "stop()"
      ],
      "created_at": "Wed, 15 Feb 2012 13:03:31 GMT"
    }
  ]
}
```

**Error:** `400` — `{"message": "The 'limit' parameter can't be larger than 1000"}`

### GET `/v3/routes/{id}` — Get a Single Route

**Path parameters:**

| Name | Type | Description |
|------|------|-------------|
| `id` | string | Route ID (hex string). |

**Response (200):**

```json
{
  "route": {
    "id": "4f3bad2335335426750048c6",
    "priority": 0,
    "description": "Sample route",
    "expression": "match_recipient(\".*@samples.mailgun.org\")",
    "actions": [
      "forward(\"http://myhost.com/messages/\")",
      "stop()"
    ],
    "created_at": "Wed, 15 Feb 2012 13:03:31 GMT"
  }
}
```

**Error:** `404` — `{"message": "Route not found"}`

### PUT `/v3/routes/{id}` — Update a Route

**Content-Type:** `application/x-www-form-urlencoded`

All fields are optional — only specified fields are updated; others remain unchanged.

**Path parameters:**

| Name | Type | Description |
|------|------|-------------|
| `id` | string | Route ID (hex string). |

**Request fields:**

| Field | Type | Description |
|-------|------|-------------|
| `priority` | integer | New priority value. |
| `description` | string | New description. |
| `expression` | string | New filter expression. |
| `action` | string[] | New action(s). Multiple `action` form fields for multiple actions. |

**Response (200):**

```json
{
  "message": "Route has been updated",
  "route": {
    "id": "4f3bad2335335426750048c6",
    "priority": 0,
    "description": "Sample route",
    "expression": "match_recipient(\".*@samples.mailgun.org\")",
    "actions": [
      "forward(\"http://myhost.com/messages/\")",
      "stop()"
    ],
    "created_at": "Wed, 15 Feb 2012 13:03:31 GMT"
  }
}
```

**Note:** The Ruby SDK integration tests show the update response may return route fields at the top level (flat) rather than nested under `"route"`. The Node.js SDK returns the full response body directly. The mock should return the nested form (`"route": { ... }`) to match the OpenAPI spec, as this is more consistent.

**Error:** `404` — `{"message": "Route not found"}`

### DELETE `/v3/routes/{id}` — Delete a Route

**Path parameters:**

| Name | Type | Description |
|------|------|-------------|
| `id` | string | Route ID (hex string). |

**Response (200):**

```json
{
  "message": "Route has been deleted",
  "id": "4f3bad2335335426750048c6"
}
```

Note: Delete response returns `id` at the top level (not nested in a `route` object).

**Error:** `404` — `{"message": "Route not found"}`

### GET `/v3/routes/match` — Test Route Matching

Test whether an address matches at least one route.

**Query parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `address` | string | Yes | Email address to match against route expressions. |

**Response (200) — Match found:**

```json
{
  "route": {
    "id": "4f3bad2335335426750048c6",
    "priority": 0,
    "description": "Sample route",
    "expression": "match_recipient(\".*@samples.mailgun.org\")",
    "actions": [
      "forward(\"http://myhost.com/messages/\")",
      "stop()"
    ],
    "created_at": "Wed, 15 Feb 2012 13:03:31 GMT"
  }
}
```

**Error:** `404` — `{"message": "Route not found"}`

---

## Route Expressions (Filters)

Expressions are filter rules that determine which inbound messages trigger a route. Mailgun uses Python-style regular expressions.

### Filter Functions

| Function | Syntax | Description |
|----------|--------|-------------|
| `match_recipient` | `match_recipient("pattern")` | Matches the SMTP recipient address against a regex pattern. |
| `match_header` | `match_header("header", "pattern")` | Matches an arbitrary MIME header against a regex pattern. |
| `catch_all` | `catch_all()` | Matches all incoming messages. Only triggers if no preceding routes matched. |

### Expression Examples

```
match_recipient("foo@bar.com")                           # exact match
match_recipient(".*@bar.com")                            # any address at bar.com
match_recipient("^chris\+(.*)@example.com$")             # plus addressing
match_header("subject", ".*support")                     # subject contains "support"
match_header("subject", "(.*)(urgent|help|asap)(.*)")    # multiple keywords
match_header("X-Mailgun-Sflag", "Yes")                   # spam flag
catch_all()                                              # default/fallback route
```

### Boolean Operators

Expressions support `and` to combine conditions:

```
match_recipient('^(.*)@example.com$') and match_header("Content-Language", "^(.*)en-US(.*)$")
```

### Regex Captures

Regex capture groups can be referenced in actions:
- Numbered captures: `\1`, `\2`, etc.
- Named captures: `(?P<name>...)` referenced as `\g<name>` in actions.

### Quoting

Both single and double quotes are valid inside expressions:
- `match_recipient(".*@gmail.com")` (double quotes — OpenAPI spec convention)
- `match_recipient('.*@gmail.com')` (single quotes — Python SDK convention)

The mock should accept both.

---

## Route Actions

Actions define what happens when an expression matches. A route can have multiple actions.

### Action Types

| Action | Syntax | Description |
|--------|--------|-------------|
| `forward()` | `forward("destination")` | Forward the message to an email address or HTTP URL. |
| `store()` | `store()` or `store(notify="url")` | Store the message temporarily (3 days). Optionally send a webhook notification. |
| `stop()` | `stop()` | Stop processing subsequent (lower-priority) routes. |

### `forward()` Details

The destination can be:
- **Email address:** `forward("mailbox@myapp.com")` — forwards the email as-is.
- **HTTP URL:** `forward("http://myapp.com/messages")` — Mailgun POSTs the parsed message content to this URL.

When forwarding to a URL, Mailgun posts the parsed email including headers, plain text body, HTML body, and attachments.

### `store()` Details

Stores the message on Mailgun's servers for later retrieval via the Messages API.

- **Without notify:** `store()` — message stored silently.
- **With notify:** `store(notify="http://myapp.com/callback")` — stores the message AND sends a webhook notification.

The notification payload differs from `forward()`:
- Attachments are NOT included inline — instead, an `attachments` JSON array contains download URLs.
- Each attachment object has: `url`, `content-type`, `name`, `size`.
- A `message-url` field provides the storage URL for the full message.
- The `content-id-map` maps content IDs to attachment URLs.

**Retrieval:** Stored messages are retrieved via `GET /v3/domains/{domain}/messages/{storage_key}` (already documented in messages.md). The storage key is found in the `storage.key` field of `stored` events.

**Retention:** Real Mailgun retains stored messages for 3 days (up to 7 depending on plan). The mock should keep stored messages indefinitely or until explicitly deleted.

### `stop()` Details

Without `stop()`, **all matching routes are evaluated** — a message can trigger multiple routes. `stop()` halts evaluation so no further routes process the message.

Common pattern — forward and stop:
```
actions: ["forward(\"http://myhost.com/messages/\")", "stop()"]
```

---

## Priority and Matching Behavior

1. **Priority ordering:** Routes are evaluated from lowest `priority` number (highest priority) to highest number.
2. **Equal priority:** Routes with the same priority are evaluated in chronological order (by creation time).
3. **All matching routes fire:** Unlike most routing systems, Mailgun evaluates all matching routes unless `stop()` is encountered.
4. **`catch_all()` special behavior:** Only matches if no preceding routes matched the message. Should be configured at the lowest priority (highest number).
5. **Expression + action character limit:** 4,000 characters total per route. If more is needed, create multiple routes with the same expression.

---

## Pagination

Routes use **offset-based pagination only** (`skip`/`limit`), unlike most other Mailgun list endpoints which support cursor-based pagination.

- `skip` (integer, default 0) — number of records to skip.
- `limit` (integer, default 100, max 1000) — max records to return.
- Response includes `total_count` alongside `items`.

---

## Mock Behavior Notes

### What to implement

1. **Full CRUD** — Create, list, get, update, delete routes.
2. **Route matching endpoint** — `GET /v3/routes/match?address=...` evaluates expressions against the given address and returns the first matching route.
3. **Expression parsing** — Parse `match_recipient`, `match_header`, and `catch_all` expressions. For the mock, support basic regex matching against stored recipient/header values.
4. **Priority-ordered evaluation** — When simulating inbound message processing, evaluate routes in priority order, fire all matching route actions, and stop at `stop()`.
5. **`store()` action** — When a route matches an inbound message with a `store()` action, store the message for retrieval via the existing `GET /v3/domains/{domain}/messages/{storage_key}` endpoint.
6. **`forward()` action** — When a route matches with a `forward()` action pointing to an HTTP URL, POST the parsed message to that URL. For email forwarding, generate a new outbound message.
7. **`action`/`actions` field naming** — Accept `action` (singular) in requests, return `actions` (plural) in responses.
8. **Route IDs** — Generate 24-character hex string IDs (MongoDB ObjectId-style).
9. **Timestamps** — Use RFC 2822 format for `created_at` (e.g. `"Wed, 15 Feb 2012 13:03:31 GMT"`).
10. **Partial updates** — PUT only updates fields that are provided; omitted fields remain unchanged.
11. **Limit validation** — Return 400 if `limit > 1000`.

### What to stub/simplify

1. **Expression evaluation depth** — Full Python-style regex with capture group substitution in actions is complex. The mock should support basic `match_recipient` regex matching, `match_header` regex matching, and `catch_all()`. Capture group references in actions (`\1`, `\g<name>`) can be left unresolved — they're rarely used in testing.
2. **`store(notify=...)` callback** — The mock can support this by posting a notification payload to the URL, but the detailed payload differences (attachment URLs vs inline) can be simplified. Just POST a JSON event payload.
3. **Inbound email simulation** — Real Mailgun receives email via MX records and triggers routes. The mock should provide a mock-specific endpoint (e.g. `POST /mock/inbound/{domain}`) to simulate inbound messages and trigger route evaluation. This should be documented in the Web UI plan.
4. **3-day retention** — The mock should keep stored messages indefinitely. No auto-expiration needed.
5. **4,000-character expression limit** — The mock doesn't need to enforce this.
6. **`and` operator** — Support `and` between two filter functions. More complex boolean logic (`or`, `not`) is not documented in the Mailgun API and doesn't need support.

### Integration Points

- **Events:** When routes process an inbound message, generate `stored` events (for `store()` action) or `accepted`/`delivered` events (for `forward()` action). The event's `is-routed` / `message.is_routed` field should be `true`.
- **Stored messages:** The `store()` action connects to the message retrieval endpoint documented in messages.md (`GET /v3/domains/{domain}/messages/{storage_key}`).
- **Webhooks:** Route-triggered events (`stored`, `delivered`, etc.) should trigger webhook delivery if webhooks are configured for those event types.

### Field Casing

Routes use `snake_case` for `created_at` and `total_count`, consistent with most Mailgun API responses.

### Content-Type

Create and update endpoints use `application/x-www-form-urlencoded` (not JSON). The `action` parameter is repeated for multiple actions (exploded array in form data).

---

## Client Library Patterns

### Node.js (`mailgun.js`)

```javascript
// List routes
const routes = await mg.routes.list({ limit: 50, skip: 0 });
// Returns: Route[] (unwraps response.body.items, discards total_count)

// Get single route
const route = await mg.routes.get('562da483125730608a7d1719');
// Returns: Route (unwraps response.body.route)

// Create route
const route = await mg.routes.create({
  priority: 0,
  description: 'sample',
  expression: 'match_recipient(".*@example.org")',
  action: ['forward("http://myhost.com/messages/")', 'stop()']
});
// Sends form-data via postWithFD. Returns: Route (unwraps response.body.route)

// Update route
const result = await mg.routes.update('562da483125730608a7d1719', {
  priority: 10,
  description: 'updated'
});
// Returns: UpdateRouteResponse (full body — includes message + route fields)

// Delete route
const result = await mg.routes.destroy('562da483125730608a7d1719');
// Returns: DestroyRouteResponse { id, message }
```

**Types:**
```typescript
type Route = {
  actions: string[];
  created_at: string;
  description: string;
  expression: string;
  id: string;
  priority: number;
}

type CreateUpdateRouteData = {
  priority?: number;
  description?: string;
  expression: string;
  action: string[];       // singular "action" for requests
}

type RoutesListQuery = { limit?: number; skip?: number; }
type UpdateRouteResponse = Route & { message: string; }
type DestroyRouteResponse = { id: string; message: string; }
```

### Ruby (`mailgun-ruby`)

No dedicated Route class — uses generic `Mailgun::Client` HTTP methods:

```ruby
# List routes
result = mg_client.get "routes", { limit: 50, skip: 10 }

# Get single route
result = mg_client.get "routes/#{route_id}"

# Create route (single action as string, or array for multiple)
result = mg_client.post "routes", {
  priority: 10,
  description: 'Test route',
  expression: 'match_recipient(".*@gmail.com")',
  action: 'forward("alice@example.com")'
}

# Create route (multiple actions)
result = mg_client.post "routes", {
  priority: 10,
  expression: 'match_recipient(".*@gmail.com")',
  action: ['forward("alice@example.com")', 'stop()']
}

# Update route
result = mg_client.put "routes/#{route_id}", { priority: 5, description: 'Updated' }

# Delete route
result = mg_client.delete "routes/#{route_id}"
```

The `action` field can be a single string or an array — both are accepted. The response always returns `actions` as an array.

### Python (`mailgun-python`)

```python
# List routes
req = client.routes.get(domain=domain, filters={"skip": 0, "limit": 1})
# Response: req.json()["items"], req.json()["total_count"]

# Get route by ID
req = client.routes.get(domain=domain, route_id="6012d994e8d489e24a127e79")
# Response: req.json()["route"]

# Create route
data = {
    "priority": 0,
    "description": "Sample route",
    "expression": "match_recipient('.*@example.com')",
    "action": ["forward('http://myhost.com/messages/')", "stop()"],
}
req = client.routes.create(domain=domain, data=data)
# Response: req.json()["message"], req.json()["route"]

# Update route (partial — only priority)
req = client.routes.put(domain=domain, data={"priority": 2}, route_id="60142b357c90c3c9f228e0a6")

# Delete route
req = client.routes.delete(domain=domain, route_id="60142b357c90c3c9f228e0a6")

# Match route to address
req = client.routes_match.get(domain=domain, filters={"address": sender})
# Response: req.json()["route"]
```

Note: The Python SDK accesses `/v3/routes/match` via `client.routes_match` (underscore in the property name).

---

## Test Scenarios

1. **Route CRUD lifecycle** — Create → Get → Update → Delete a route.
2. **List with pagination** — Create multiple routes, list with `skip`/`limit`, verify `total_count`.
3. **Priority ordering** — Create routes with different priorities, verify list order matches priority.
4. **Partial update** — Update only `priority`, verify other fields remain unchanged.
5. **Multiple actions** — Create a route with `forward()` and `stop()`, verify both are returned in `actions`.
6. **`action`/`actions` naming** — Send `action` in request, verify `actions` in response.
7. **Match endpoint** — Create a `match_recipient(".*@example.com")` route, match against `test@example.com` (should match) and `test@other.com` (should 404).
8. **`catch_all()` matching** — Create a `catch_all()` route, verify it matches when no other routes do.
9. **`match_header` matching** — Create a `match_header("subject", ".*urgent.*")` route, test matching behavior.
10. **Route ID format** — Verify generated IDs are 24-character hex strings.
11. **Limit validation** — Request `GET /v3/routes?limit=2000`, expect 400 error.
12. **404 on missing** — GET/PUT/DELETE a non-existent route ID, expect 404.
13. **Inbound message simulation** — Simulate an inbound message, verify routes are evaluated in priority order and matching routes fire their actions.
14. **`stop()` halts evaluation** — Create two matching routes; the first has `stop()`. Verify only the first route's actions are executed.
15. **`store()` action** — Simulate inbound message matching a route with `store()`, verify the message is retrievable via `GET /v3/domains/{domain}/messages/{storage_key}`.

---

## References

- **OpenAPI spec:** `mailgun.yaml` — endpoints at lines 9788–10168, `RouteResponse` schema at lines 20043–20077
- **API docs:** https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/routes
- **Route filters:** https://documentation.mailgun.com/docs/mailgun/user-manual/receive-forward-store/route-filters
- **Route actions:** https://documentation.mailgun.com/docs/mailgun/user-manual/receive-forward-store/route-actions
- **Storing/retrieving messages:** https://documentation.mailgun.com/docs/mailgun/user-manual/receive-forward-store/storing-and-retrieving-messages
- **Help center:** https://help.mailgun.com/hc/en-us/articles/360011355893-Routes
- **Node.js SDK:** https://github.com/mailgun/mailgun.js — `lib/Classes/Routes.ts`, `lib/Types/Routes/Routes.ts`
- **Ruby SDK:** https://github.com/mailgun/mailgun-ruby — generic client, `spec/integration/routes_spec.rb`
- **Python SDK:** https://github.com/mailgun/mailgun-python — `mailgun/handlers/routes_handler.py`, `mailgun/examples/routes_examples.py`
