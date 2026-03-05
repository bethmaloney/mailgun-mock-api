import { test, expect } from "./fixtures";

const DOMAIN = "evt-e2e.example.com";

/**
 * Extract the storage key from a sendMessage response.
 * The response id looks like "<timestamp.hex@domain>" — the storage key
 * is the part inside the angle brackets.
 */
function extractStorageKey(sendResponse: Record<string, unknown>): string {
  const id = sendResponse.id as string;
  return id.replace(/^</, "").replace(/>$/, "");
}

test.describe("Events Page", () => {
  // 1. Shows empty state with domain selector — no events before domain selected
  test("shows empty state with domain selector", async ({ page }) => {
    await page.goto("/events");
    await expect(page.locator("main h1")).toHaveText("Events");

    // No domains exist, so the select shows "No domains available"
    await expect(page.locator("#domain-select")).toBeVisible();
    await expect(
      page.locator("#domain-select option", { hasText: "No domains available" }),
    ).toBeAttached();

    // Info message should appear
    await expect(
      page.getByText("No domains configured. Create a domain first to view events."),
    ).toBeVisible();

    // DataTable should NOT be rendered (wrapped in v-if="selectedDomain")
    await expect(page.locator("table.data-table")).not.toBeVisible();
  });

  // 2. Domain selector populated — create domains, verify dropdown contains them
  test("domain selector populated with created domains", async ({ page, api }) => {
    await api.createDomain("alpha-events.example.com");
    await api.createDomain("beta-events.example.com");

    await page.goto("/events");
    await expect(page.locator("main h1")).toHaveText("Events");

    const domainSelect = page.locator("#domain-select");
    await expect(domainSelect).toBeVisible();

    // Both domains should appear as options
    await expect(domainSelect.locator("option", { hasText: "alpha-events.example.com" })).toBeAttached();
    await expect(domainSelect.locator("option", { hasText: "beta-events.example.com" })).toBeAttached();
  });

  // 3. Lists events after selecting domain — send message + trigger events, select domain, verify event rows
  test("lists events after selecting domain", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Send a message — auto-generates "accepted" and "delivered" events
    const result = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Events test message",
      text: "Testing events",
    });
    const storageKey = extractStorageKey(result);

    // Trigger an additional "opened" event
    await api.triggerEvent(DOMAIN, "open", storageKey);

    await page.goto("/events");
    await expect(page.locator("main h1")).toHaveText("Events");

    // The domain should be auto-selected (first domain) and events loaded
    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // Should have at least 3 events: accepted, delivered, opened
    const rows = table.locator("tbody tr");
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(3);

    // Verify event types are shown as status badges
    await expect(table.locator(".status-badge", { hasText: "accepted" })).toBeVisible();
    await expect(table.locator(".status-badge", { hasText: "delivered" })).toBeVisible();
    await expect(table.locator(".status-badge", { hasText: "opened" })).toBeVisible();

    // Verify recipient column
    await expect(table.getByRole("cell", { name: `recipient@${DOMAIN}` }).first()).toBeVisible();
  });

  // 4. Event type shown as status badge — verify color-coded badge for different event types
  test("event type shown as color-coded status badge", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    const result = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Badge color test",
      text: "Testing badge colors",
    });
    const storageKey = extractStorageKey(result);

    // Trigger various event types to test badge colors
    await api.triggerEvent(DOMAIN, "fail", storageKey);
    await api.triggerEvent(DOMAIN, "open", storageKey);
    await api.triggerEvent(DOMAIN, "click", storageKey);
    await api.triggerEvent(DOMAIN, "unsubscribe", storageKey);
    await api.triggerEvent(DOMAIN, "complain", storageKey);

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // accepted/delivered -> badge-success (green)
    const acceptedBadge = table.locator(".status-badge", { hasText: "accepted" }).first();
    await expect(acceptedBadge).toHaveClass(/badge-success/);

    const deliveredBadge = table.locator(".status-badge", { hasText: "delivered" }).first();
    await expect(deliveredBadge).toHaveClass(/badge-success/);

    // failed -> badge-danger (red)
    const failedBadge = table.locator(".status-badge", { hasText: "failed" }).first();
    await expect(failedBadge).toHaveClass(/badge-danger/);

    // opened -> badge-info (blue)
    const openedBadge = table.locator(".status-badge", { hasText: "opened" }).first();
    await expect(openedBadge).toHaveClass(/badge-info/);

    // clicked -> badge-info (blue)
    const clickedBadge = table.locator(".status-badge", { hasText: "clicked" }).first();
    await expect(clickedBadge).toHaveClass(/badge-info/);

    // unsubscribed -> badge-warning (yellow)
    const unsubBadge = table.locator(".status-badge", { hasText: "unsubscribed" }).first();
    await expect(unsubBadge).toHaveClass(/badge-warning/);

    // complained -> badge-danger (red)
    const complainedBadge = table.locator(".status-badge", { hasText: "complained" }).first();
    await expect(complainedBadge).toHaveClass(/badge-danger/);
  });

  // 5. Filter by event type — filter to only "delivered" events
  test("filter by event type", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Send a message -> generates accepted + delivered
    const result = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Filter type test",
      text: "Testing type filter",
    });
    const storageKey = extractStorageKey(result);

    // Trigger an open event for variety
    await api.triggerEvent(DOMAIN, "open", storageKey);

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // Verify we have multiple event types initially
    const initialRows = table.locator("tbody tr");
    const initialCount = await initialRows.count();
    expect(initialCount).toBeGreaterThanOrEqual(3);

    // Select "delivered" from the event type filter
    // The event type select is the one with "All Event Types" option (not #domain-select)
    const eventTypeSelect = page.locator("select").filter({ hasText: "All Event Types" });
    await eventTypeSelect.selectOption("delivered");

    // Click Filter button
    await page.getByRole("button", { name: "Filter" }).click();

    // Should show only delivered events
    await expect(table.locator(".status-badge", { hasText: "delivered" })).toBeVisible();
    await expect(table.locator(".status-badge", { hasText: "accepted" })).not.toBeVisible();
    await expect(table.locator(".status-badge", { hasText: "opened" })).not.toBeVisible();
  });

  // 6. Filter by recipient — filter events by recipient address
  test("filter by recipient", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Send messages to different recipients
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `alice@${DOMAIN}`,
      subject: "For Alice",
      text: "Alice message",
    });
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `bob@${DOMAIN}`,
      subject: "For Bob",
      text: "Bob message",
    });

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // Initially should have events for both recipients
    await expect(table.getByRole("cell", { name: `alice@${DOMAIN}` }).first()).toBeVisible();
    await expect(table.getByRole("cell", { name: `bob@${DOMAIN}` }).first()).toBeVisible();

    // Type recipient filter and apply
    await page.getByPlaceholder("Recipient").fill(`alice@${DOMAIN}`);
    await page.getByRole("button", { name: "Filter" }).click();

    // Should only show events for alice
    await expect(table.getByRole("cell", { name: `alice@${DOMAIN}` }).first()).toBeVisible();
    await expect(table.getByRole("cell", { name: `bob@${DOMAIN}` })).not.toBeVisible();
  });

  // 7. Combined filters — apply event type + recipient filter
  test("combined filters", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Send a message to alice
    const result1 = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `alice@${DOMAIN}`,
      subject: "Alice combo",
      text: "Alice message",
    });
    const key1 = extractStorageKey(result1);

    // Trigger an open for alice
    await api.triggerEvent(DOMAIN, "open", key1);

    // Send a message to bob
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `bob@${DOMAIN}`,
      subject: "Bob combo",
      text: "Bob message",
    });

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // Apply combined filter: event type = delivered, recipient = alice
    const eventTypeSelect = page.locator("select").filter({ hasText: "All Event Types" });
    await eventTypeSelect.selectOption("delivered");
    await page.getByPlaceholder("Recipient").fill(`alice@${DOMAIN}`);
    await page.getByRole("button", { name: "Filter" }).click();

    // Should only show delivered events for alice
    await expect(table.locator(".status-badge", { hasText: "delivered" })).toBeVisible();
    await expect(table.getByRole("cell", { name: `alice@${DOMAIN}` }).first()).toBeVisible();

    // Should NOT show bob's events or alice's non-delivered events
    await expect(table.getByRole("cell", { name: `bob@${DOMAIN}` })).not.toBeVisible();
    await expect(table.locator(".status-badge", { hasText: "accepted" })).not.toBeVisible();
    await expect(table.locator(".status-badge", { hasText: "opened" })).not.toBeVisible();
  });

  // 8. Expand event detail — click event row, verify full JSON in pre block
  test("expand event detail", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Detail expand test",
      text: "Testing expand",
    });

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // Click on the event type badge link to expand detail
    const eventLink = table.locator("a.event-link").first();
    await eventLink.click();

    // Event detail panel should appear
    const detail = page.locator(".event-detail");
    await expect(detail).toBeVisible();
    await expect(detail.getByText("Event Detail")).toBeVisible();

    // Verify the JSON is rendered in a pre block
    const jsonPre = detail.locator("pre.event-json");
    await expect(jsonPre).toBeVisible();

    // The JSON should contain event properties
    const jsonText = await jsonPre.textContent();
    expect(jsonText).toBeTruthy();

    // Parse the JSON to verify it's valid
    const parsed = JSON.parse(jsonText!);
    expect(parsed).toHaveProperty("event");
    expect(parsed).toHaveProperty("recipient");
  });

  // 9. Collapse event detail — click expanded event again to collapse
  test("collapse event detail", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Collapse test",
      text: "Testing collapse",
    });

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // Click event type badge to expand
    const eventLink = table.locator("a.event-link").first();
    await eventLink.click();

    // Detail should be visible
    await expect(page.locator(".event-detail")).toBeVisible();
    await expect(page.getByText("Event Detail")).toBeVisible();

    // Click the same event type badge again to collapse
    await eventLink.click();

    // Detail should be hidden
    await expect(page.locator(".event-detail")).not.toBeVisible();
  });

  // 10. Pagination — generate enough events, verify next/previous navigation
  test("pagination works with next and previous buttons", async ({ page, api }) => {
    test.setTimeout(60000);
    await api.createDomain(DOMAIN);

    // Send 3 messages -> each creates 2 events (accepted + delivered) = 6 events total
    for (let i = 1; i <= 3; i++) {
      await api.sendMessage(DOMAIN, {
        from: `sender@${DOMAIN}`,
        to: `recipient${i}@${DOMAIN}`,
        subject: `Pagination msg ${String(i).padStart(3, "0")}`,
        text: `Body ${i}`,
      });
    }

    // Intercept events API calls and add limit=2 to force pagination
    await page.route("**/v3/*/events**", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("limit")) {
        url.searchParams.set("limit", "2");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible({ timeout: 10000 });

    // Should show first page with 2 rows (limit=2)
    const rows = table.locator("tbody tr");
    await expect(rows).toHaveCount(2, { timeout: 10000 });

    // Next button should be enabled on the first page (more results exist)
    const nextBtn = page.getByRole("button", { name: "Next" });
    const prevBtn = page.getByRole("button", { name: "Previous" });
    await expect(nextBtn).toBeEnabled();

    // Record the text of first-page rows to verify navigation changes content
    const firstPageText = await rows.first().textContent();

    // Go to next page
    await nextBtn.click();

    // Wait for the next page to load — should still have rows
    await expect(rows.first()).toBeVisible({ timeout: 10000 });
    // Previous should be enabled now that we're past the first page
    await expect(prevBtn).toBeEnabled();

    // The content should have changed from the first page
    const secondPageText = await rows.first().textContent();
    expect(secondPageText).not.toBe(firstPageText);

    // Navigate forward to the last page
    while (await nextBtn.isEnabled()) {
      await nextBtn.click();
      await expect(rows.first()).toBeVisible({ timeout: 10000 });
    }

    // On the last page, Next should be disabled
    await expect(nextBtn).toBeDisabled();
    // Previous should still be enabled
    await expect(prevBtn).toBeEnabled();

    // Go back using Previous — should show rows again
    await prevBtn.click();
    await expect(rows.first()).toBeVisible({ timeout: 10000 });
  });

  // 11. Auto-refreshes on WebSocket event.new — trigger event while viewing, verify it appears
  // Note: This test expects the backend to broadcast "event.new" via WebSocket
  // when events are created. The EventsPage component listens for this message
  // type and auto-refreshes. If the backend does not yet broadcast "event.new",
  // this test will fail — that is expected per the test plan.
  test("auto-refreshes on WebSocket event.new", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await page.goto("/events");
    await expect(page.locator("main h1")).toHaveText("Events");

    // The domain should auto-select. Initially there should be no events.
    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();

    // Send a message while viewing the events page — this generates
    // "accepted" and "delivered" events. The backend should broadcast
    // an "event.new" WebSocket message, causing the page to auto-refresh.
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "WebSocket live event",
      text: "Arrived via WS",
    });

    // Wait for the page to auto-refresh via WebSocket and show the new events.
    // The "No data available." text should disappear as event rows appear.
    await expect(page.getByText("No data available.")).not.toBeVisible({ timeout: 10000 });

    // Verify at least one event row appeared with a status badge
    await expect(table.locator(".status-badge").first()).toBeVisible({ timeout: 5000 });

    // The events should include "accepted" and/or "delivered" from the auto-generated events
    await expect(
      table.locator(".status-badge", { hasText: /accepted|delivered/ }).first(),
    ).toBeVisible({ timeout: 5000 });
  });
});
