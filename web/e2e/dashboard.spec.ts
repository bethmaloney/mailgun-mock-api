import { test, expect } from "./fixtures";

/**
 * Helper to extract the storage key from a sendMessage response.
 * The response id looks like "<timestamp.hex@domain>" — the storage key
 * is the part inside the angle brackets.
 */
function extractStorageKey(sendResponse: Record<string, unknown>): string {
  const id = sendResponse.id as string;
  return id.replace(/^</, "").replace(/>$/, "");
}

/**
 * Locate a stat value inside a dashboard card.
 * Finds the card by its h2 title, then locates the stat-row with the given
 * label and returns the locator for its stat-value span.
 */
function statValue(
  page: import("@playwright/test").Page,
  cardTitle: string,
  label: string,
) {
  const card = page.locator(".card", { has: page.locator(`h2.card-title:text-is("${cardTitle}")`) });
  const row = card.locator(".stat-row", { has: page.locator(`.stat-label:text-is("${label}")`) });
  return row.locator(".stat-value");
}

/**
 * Locate an event count by the StatusBadge text (e.g. "accepted").
 * Each .event-stat contains a StatusBadge (whose textContent is lowercase,
 * with CSS text-transform: capitalize for visual rendering) and a .event-count span.
 */
function eventCount(
  page: import("@playwright/test").Page,
  badgeText: string,
) {
  const stat = page.locator(".event-stat", { has: page.locator(`.status-badge:text-is("${badgeText}")`) });
  return stat.locator(".event-count");
}

const DOMAIN = "dashboard-e2e.example.com";

