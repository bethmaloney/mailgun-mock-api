# E2E Testing Plan

Comprehensive Playwright test plan for 100% frontend behavior coverage.

## Existing Tests

### `smoke.spec.ts` ✅
- [x] All 12 pages load correctly (verifies heading matches)
- [x] Sidebar navigation links work

### `domains.spec.ts` ✅
- [x] Shows empty state (0 total)
- [x] Create domain via API and verify in list
- [x] Create multiple domains
- [x] Create domain via UI form
- [x] View domain detail (detail panel and DNS sections)
- [x] Delete domain via UI with confirmation dialog

---

## Tests Needed

### `dashboard.spec.ts` ✅
- [x] Shows zero-state when no data exists — all counts show 0, no recent deliveries
- [x] Shows message statistics — send messages via API, verify total and last-hour counts
- [x] Shows domain statistics — create domains (some verified, some not), verify total/active/unverified counts
- [x] Shows webhook count — create webhooks via API, verify configured count
- [x] Shows event statistics — trigger events via API, verify accepted/delivered/failed/opened/clicked/complained/unsubscribed counts
- [x] Shows recent webhook deliveries table — create webhook + send message, verify delivery row with URL, event, status code, timestamp
- [x] Auto-refreshes on WebSocket `message.new` — send message while on dashboard, verify counts update without manual refresh
- [x] Auto-refreshes on WebSocket `data.reset` — populate data, reset via API, verify counts return to 0

### `messages.spec.ts` ✅
- [x] Shows empty state — "No data available" when no messages
- [x] Lists messages after sending — send messages via API, verify table rows (from, to, subject, domain, status, date)
- [x] Shows tags in table — send message with tags, verify tags display
- [x] Filter by domain — send messages to different domains, filter by one domain, verify only matching shown
- [x] Filter by from — filter messages by sender address
- [x] Filter by to — filter messages by recipient
- [x] Filter by subject — filter by subject text
- [x] Combined filters — apply multiple filters simultaneously
- [x] Pagination — send enough messages to paginate, verify next/previous buttons work
- [x] View message detail — click subject, verify detail panel shows from, to, subject, domain, message ID, storage key
- [x] Detail shows text body — send message with text body, verify text body section
- [x] Detail shows HTML body in iframe — send message with HTML body, verify iframe preview
- [x] Detail shows custom headers — send message with h:X-Custom header, verify JSON display
- [x] Detail shows custom variables — send message with v:vars, verify JSON display
- [x] Detail shows attachments — send message with attachment, verify attachment list with filename, type, size
- [x] Detail shows events timeline — trigger events on a message, verify events with status badges
- [x] Close detail by clicking again — click subject to open, click again to close
- [x] Delete individual message — click delete icon, confirm dialog, verify removed
- [x] Clear all messages — click clear all, confirm, verify table empty
- [x] Auto-refreshes on WebSocket `message.new` — send message while viewing page, verify it appears

### `events.spec.ts`
- [ ] Shows empty state with domain selector — no events before domain selected
- [ ] Domain selector populated — create domains, verify dropdown contains them
- [ ] Lists events after selecting domain — send message + trigger events, select domain, verify event rows
- [ ] Event type shown as status badge — verify color-coded badge for different event types
- [ ] Filter by event type — filter to only "delivered" events
- [ ] Filter by recipient — filter events by recipient address
- [ ] Combined filters — apply event type + recipient filter
- [ ] Expand event detail — click event row, verify full JSON in pre block
- [ ] Collapse event detail — click expanded event again to collapse
- [ ] Pagination — generate enough events, verify next/previous navigation
- [ ] Auto-refreshes on WebSocket `event.new` — trigger event while viewing, verify it appears

### `templates.spec.ts`
- [ ] Shows empty state — no templates for selected domain
- [ ] Domain selector populated — verify domains appear in dropdown
- [ ] Lists templates — create templates via API, verify table shows name, description, created_at
- [ ] View template detail — click template name, verify detail panel with name, description, active version
- [ ] View versions list in detail — verify versions table with tag, engine, active, comment
- [ ] View version detail — click version tag, verify version detail with template body
- [ ] Delete template — click delete, confirm, verify removed
- [ ] Pagination for templates — verify pagination with many templates
- [ ] Pagination for versions — verify pagination with many versions

### `mailing-lists.spec.ts`
- [ ] Shows empty state — no mailing lists displayed
- [ ] Create mailing list — fill address, name, description, submit, verify appears in table
- [ ] Create list with only address — name and description optional
- [ ] Lists show correct columns — address, name, members count, access level, created at
- [ ] View list detail — click address, verify detail panel with all fields
- [ ] Add member to list — fill email + name, add, verify member appears in members table
- [ ] Member shows subscribed status — verify subscribed column shows yes/no
- [ ] Remove member from list — click delete on member, verify removed
- [ ] Delete mailing list — click delete on list, confirm, verify removed
- [ ] Pagination for lists — verify pagination with many lists
- [ ] Pagination for members — verify pagination with many members

