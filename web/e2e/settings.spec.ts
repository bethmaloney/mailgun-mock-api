import { test, expect } from "./fixtures";

test.describe("Settings Page", () => {
  test("loads current configuration — all settings sections display current values", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator("main h1")).toHaveText("Settings");

    // Verify loading state clears and sections appear
    await expect(page.getByText("Loading configuration...")).not.toBeVisible();

    // All five section headings are visible
    await expect(page.getByRole("heading", { name: "Event Generation" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Domain Behavior" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Webhook Delivery" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Authentication" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Storage" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Data Reset" })).toBeVisible();

    // Verify default values are populated
    await expect(page.locator("#auto-deliver")).toBeChecked();
    await expect(page.locator("#delivery-delay")).toHaveValue("0");
    await expect(page.locator("#default-status-code")).toHaveValue("250");
    await expect(page.locator("#auto-fail-rate")).toHaveValue("0");
    await expect(page.locator("#domain-auto-verify")).toBeChecked();
    await expect(page.locator("#webhook-retry-mode")).toHaveValue("immediate");
    await expect(page.locator("#webhook-timeout")).toHaveValue("5000");
    await expect(page.locator("#auth-mode")).toHaveValue("accept_any");
    await expect(page.locator("#store-attachments")).not.toBeChecked();
    await expect(page.locator("#max-messages")).toHaveValue("0");
    await expect(page.locator("#max-events")).toHaveValue("0");
  });

  test("Event Generation — save settings", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Event Generation" })).toBeVisible();

    // Change settings
    await page.locator("#auto-deliver").uncheck();
    await page.locator("#delivery-delay").fill("500");
    await page.locator("#default-status-code").fill("200");
    await page.locator("#auto-fail-rate").fill("0.25");

    // Save
    await page.getByRole("button", { name: "Save Event Generation" }).click();

    // Verify success message
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("Event generation settings saved");

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole("heading", { name: "Event Generation" })).toBeVisible();
    await expect(page.locator("#auto-deliver")).not.toBeChecked();
    await expect(page.locator("#delivery-delay")).toHaveValue("500");
    await expect(page.locator("#default-status-code")).toHaveValue("200");
    await expect(page.locator("#auto-fail-rate")).toHaveValue("0.25");
  });

  test("Domain Behavior — save settings", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Domain Behavior" })).toBeVisible();

    // Toggle and change values
    await page.locator("#domain-auto-verify").uncheck();
    await page.locator("#sandbox-domain").fill("my-sandbox.mailgun.org");

    // Save
    await page.getByRole("button", { name: "Save Domain Behavior" }).click();

    // Verify success
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("Domain behavior settings saved");

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole("heading", { name: "Domain Behavior" })).toBeVisible();
    await expect(page.locator("#domain-auto-verify")).not.toBeChecked();
    await expect(page.locator("#sandbox-domain")).toHaveValue("my-sandbox.mailgun.org");
  });

  test("Webhook Delivery — save settings", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Webhook Delivery" })).toBeVisible();

    // Change settings
    await page.locator("#webhook-retry-mode").selectOption("realistic");
    await page.locator("#webhook-timeout").fill("10000");

    // Save
    await page.getByRole("button", { name: "Save Webhook Delivery" }).click();

    // Verify success
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("Webhook delivery settings saved");

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole("heading", { name: "Webhook Delivery" })).toBeVisible();
    await expect(page.locator("#webhook-retry-mode")).toHaveValue("realistic");
    await expect(page.locator("#webhook-timeout")).toHaveValue("10000");
  });

  test("Authentication — save settings and signing key is read-only", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Authentication" })).toBeVisible();

    // Verify signing key is displayed as a read-only span (not an input)
    const signingKey = page.locator("span.mono.readonly-value");
    await expect(signingKey).toBeVisible();
    // Signing key should not be an input element
    const inputCount = await page.locator("#signing-key").count();
    expect(inputCount).toBe(0);

    // Change auth mode
    await page.locator("#auth-mode").selectOption("validate");

    // Save
    await page.getByRole("button", { name: "Save Authentication" }).click();

    // Verify success
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("Authentication settings saved");

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole("heading", { name: "Authentication" })).toBeVisible();
    await expect(page.locator("#auth-mode")).toHaveValue("validate");
  });

  test("Storage — save settings", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Storage" })).toBeVisible();

    // Change settings
    await page.locator("#store-attachments").check();
    await page.locator("#max-messages").fill("1000");
    await page.locator("#max-events").fill("5000");

    // Save
    await page.getByRole("button", { name: "Save Storage" }).click();

    // Verify success
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("Storage settings saved");

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole("heading", { name: "Storage" })).toBeVisible();
    await expect(page.locator("#store-attachments")).toBeChecked();
    await expect(page.locator("#max-messages")).toHaveValue("1000");
    await expect(page.locator("#max-events")).toHaveValue("5000");
  });

  test("success message auto-hides after ~3 seconds", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Event Generation" })).toBeVisible();

    // Save to trigger success message
    await page.getByRole("button", { name: "Save Event Generation" }).click();

    // Success message should appear
    await expect(page.locator(".success")).toBeVisible();

    // Wait for it to auto-hide (3000ms timer + buffer)
    await expect(page.locator(".success")).not.toBeVisible({ timeout: 5000 });
  });

  test("Reset All Data — click reset, confirm, verify success", async ({ page, api }) => {
    // Create some data first
    await api.createDomain("reset-test.example.com");

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Data Reset" })).toBeVisible();

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Click Reset All Data
    await page.getByRole("button", { name: "Reset All Data" }).click();

    // Verify success message
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("reset");
  });

  test("Reset Messages & Events — click reset, confirm, verify success", async ({ page, api }) => {
    // Create a domain and send a message first
    await api.createDomain("msg-reset.example.com");
    await api.sendMessage("msg-reset.example.com", {
      from: "test@msg-reset.example.com",
      to: "user@example.com",
      subject: "Test message",
      text: "Hello",
    });

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Data Reset" })).toBeVisible();

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Click Reset Messages & Events
    await page.getByRole("button", { name: "Reset Messages & Events" }).click();

    // Verify success message
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("reset");
  });

  test("Reset Per Domain — select domain, click reset, confirm, verify success", async ({ page, api }) => {
    // Create a domain first
    await api.createDomain("domain-reset.example.com");

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Data Reset" })).toBeVisible();

    // Wait for the domain dropdown to be populated
    const domainSelect = page.locator(".reset-domain-controls select");
    await expect(domainSelect).toBeEnabled();
    await domainSelect.selectOption("domain-reset.example.com");

    // Accept the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());

    // Click Reset Domain
    await page.getByRole("button", { name: "Reset Domain" }).click();

    // Verify success message
    await expect(page.locator(".success")).toBeVisible();
    await expect(page.locator(".success")).toContainText("reset");
  });

  test("Cancel reset confirmation — no reset occurs", async ({ page, api }) => {
    await api.createDomain("cancel-test.example.com");

    await page.goto("/settings");
    await expect(page.getByRole("heading", { name: "Data Reset" })).toBeVisible();

    // Dismiss the confirm dialog (cancel)
    page.on("dialog", (dialog) => dialog.dismiss());

    // Click Reset All Data — but cancel
    await page.getByRole("button", { name: "Reset All Data" }).click();

    // No success message should appear
    await expect(page.locator(".success")).not.toBeVisible();

    // The page should still show settings normally (no error either)
    await expect(page.locator(".error")).not.toBeVisible();
    await expect(page.getByRole("heading", { name: "Event Generation" })).toBeVisible();
  });
});