test.describe("Dashboard", () => {
  test("shows zero-state when no data exists", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    // Messages card
    await expect(statValue(page, "Messages", "Total")).toHaveText("0");
    await expect(statValue(page, "Messages", "Last Hour")).toHaveText("0");

    // Domains card
    await expect(statValue(page, "Domains", "Total")).toHaveText("0");
    await expect(statValue(page, "Domains", "Active")).toHaveText("0");
    await expect(statValue(page, "Domains", "Unverified")).toHaveText("0");

    // Webhooks card
    await expect(statValue(page, "Webhooks", "Configured")).toHaveText("0");

    // Events card — all zeroes
    await expect(eventCount(page, "accepted")).toHaveText("0");
    await expect(eventCount(page, "delivered")).toHaveText("0");
    await expect(eventCount(page, "failed")).toHaveText("0");
    await expect(eventCount(page, "opened")).toHaveText("0");
    await expect(eventCount(page, "clicked")).toHaveText("0");
    await expect(eventCount(page, "complained")).toHaveText("0");
    await expect(eventCount(page, "unsubscribed")).toHaveText("0");

    // No recent deliveries
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  test("shows message statistics", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Send 2 messages
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Dashboard msg 1",
      text: "hello",
    });
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Dashboard msg 2",
      text: "hello again",
    });

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    await expect(statValue(page, "Messages", "Total")).toHaveText("2");
    await expect(statValue(page, "Messages", "Last Hour")).toHaveText("2");
  });

  test("shows domain statistics", async ({ page, api }) => {
    // Create 2 domains with auto_verify on (default) — they become "active"
    await api.createDomain("active1.example.com");
    await api.createDomain("active2.example.com");

    // Turn off auto_verify so the next domain is "unverified"
    await api.updateConfig({
      domain_behavior: { domain_auto_verify: false },
    });
    await api.createDomain("unverified1.example.com");

    // Restore auto_verify for other tests
    await api.updateConfig({
      domain_behavior: { domain_auto_verify: true },
    });

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    await expect(statValue(page, "Domains", "Total")).toHaveText("3");
    await expect(statValue(page, "Domains", "Active")).toHaveText("2");
    await expect(statValue(page, "Domains", "Unverified")).toHaveText("1");
  });

  test("shows webhook count", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.createWebhook(DOMAIN, {
      id: "delivered",
      url: "http://example.com/hook1",
    });
    await api.createWebhook(DOMAIN, {
      id: "opened",
      url: "http://example.com/hook2",
    });

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    await expect(statValue(page, "Webhooks", "Configured")).toHaveText("2");
  });

  test("shows event statistics", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Send a message — auto-generates "accepted" and "delivered" events
    const sendResult = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Event stats test",
      text: "testing events",
    });
    const storageKey = extractStorageKey(sendResult);

    // Trigger additional event types
    await api.triggerEvent(DOMAIN, "fail", storageKey);
    await api.triggerEvent(DOMAIN, "open", storageKey);
    await api.triggerEvent(DOMAIN, "click", storageKey);
    await api.triggerEvent(DOMAIN, "unsubscribe", storageKey);
    await api.triggerEvent(DOMAIN, "complain", storageKey);

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    // 1 accepted (auto from send), 1 delivered (auto from send)
    await expect(eventCount(page, "accepted")).toHaveText("1");
    await expect(eventCount(page, "delivered")).toHaveText("1");
    await expect(eventCount(page, "failed")).toHaveText("1");
    await expect(eventCount(page, "opened")).toHaveText("1");
    await expect(eventCount(page, "clicked")).toHaveText("1");
    await expect(eventCount(page, "complained")).toHaveText("1");
    await expect(eventCount(page, "unsubscribed")).toHaveText("1");
  });

  test("shows recent webhook deliveries table", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Create a webhook for "delivered" events
    await api.createWebhook(DOMAIN, {
      id: "delivered",
      url: "http://localhost:9999/test-hook",
    });

    // Send a message so we have a message_id to reference
    const sendResult = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Webhook delivery test",
      text: "testing webhook deliveries",
    });
    const messageId = sendResult.id as string;

    // Explicitly trigger the webhook delivery via the mock API
    await api.triggerWebhook({
      domain: DOMAIN,
      event_type: "delivered",
      recipient: `recipient@${DOMAIN}`,
      message_id: messageId,
    });

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    // Check that the Recent Webhook Deliveries table has data
    const deliveriesCard = page.locator(".card", {
      has: page.locator('h2.card-title:text-is("Recent Webhook Deliveries")'),
    });

    // There should be at least one row with the webhook URL
    const table = deliveriesCard.locator("table");
    await expect(table).toBeVisible();

    // Verify the webhook URL appears in the table
    await expect(table.getByText("http://localhost:9999/test-hook")).toBeVisible();

    // Verify an event type column value is present
    await expect(table.getByText("delivered")).toBeVisible();
  });

  test("auto-refreshes on WebSocket message.new", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    // Verify initial zero state for messages
    await expect(statValue(page, "Messages", "Total")).toHaveText("0");

    // Send a message while on the dashboard page — the WebSocket should
    // trigger an automatic refresh
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "WS auto-refresh test",
      text: "hello websocket",
    });

    // Wait for the dashboard to auto-refresh via WebSocket
    await expect(statValue(page, "Messages", "Total")).toHaveText("1", {
      timeout: 10000,
    });

    // Events should also update (accepted + delivered from auto_deliver)
    await expect(eventCount(page, "accepted")).toHaveText("1", {
      timeout: 5000,
    });
  });

  test("auto-refreshes on WebSocket data.reset", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Populate some data
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Reset test",
      text: "will be reset",
    });

    await page.goto("/");
    await expect(page.locator("main h1")).toHaveText("Dashboard");

    // Verify non-zero counts
    await expect(statValue(page, "Messages", "Total")).toHaveText("1");
    await expect(statValue(page, "Domains", "Total")).toHaveText("1");

    // Reset all data via API — this should broadcast data.reset via WebSocket
    await api.reset();

    // Wait for the dashboard to auto-refresh and show zero counts
    await expect(statValue(page, "Messages", "Total")).toHaveText("0", {
      timeout: 10000,
    });
    await expect(statValue(page, "Domains", "Total")).toHaveText("0", {
      timeout: 5000,
    });
    await expect(eventCount(page, "accepted")).toHaveText("0", {
      timeout: 5000,
    });
  });
});