### `routes.spec.ts`
- [ ] Shows empty state — no routes displayed
- [ ] Toggle create form — click "Add Route" to show form, click "Cancel" to hide
- [ ] Create route via UI — fill priority, expression, actions, description, submit, verify appears
- [ ] Routes table columns — priority, expression, actions, description, created at
- [ ] View route detail — click expression, verify detail panel with ID, priority, description, expression, actions
- [ ] Delete route — click delete, confirm, verify removed
- [ ] Pagination — verify next/previous with many routes

### `suppressions.spec.ts`
- [ ] Domain selector required — table not shown until domain selected
- [ ] Bounces tab — empty state
- [ ] Bounces tab — add bounce — fill address, code, error message, submit, verify appears
- [ ] Bounces tab — delete bounce — click delete, verify removed
- [ ] Bounces tab — clear all — click clear all, confirm, verify all removed
- [ ] Complaints tab — switch tab — click Complaints tab, verify active
- [ ] Complaints tab — add complaint — fill address, submit, verify appears
- [ ] Complaints tab — delete complaint — click delete, verify removed
- [ ] Unsubscribes tab — switch tab
- [ ] Unsubscribes tab — add unsubscribe — fill address and tag, submit, verify appears
- [ ] Unsubscribes tab — delete unsubscribe — click delete, verify removed
- [ ] Allowlist tab — switch tab
- [ ] Allowlist tab — add by address — select type "address", fill value, submit, verify appears
- [ ] Allowlist tab — add by domain — select type "domain", fill value, submit, verify appears
- [ ] Allowlist tab — delete entry — click delete, verify removed
- [ ] Search filter — add multiple entries, type in search, verify client-side filtering
- [ ] Pagination — verify pagination across tabs
- [ ] Clear all per tab — verify clear all only clears active tab's data

### `webhooks.spec.ts`
- [ ] Shows empty state — no webhooks configured, no delivery log
- [ ] Domain selector populated — verify domains in dropdown
- [ ] Create webhook — select event type, enter URL, submit, verify appears in table
- [ ] Webhook table shows event type badge and URLs
- [ ] Delete webhook — click delete, verify removed
- [ ] Delivery log shows entries — create webhook, send message, verify log entry
- [ ] Expand delivery detail — click timestamp, verify request/response JSON
- [ ] Collapse delivery detail — click again to collapse
- [ ] Delivery log pagination — verify pagination with many deliveries

### `settings.spec.ts`
- [ ] Loads current configuration — all settings sections display current values
- [ ] Event Generation — save settings — change auto_deliver, delivery_delay_ms, etc., save, verify success
- [ ] Domain Behavior — save settings — toggle domain_auto_verify, change sandbox_domain, save
- [ ] Webhook Delivery — save settings — change webhook_retry_mode, webhook_timeout_ms, save
- [ ] Authentication — save settings — change auth_mode, verify signing_key is read-only
- [ ] Storage — save settings — toggle store_attachment_bytes, change max_messages, max_events, save
- [ ] Success message auto-hides — verify success message disappears after ~3 seconds
- [ ] Reset All Data — click reset all, confirm, verify success
- [ ] Reset Messages & Events — click reset messages, confirm, verify success
- [ ] Reset Per Domain — select domain, click reset, confirm, verify success
- [ ] Cancel reset confirmation — click reset, cancel dialog, verify no reset occurred

### `trigger-events.spec.ts`
- [ ] 3-step workflow visibility — only domain selector visible initially
- [ ] Domain selector loads domains
- [ ] Message list loads after domain selected — select domain, verify messages table appears
- [ ] Message search filter — type in search, verify messages filtered
- [ ] Select message — click select button, verify step 3 with message summary
- [ ] Event type buttons — verify all 6 buttons present (Deliver, Fail, Open, Click, Unsubscribe, Complain)
- [ ] Trigger deliver event — click Deliver, Trigger, verify success
- [ ] Trigger fail event — shows severity and error fields — click Fail, verify fields appear
- [ ] Trigger fail event — fill severity + error, trigger, verify success
- [ ] Trigger click event — shows URL field — click Click, verify URL input
- [ ] Trigger click event — fill URL, trigger, verify success
- [ ] Trigger open event — trigger, verify success
- [ ] Trigger unsubscribe event — trigger, verify success
- [ ] Trigger complain event — trigger, verify success
- [ ] Error result displayed — trigger event that fails, verify error badge

### `simulate-inbound.spec.ts`
- [ ] Form shows required fields — from, to, subject visible, submit button disabled
- [ ] Domain selector auto-populates To field — select domain, verify To updates
- [ ] Submit button disabled until required fields filled
- [ ] Simulate inbound message — fill form, submit, verify result panel
- [ ] Result shows matched routes — create route via API, simulate, verify matched routes
- [ ] Result shows "no routes matched" — simulate without matching routes, verify info message
- [ ] Result shows actions executed — verify actions section when routes match
- [ ] Reset form — fill form, click reset, verify all fields cleared

### `shared-components.spec.ts`
- [ ] DataTable loading state — verify "Loading..." shown during fetch
- [ ] DataTable empty state — verify "No data available" when rows empty
- [ ] StatusBadge colors — verify correct colors for delivered (green), opened (blue), failed (red), unsubscribed (orange)
- [ ] Pagination disabled states — Previous disabled on first page, Next disabled on last page
- [ ] WebSocket connection indicator — verify green dot when connected in sidebar
