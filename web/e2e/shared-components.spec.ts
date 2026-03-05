import { test, expect } from "./fixtures";

const DOMAIN = "shared-e2e.example.com";

/**
 * Extract the storage key from a sendMessage response.
 * The response id looks like "<timestamp.hex@domain>" — the storage key
 * is the part inside the angle brackets.
 */
function extractStorageKey(sendResponse: Record<string, unknown>): string {
  const id = sendResponse.id as string;
  return id.replace(/^</, "").replace(/>$/, "");
}

test.describe("Shared Components", () => {
  // 1. DataTable loading state — intercept API to delay response, verify "Loading..." shown
  test("DataTable shows loading state during fetch", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Loading state test",
      text: "Testing loading",
    });

    // Intercept the messages API call and delay it so we can observe the loading state
    let resolveDelay: () => void;
    const delayPromise = new Promise<void>((resolve) => {
      resolveDelay = resolve;
    });

    await page.route("**/mock/messages**", async (route) => {
      await delayPromise;
      await route.continue();
    });

    // Navigate — the loading state should appear while the API call is pending
    page.goto("/messages"); // intentionally not awaited

    // The loading indicator should be visible while the request is delayed
    await expect(page.locator(".data-table-loading")).toBeVisible({ timeout: 5000 });
    await expect(page.locator(".data-table-loading")).toHaveText("Loading...");

    // Release the delayed request
    resolveDelay!();

    // After the data loads, the loading indicator should disappear and data should show
    await expect(page.locator(".data-table-loading")).not.toBeVisible({ timeout: 10000 });
    await expect(page.locator("table.data-table")).toBeVisible();
  });

  // 2. DataTable empty state — navigate to messages with no data, verify "No data available."
  test("DataTable shows empty state when no rows", async ({ page, api: _api }) => {
    // The api fixture is referenced to ensure reset() runs before this test
    await page.goto("/messages");
    await expect(page.locator("main h1")).toHaveText("Messages");

    // Should show "0 total" and the empty state text
    await expect(page.getByText("0 total")).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  // 3. StatusBadge colors — trigger various event types, check badge classes and CSS colors
  test("StatusBadge shows correct colors for different event types", async ({
    page,
    api,
  }) => {
    await api.createDomain(DOMAIN);

    const result = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Badge color test",
      text: "Testing badge colors",
    });
    const storageKey = extractStorageKey(result);

    // Trigger events of different types
    await api.triggerEvent(DOMAIN, "open", storageKey);
    await api.triggerEvent(DOMAIN, "fail", storageKey);
    await api.triggerEvent(DOMAIN, "unsubscribe", storageKey);

    await page.goto("/events");

    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();

    // delivered -> badge-success (green: bg #dcfce7, text #166534)
    const deliveredBadge = table
      .locator(".status-badge", { hasText: "delivered" })
      .first();
    await expect(deliveredBadge).toHaveClass(/badge-success/);
    await expect(deliveredBadge).toHaveCSS(
      "background-color",
      "rgb(220, 252, 231)",
    ); // #dcfce7
    await expect(deliveredBadge).toHaveCSS("color", "rgb(22, 101, 52)"); // #166534

    // opened -> badge-info (blue: bg #dbeafe, text #1e40af)
    const openedBadge = table
      .locator(".status-badge", { hasText: "opened" })
      .first();
    await expect(openedBadge).toHaveClass(/badge-info/);
    await expect(openedBadge).toHaveCSS(
      "background-color",
      "rgb(219, 234, 254)",
    ); // #dbeafe
    await expect(openedBadge).toHaveCSS("color", "rgb(30, 64, 175)"); // #1e40af

    // failed -> badge-danger (red: bg #fee2e2, text #991b1b)
    const failedBadge = table
      .locator(".status-badge", { hasText: "failed" })
      .first();
    await expect(failedBadge).toHaveClass(/badge-danger/);
    await expect(failedBadge).toHaveCSS(
      "background-color",
      "rgb(254, 226, 226)",
    ); // #fee2e2
    await expect(failedBadge).toHaveCSS("color", "rgb(153, 27, 27)"); // #991b1b

    // unsubscribed -> badge-warning (orange: bg #fef3c7, text #92400e)
    const unsubBadge = table
      .locator(".status-badge", { hasText: "unsubscribed" })
      .first();
    await expect(unsubBadge).toHaveClass(/badge-warning/);
    await expect(unsubBadge).toHaveCSS(
      "background-color",
      "rgb(254, 243, 199)",
    ); // #fef3c7
    await expect(unsubBadge).toHaveCSS("color", "rgb(146, 64, 14)"); // #92400e
  });

  // 4. Pagination disabled states — Previous disabled on first page, Next disabled on last page
  test("Pagination buttons have correct disabled states", async ({
    page,
    api,
  }) => {
    test.setTimeout(60000);
    await api.createDomain(DOMAIN);

    // First: with only 1 message, both buttons should be disabled (fits on one page)
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Only message",
      text: "Single message",
    });

    await page.goto("/messages");
    await expect(page.getByText("1 total")).toBeVisible();

    const prevBtn = page.getByRole("button", { name: "Previous" });
    const nextBtn = page.getByRole("button", { name: "Next" });

    // With 1 message (less than page size), both should be disabled
    await expect(prevBtn).toBeDisabled();
    await expect(nextBtn).toBeDisabled();

    // Now add enough messages to force pagination (send 4 more = 5 total)
    for (let i = 2; i <= 5; i++) {
      await api.sendMessage(DOMAIN, {
        from: `sender@${DOMAIN}`,
        to: `recipient@${DOMAIN}`,
        subject: `Pagination msg ${i}`,
        text: `Body ${i}`,
      });
    }

    // Intercept API calls to set limit=3 to force 2 pages with 5 messages
    await page.route("**/mock/messages**", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("limit")) {
        url.searchParams.set("limit", "3");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/messages");
    await expect(page.getByText("5 total")).toBeVisible({ timeout: 10000 });

    const rows = page.locator("table.data-table tbody tr");
    await expect(rows).toHaveCount(3, { timeout: 10000 });

    // On first page: Previous disabled, Next enabled
    await expect(prevBtn).toBeDisabled();
    await expect(nextBtn).toBeEnabled();

    // Navigate to last page
    await nextBtn.click();
    await expect(rows).toHaveCount(2, { timeout: 10000 });

    // On last page: Previous enabled, Next disabled
    await expect(prevBtn).toBeEnabled();
    await expect(nextBtn).toBeDisabled();
  });

  // 5. WebSocket connection indicator — verify green dot when connected
  test("WebSocket connection indicator shows connected state", async ({
    page,
  }) => {
    await page.goto("/messages");

    // The sidebar connection status should show "Connected" after WebSocket connects
    const connectionStatus = page.locator(".connection-status");
    await expect(connectionStatus).toBeVisible();

    // Wait for WebSocket to connect — the indicator should become green
    await expect(connectionStatus).toHaveClass(/is-connected/, {
      timeout: 10000,
    });

    // Verify the text says "Connected"
    const statusText = connectionStatus.locator(".status-text");
    await expect(statusText).toHaveText("Connected");

    // Verify the dot is green (#22c55e -> rgb(34, 197, 94))
    const statusDot = connectionStatus.locator(".status-dot");
    await expect(statusDot).toHaveCSS(
      "background-color",
      "rgb(34, 197, 94)",
    );
  });
});
