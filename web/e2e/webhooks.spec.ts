import { test, expect } from "./fixtures";

const DOMAIN = "webhook-test.example.com";

test.describe("Webhooks", () => {
  test("shows empty state — no webhooks configured, no delivery log", async ({ page, api: _api }) => {
    // _api triggers the fixture's reset() so this test sees an empty DB.
    await page.goto("/webhooks");
    await expect(page.getByRole("heading", { name: "Webhooks" })).toBeVisible();

    // No domains — info message shown
    await expect(
      page.getByText("No domains configured. Create a domain first to manage webhooks.")
    ).toBeVisible();

    // Delivery log section shows 0 total
    await expect(page.getByText("0 total")).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  test("domain selector populated — verify domains in dropdown", async ({ page, api }) => {
    await api.createDomain("alpha.example.com");
    await api.createDomain("beta.example.com");
    await page.goto("/webhooks");

    // Domain selector should be visible
    const select = page.locator("#domain-select");
    await expect(select).toBeVisible();

    // Both domains should be options
    await expect(select.locator("option", { hasText: "alpha.example.com" })).toBeAttached();
    await expect(select.locator("option", { hasText: "beta.example.com" })).toBeAttached();

    // Info message should NOT be shown when domains exist
    await expect(
      page.getByText("No domains configured. Create a domain first to manage webhooks.")
    ).not.toBeVisible();
  });

  test("create webhook — select event type, enter URL, submit, verify appears in table", async ({
    page,
    api,
  }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/webhooks");

    // Wait for domain to be selected and form to appear
    await expect(page.getByPlaceholder("https://example.com/webhook")).toBeVisible();

    // Create button should be disabled when URL is empty
    await expect(page.getByRole("button", { name: "Create" })).toBeDisabled();

    // Select "accepted" event type
    const eventSelect = page.locator(".add-webhook-form select");
    await eventSelect.selectOption("accepted");

    // Enter URL
    await page.getByPlaceholder("https://example.com/webhook").fill("https://hooks.test.com/accepted");

    // Create button should now be enabled
    await expect(page.getByRole("button", { name: "Create" })).toBeEnabled();

    // Submit
    await page.getByRole("button", { name: "Create" }).click();

    // Verify webhook appears in the table
    await expect(page.getByText("https://hooks.test.com/accepted")).toBeVisible();

    // URL input should be cleared after creation
    await expect(page.getByPlaceholder("https://example.com/webhook")).toHaveValue("");
  });

  test("webhook table shows event type badge and URLs", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createWebhook(DOMAIN, { id: "delivered", url: "https://hooks.test.com/delivered" });
    await api.createWebhook(DOMAIN, { id: "accepted", url: "https://hooks.test.com/accepted" });
    await page.goto("/webhooks");

    // Verify webhook URLs are visible in the table
    await expect(page.getByText("https://hooks.test.com/delivered")).toBeVisible();
    await expect(page.getByText("https://hooks.test.com/accepted")).toBeVisible();

    // Event type badges should be present (StatusBadge renders the text)
    await expect(page.locator(".status-badge", { hasText: "delivered" })).toBeVisible();
    await expect(page.locator(".status-badge", { hasText: "accepted" })).toBeVisible();

    // URLs should be in mono font
    await expect(page.locator(".mono", { hasText: "https://hooks.test.com/delivered" })).toBeVisible();
  });

  test("delete webhook — click delete, verify removed", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createWebhook(DOMAIN, { id: "delivered", url: "https://hooks.test.com/to-delete" });
    await page.goto("/webhooks");

    // Verify webhook URL is visible
    await expect(page.getByText("https://hooks.test.com/to-delete")).toBeVisible();

    // Click delete — no confirm dialog for webhooks
    await page.locator("button.btn-danger.btn-sm", { hasText: "Delete" }).first().click();

    // Webhook should be removed
    await expect(page.getByText("https://hooks.test.com/to-delete")).not.toBeVisible();
  });

  test("delivery log shows entries — create webhook, trigger, verify log entry", async ({
    page,
    api,
  }) => {
    await api.createDomain(DOMAIN);
    await api.createWebhook(DOMAIN, { id: "delivered", url: "https://hooks.test.com/delivery" });
    await api.triggerWebhook({ domain: DOMAIN, event_type: "delivered" });

    await page.goto("/webhooks");

    // Delivery log should show 1 total
    await expect(page.getByText("1 total")).toBeVisible();

    // Delivery entry should show the URL in the delivery log table
    const deliveryTable = page.locator(".card-section").nth(1).locator("table");
    await expect(deliveryTable.getByText("https://hooks.test.com/delivery")).toBeVisible();

    // Event type badge in delivery log (there are two 'delivered' badges per row: event type + status)
    await expect(deliveryTable.locator(".status-badge", { hasText: "delivered" }).first()).toBeVisible();
  });

  test("expand delivery detail — click timestamp, verify request/response JSON", async ({
    page,
    api,
  }) => {
    await api.createDomain(DOMAIN);
    await api.createWebhook(DOMAIN, { id: "delivered", url: "https://hooks.test.com/detail" });
    await api.triggerWebhook({ domain: DOMAIN, event_type: "delivered" });

    await page.goto("/webhooks");

    // Wait for delivery log to load
    await expect(page.getByText("1 total")).toBeVisible();

    // Click the timestamp link to expand
    await page.locator("a.delivery-link").first().click();

    // Delivery detail section should appear
    await expect(page.getByRole("heading", { name: "Delivery Detail" })).toBeVisible();

    // Detail fields should be visible
    const detailSection = page.locator(".delivery-detail");
    await expect(detailSection.getByText("Webhook ID")).toBeVisible();

    // Request and Response JSON sections
    await expect(page.getByRole("heading", { name: "Request" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Response" })).toBeVisible();

    // JSON blocks should be present
    const jsonBlocks = page.locator("pre.detail-json");
    await expect(jsonBlocks).toHaveCount(2);
  });

  test("collapse delivery detail — click again to collapse", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createWebhook(DOMAIN, { id: "delivered", url: "https://hooks.test.com/collapse" });
    await api.triggerWebhook({ domain: DOMAIN, event_type: "delivered" });

    await page.goto("/webhooks");
    await expect(page.getByText("1 total")).toBeVisible();

    // Expand
    await page.locator("a.delivery-link").first().click();
    await expect(page.getByRole("heading", { name: "Delivery Detail" })).toBeVisible();

    // Click timestamp again to collapse
    await page.locator("a.delivery-link").first().click();

    // Detail should be hidden
    await expect(page.getByRole("heading", { name: "Delivery Detail" })).not.toBeVisible();
    await expect(page.locator("pre.detail-json")).toHaveCount(0);
  });

  test("delivery log pagination — verify pagination with many deliveries", async ({
    page,
    api,
  }) => {
    await api.createDomain(DOMAIN);
    await api.createWebhook(DOMAIN, { id: "delivered", url: "https://hooks.test.com/paginate" });

    // Trigger 25 deliveries sequentially to avoid SQLite concurrency issues
    for (let i = 0; i < 25; i++) {
      await api.triggerWebhook({ domain: DOMAIN, event_type: "delivered" });
    }

    await page.goto("/webhooks");

    // Should show 25 total
    await expect(page.getByText("25 total")).toBeVisible();

    // Previous should be disabled on the first page
    await expect(page.getByRole("button", { name: "Previous" })).toBeDisabled();

    // Next should be enabled (more than one page)
    await expect(page.getByRole("button", { name: "Next" })).toBeEnabled();

    // Click Next
    await page.getByRole("button", { name: "Next" }).click();

    // After going to second page, Previous should be enabled
    await expect(page.getByRole("button", { name: "Previous" })).toBeEnabled();
  });
});
