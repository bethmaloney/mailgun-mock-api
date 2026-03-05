import { test, expect } from "./fixtures";

const DOMAIN = "msg-e2e.example.com";

/**
 * Extract the storage key from a sendMessage response.
 * The response id looks like "<timestamp.hex@domain>" — the storage key
 * is the part inside the angle brackets.
 */
function extractStorageKey(sendResponse: Record<string, unknown>): string {
  const id = sendResponse.id as string;
  return id.replace(/^</, "").replace(/>$/, "");
}

test.describe("Messages Page", () => {
  // 1. Shows empty state
  test("shows empty state when no messages", async ({ page }) => {
    await page.goto("/messages");
    await expect(page.locator("main h1")).toHaveText("Messages");
    await expect(page.getByText("0 total")).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  // 2. Lists messages after sending
  test("lists messages after sending via API", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `alice@${DOMAIN}`,
      to: `bob@${DOMAIN}`,
      subject: "Hello from E2E",
      text: "Test body",
    });
    await api.sendMessage(DOMAIN, {
      from: `carol@${DOMAIN}`,
      to: `dave@${DOMAIN}`,
      subject: "Second message",
      text: "Another body",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    // Verify table contains expected data
    const table = page.locator("table.data-table");
    await expect(table.getByText(`alice@${DOMAIN}`)).toBeVisible();
    await expect(table.getByText(`bob@${DOMAIN}`)).toBeVisible();
    await expect(table.getByText("Hello from E2E")).toBeVisible();
    await expect(table.getByText(`carol@${DOMAIN}`)).toBeVisible();
    await expect(table.getByText(`dave@${DOMAIN}`)).toBeVisible();
    await expect(table.getByText("Second message")).toBeVisible();
    // Domain column should show the domain name
    await expect(table.getByRole("cell", { name: DOMAIN, exact: true }).first()).toBeVisible();

    // Status badges should be present (accepted or delivered)
    await expect(table.locator(".status-badge").first()).toBeVisible();
  });

  // 3. Shows tags in table
  test("shows tags in table", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessageWithExtras(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Tagged message",
      text: "Has tags",
      tags: ["welcome", "onboarding"],
    });

    await page.goto("/messages");
    await expect(page.getByText("1 total")).toBeVisible();

    const table = page.locator("table.data-table");
    // Tags are displayed as comma-separated in the table
    await expect(table.getByText("welcome")).toBeVisible();
    await expect(table.getByText("onboarding")).toBeVisible();
  });

  // 4. Filter by domain
  test("filter by domain", async ({ page, api }) => {
    const domain1 = "alpha.example.com";
    const domain2 = "beta.example.com";
    await api.createDomain(domain1);
    await api.createDomain(domain2);

    await api.sendMessage(domain1, {
      from: `sender@${domain1}`,
      to: `recipient@${domain1}`,
      subject: "Alpha message",
      text: "From alpha",
    });
    await api.sendMessage(domain2, {
      from: `sender@${domain2}`,
      to: `recipient@${domain2}`,
      subject: "Beta message",
      text: "From beta",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    // Filter by domain1
    await page.getByPlaceholder("Domain").fill(domain1);
    await page.getByRole("button", { name: "Filter" }).click();

    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("Alpha message")).toBeVisible();
    await expect(page.getByText("Beta message")).not.toBeVisible();
  });

  // 5. Filter by from
  test("filter by from", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `alice@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Alice msg",
      text: "From alice",
    });
    await api.sendMessage(DOMAIN, {
      from: `bob@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Bob msg",
      text: "From bob",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    await page.getByPlaceholder("From").fill("alice");
    await page.getByRole("button", { name: "Filter" }).click();

    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("Alice msg")).toBeVisible();
    await expect(page.getByText("Bob msg")).not.toBeVisible();
  });

  // 6. Filter by to
  test("filter by to", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `user1@${DOMAIN}`,
      subject: "To user1",
      text: "Msg for user1",
    });
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `user2@${DOMAIN}`,
      subject: "To user2",
      text: "Msg for user2",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    await page.getByPlaceholder("To").fill("user1");
    await page.getByRole("button", { name: "Filter" }).click();

    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("To user1")).toBeVisible();
    await expect(page.getByText("To user2")).not.toBeVisible();
  });

  // 7. Filter by subject
  test("filter by subject", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Welcome aboard",
      text: "Welcome!",
    });
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Invoice ready",
      text: "Invoice attached",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    await page.getByPlaceholder("Subject").fill("Welcome");
    await page.getByRole("button", { name: "Filter" }).click();

    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("Welcome aboard")).toBeVisible();
    await expect(page.getByText("Invoice ready")).not.toBeVisible();
  });

  // 8. Combined filters
  test("combined filters", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `alice@${DOMAIN}`,
      to: `bob@${DOMAIN}`,
      subject: "Meeting notes",
      text: "Notes from today",
    });
    await api.sendMessage(DOMAIN, {
      from: `alice@${DOMAIN}`,
      to: `carol@${DOMAIN}`,
      subject: "Lunch plans",
      text: "Where to eat",
    });
    await api.sendMessage(DOMAIN, {
      from: `dave@${DOMAIN}`,
      to: `bob@${DOMAIN}`,
      subject: "Meeting agenda",
      text: "Tomorrow's agenda",
    });

    await page.goto("/messages");
    await expect(page.getByText("3 total")).toBeVisible();

    // Filter by from=alice and subject=Meeting
    await page.getByPlaceholder("From").fill("alice");
    await page.getByPlaceholder("Subject").fill("Meeting");
    await page.getByRole("button", { name: "Filter" }).click();

    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("Meeting notes")).toBeVisible();
    await expect(page.getByText("Lunch plans")).not.toBeVisible();
    await expect(page.getByText("Meeting agenda")).not.toBeVisible();
  });

  // 9. Pagination
  test("pagination works with next and previous buttons", async ({ page, api }) => {
    test.setTimeout(60000);
    await api.createDomain(DOMAIN);

    // Send 5 messages. We intercept API requests to add limit=3 so the
    // default page size of 50 is reduced, allowing pagination with fewer messages.
    for (let i = 1; i <= 5; i++) {
      await api.sendMessage(DOMAIN, {
        from: `sender@${DOMAIN}`,
        to: `recipient@${DOMAIN}`,
        subject: `Paginated msg ${String(i).padStart(3, "0")}`,
        text: `Body ${i}`,
      });
    }

    // Intercept /mock/messages API calls and add limit=3 to force pagination
    await page.route("**/mock/messages?**", (route) => {
      const url = new URL(route.request().url());
      url.searchParams.set("limit", "3");
      route.continue({ url: url.toString() });
    });
    await page.route("**/mock/messages", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("limit")) {
        url.searchParams.set("limit", "3");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/messages");
    await expect(page.getByText("5 total")).toBeVisible({ timeout: 10000 });

    // Should show first page with 3 rows (limit=3)
    const table = page.locator("table.data-table");
    const rows = table.locator("tbody tr");
    await expect(rows).toHaveCount(3);

    // Next button should be enabled, Previous should be disabled
    const nextBtn = page.getByRole("button", { name: "Next" });
    const prevBtn = page.getByRole("button", { name: "Previous" });
    await expect(nextBtn).toBeEnabled();
    await expect(prevBtn).toBeDisabled();

    // Go to next page
    await nextBtn.click();

    // Should show remaining 2 messages
    await expect(rows).toHaveCount(2, { timeout: 10000 });
    await expect(prevBtn).toBeEnabled();
    await expect(nextBtn).toBeDisabled();

    // Go back to previous page
    await prevBtn.click();
    await expect(rows).toHaveCount(3, { timeout: 10000 });
    await expect(prevBtn).toBeDisabled();
    await expect(nextBtn).toBeEnabled();
  });

  // 10. View message detail
  test("view message detail", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    const result = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Detail test message",
      text: "Detail body",
    });
    const storageKey = extractStorageKey(result);

    await page.goto("/messages");
    await expect(page.getByText("1 total")).toBeVisible();

    // Click subject to open detail
    await page.getByRole("link", { name: "Detail test message" }).click();

    // Detail panel should appear
    const detail = page.locator(".detail-panel");
    await expect(detail).toBeVisible();
    await expect(detail.getByText("Message Detail")).toBeVisible();

    // Check detail fields
    await expect(detail.locator(".detail-field", { has: page.locator("label:text-is('From')") })).toContainText(`sender@${DOMAIN}`);
    await expect(detail.locator(".detail-field", { has: page.locator("label:text-is('To')") })).toContainText(`recipient@${DOMAIN}`);
    await expect(detail.locator(".detail-field", { has: page.locator("label:text-is('Subject')") })).toContainText("Detail test message");
    await expect(detail.locator(".detail-field", { has: page.locator("label:text-is('Domain')") })).toContainText(DOMAIN);
    await expect(detail.locator(".detail-field", { has: page.locator("label:text-is('Message ID')") })).toBeVisible();
    await expect(detail.locator(".detail-field", { has: page.locator("label:text-is('Storage Key')") })).toContainText(storageKey);
  });

  // 11. Detail shows text body
  test("detail shows text body", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Text body test",
      text: "This is the plain text body content.",
    });

    await page.goto("/messages");
    await page.getByRole("link", { name: "Text body test" }).click();

    const detail = page.locator(".detail-panel");
    await expect(detail.getByRole("heading", { name: "Text Body" })).toBeVisible();
    await expect(detail.locator("pre.body-content")).toContainText("This is the plain text body content.");
  });

  // 12. Detail shows HTML body in iframe
  test("detail shows HTML body in iframe", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "HTML body test",
      html: "<h1>Hello World</h1><p>This is HTML content</p>",
    });

    await page.goto("/messages");
    await page.getByRole("link", { name: "HTML body test" }).click();

    const detail = page.locator(".detail-panel");
    await expect(detail.getByRole("heading", { name: "HTML Body" })).toBeVisible();
    await expect(detail.locator("iframe.html-preview")).toBeVisible();

    // Verify the iframe has the srcdoc attribute set
    const srcdoc = await detail.locator("iframe.html-preview").getAttribute("srcdoc");
    expect(srcdoc).toContain("Hello World");
  });

  // 13. Detail shows custom headers
  test("detail shows custom headers", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessageWithExtras(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Custom headers test",
      text: "Has custom headers",
      headers: { "X-My-Header": "custom-value-123" },
    });

    await page.goto("/messages");
    await page.getByRole("link", { name: "Custom headers test" }).click();

    const detail = page.locator(".detail-panel");
    await expect(detail.getByRole("heading", { name: "Custom Headers" })).toBeVisible();
    // Locate the pre block after the Custom Headers heading
    const headersSection = detail.locator(".detail-section", { has: page.getByRole("heading", { name: "Custom Headers" }) });
    await expect(headersSection.locator("pre.body-content")).toContainText("X-My-Header");
    await expect(headersSection.locator("pre.body-content")).toContainText("custom-value-123");
  });

  // 14. Detail shows custom variables
  test("detail shows custom variables", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessageWithExtras(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Custom vars test",
      text: "Has custom variables",
      variables: { "my-var": "var-value-456" },
    });

    await page.goto("/messages");
    await page.getByRole("link", { name: "Custom vars test" }).click();

    const detail = page.locator(".detail-panel");
    await expect(detail.getByRole("heading", { name: "Custom Variables" })).toBeVisible();
    const varsSection = detail.locator(".detail-section", { has: page.getByRole("heading", { name: "Custom Variables" }) });
    await expect(varsSection.locator("pre.body-content")).toContainText("my-var");
    await expect(varsSection.locator("pre.body-content")).toContainText("var-value-456");
  });

  // 15. Detail shows attachments
  test("detail shows attachments", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessageWithAttachment(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Attachment test",
      text: "Has an attachment",
      filename: "test-file.txt",
      contentType: "text/plain",
      content: Buffer.from("Hello, this is a test file."),
    });

    await page.goto("/messages");
    await page.getByRole("link", { name: "Attachment test" }).click();

    const detail = page.locator(".detail-panel");
    await expect(detail.getByText("Attachments")).toBeVisible();

    const attachmentList = detail.locator(".attachment-list");
    await expect(attachmentList.getByText("test-file.txt")).toBeVisible();
    await expect(attachmentList.getByText("text/plain")).toBeVisible();
  });

  // 16. Detail shows events timeline
  test("detail shows events timeline", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    const result = await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Events timeline test",
      text: "Has events",
    });
    const storageKey = extractStorageKey(result);

    // Trigger additional events
    await api.triggerEvent(DOMAIN, "open", storageKey);
    await api.triggerEvent(DOMAIN, "click", storageKey);

    await page.goto("/messages");
    await page.getByRole("link", { name: "Events timeline test" }).click();

    const detail = page.locator(".detail-panel");
    await expect(detail.getByRole("heading", { name: "Events Timeline" })).toBeVisible();

    // Should show status badges for each event type
    const timeline = detail.locator(".timeline");
    // At least 2 events: accepted + delivered from auto, plus open + click triggered
    const badgeCount = await timeline.locator(".status-badge").count();
    expect(badgeCount).toBeGreaterThanOrEqual(2);

    // Check for specific event type badges (accepted and delivered from auto-events)
    await expect(timeline.locator(".status-badge", { hasText: "accepted" })).toBeVisible();
    await expect(timeline.locator(".status-badge", { hasText: "delivered" })).toBeVisible();
  });

  // 17. Close detail by clicking again
  test("close detail by clicking subject again", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Toggle detail test",
      text: "Toggle me",
    });

    await page.goto("/messages");

    // Click subject to open detail
    await page.getByRole("link", { name: "Toggle detail test" }).click();
    await expect(page.locator(".detail-panel")).toBeVisible();
    await expect(page.getByText("Message Detail")).toBeVisible();

    // Click subject again to close detail
    await page.getByRole("link", { name: "Toggle detail test" }).click();
    await expect(page.locator(".detail-panel")).not.toBeVisible();
  });

  // 18. Delete individual message
  test("delete individual message", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Keep this message",
      text: "Keep",
    });
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Delete this message",
      text: "Delete",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Find the row with "Delete this message" and click its delete button
    const row = page.locator("table.data-table tbody tr", { hasText: "Delete this message" });
    await row.locator("button.btn-delete").click();

    // Verify the message is removed
    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("Delete this message")).not.toBeVisible();
    await expect(page.getByText("Keep this message")).toBeVisible();
  });

  // 19. Clear all messages
  test("clear all messages", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Message 1",
      text: "First",
    });
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Message 2",
      text: "Second",
    });

    await page.goto("/messages");
    await expect(page.getByText("2 total")).toBeVisible();

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Click Clear All button
    await page.getByRole("button", { name: "Clear All" }).click();

    // Verify all messages are removed
    await expect(page.getByText("0 total")).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  // 20. Auto-refreshes on WebSocket message.new
  test("auto-refreshes on WebSocket message.new", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    await page.goto("/messages");
    await expect(page.getByText("0 total")).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();

    // Send a message while the page is open — WebSocket should trigger refresh
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "WebSocket live message",
      text: "Arrived via WS",
    });

    // Wait for the page to auto-refresh and show the new message
    await expect(page.getByText("1 total")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("WebSocket live message")).toBeVisible({ timeout: 5000 });
  });
});
