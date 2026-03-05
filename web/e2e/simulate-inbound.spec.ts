import { test, expect } from "./fixtures";

const DOMAIN = "inbound-e2e.example.com";

test.describe("Simulate Inbound Page", () => {
  // 1. Form shows required fields — from, to, subject visible, submit button disabled
  test("form shows required fields when domain exists", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/simulate-inbound");

    await expect(page.locator("main h1")).toHaveText("Simulate Inbound");

    // Wait for domain to auto-select
    const domainSelect = page.locator("#inbound-domain-select");
    await expect(domainSelect).toHaveValue(DOMAIN);

    // Required form fields should be visible
    await expect(page.locator("#inbound-from")).toBeVisible();
    await expect(page.locator("#inbound-to")).toBeVisible();
    await expect(page.locator("#inbound-subject")).toBeVisible();

    // Check placeholders
    await expect(page.locator("#inbound-from")).toHaveAttribute("placeholder", "sender@example.com");
    await expect(page.locator("#inbound-subject")).toHaveAttribute("placeholder", "Email subject line");

    // Submit button should be visible but disabled (from and subject are empty)
    const submitBtn = page.locator("button.btn-send");
    await expect(submitBtn).toBeVisible();
    await expect(submitBtn).toHaveText("Simulate Inbound");
    await expect(submitBtn).toBeDisabled();
  });

  // 2. Domain selector auto-populates To field
  test("domain selector auto-populates To field", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/simulate-inbound");

    // Wait for domain to auto-select
    const domainSelect = page.locator("#inbound-domain-select");
    await expect(domainSelect).toHaveValue(DOMAIN);

    // To field should be auto-populated with recipient@{domain}
    const toField = page.locator("#inbound-to");
    await expect(toField).toHaveValue(`recipient@${DOMAIN}`);
  });

  // 3. Submit button disabled until required fields filled
  test("submit button disabled until required fields filled", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/simulate-inbound");

    const submitBtn = page.locator("button.btn-send");
    await expect(page.locator("#inbound-domain-select")).toHaveValue(DOMAIN);

    // To is auto-populated, but from and subject are empty — button should be disabled
    await expect(submitBtn).toBeDisabled();

    // Fill only from — still disabled (subject empty)
    await page.locator("#inbound-from").fill("sender@example.com");
    await expect(submitBtn).toBeDisabled();

    // Fill subject — now all required fields are filled, button should be enabled
    await page.locator("#inbound-subject").fill("Test subject");
    await expect(submitBtn).toBeEnabled();
  });

  // 4. Simulate inbound message — fill form, submit, verify result panel
  test("simulate inbound message shows result panel", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/simulate-inbound");

    // Wait for domain auto-select
    await expect(page.locator("#inbound-domain-select")).toHaveValue(DOMAIN);

    // Fill required fields
    await page.locator("#inbound-from").fill("sender@example.com");
    await page.locator("#inbound-subject").fill("Test inbound subject");
    await page.locator("#inbound-body-plain").fill("Plain text body");

    // Submit
    const submitBtn = page.locator("button.btn-send");
    await expect(submitBtn).toBeEnabled();
    await submitBtn.click();

    // Result panel should appear
    await expect(page.locator("h2", { hasText: "Simulation Result" })).toBeVisible();

    // Message label should show response message
    await expect(page.getByText("Inbound message processed")).toBeVisible();
  });

  // 5. Result shows matched routes — create route via API, simulate, verify matched routes
  test("result shows matched routes", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Create a route that matches the recipient
    await api.createRoute({
      priority: 0,
      expression: `match_recipient(".*@${DOMAIN}")`,
      action: ["forward(\"https://example.com/webhook\")", "stop()"],
      description: "E2E test route",
    });

    await page.goto("/simulate-inbound");
    await expect(page.locator("#inbound-domain-select")).toHaveValue(DOMAIN);

    // Fill required fields
    await page.locator("#inbound-from").fill("sender@example.com");
    await page.locator("#inbound-subject").fill("Route match test");

    // Submit
    await page.locator("button.btn-send").click();

    // Result panel should appear
    await expect(page.locator("h2", { hasText: "Simulation Result" })).toBeVisible();

    // Matched Routes section should show the route ID (as a code element)
    const matchedRoutesSection = page.locator(".detail-section", { hasText: "Matched Routes" });
    await expect(matchedRoutesSection).toBeVisible();
    const routeCode = matchedRoutesSection.locator("code.action-item");
    await expect(routeCode.first()).toBeVisible();
  });

  // 6. Result shows "no routes matched" — simulate without matching routes, verify info message
  test("result shows no routes matched info", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/simulate-inbound");

    await expect(page.locator("#inbound-domain-select")).toHaveValue(DOMAIN);

    // Fill required fields
    await page.locator("#inbound-from").fill("sender@example.com");
    await page.locator("#inbound-subject").fill("No route test");

    // Submit
    await page.locator("button.btn-send").click();

    // Result panel should appear
    await expect(page.locator("h2", { hasText: "Simulation Result" })).toBeVisible();

    // Should show the "no routes matched" info message
    await expect(
      page.getByText("No routes matched this inbound message"),
    ).toBeVisible();
  });

  // 7. Result shows actions executed — verify actions section when routes match
  test("result shows actions executed", async ({ page, api }) => {
    await api.createDomain(DOMAIN);

    // Create a route with actions
    await api.createRoute({
      priority: 0,
      expression: `match_recipient(".*@${DOMAIN}")`,
      action: ["forward(\"https://example.com/hook\")", "stop()"],
      description: "Actions test route",
    });

    await page.goto("/simulate-inbound");
    await expect(page.locator("#inbound-domain-select")).toHaveValue(DOMAIN);

    // Fill required fields
    await page.locator("#inbound-from").fill("sender@example.com");
    await page.locator("#inbound-subject").fill("Actions test");

    // Submit
    await page.locator("button.btn-send").click();

    // Result panel should appear
    await expect(page.locator("h2", { hasText: "Simulation Result" })).toBeVisible();

    // Actions Executed section should be visible with action items
    const actionsSection = page.locator(".detail-section", { hasText: "Actions Executed" });
    await expect(actionsSection).toBeVisible();
    const actionItems = actionsSection.locator("code.action-item");
    await expect(actionItems.first()).toBeVisible();
  });

  // 8. Reset form — fill form, click reset, verify all fields cleared
  test("reset form clears all fields and result", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await page.goto("/simulate-inbound");

    await expect(page.locator("#inbound-domain-select")).toHaveValue(DOMAIN);

    // Fill all fields
    await page.locator("#inbound-from").fill("sender@example.com");
    await page.locator("#inbound-to").fill(`test@${DOMAIN}`);
    await page.locator("#inbound-subject").fill("Reset test");
    await page.locator("#inbound-body-plain").fill("Plain body");
    await page.locator("#inbound-body-html").fill("<p>HTML body</p>");

    // Submit first to get a result panel
    await page.locator("button.btn-send").click();
    await expect(page.locator("h2", { hasText: "Simulation Result" })).toBeVisible();

    // Click Reset
    await page.getByRole("button", { name: "Reset" }).click();

    // All form fields should be cleared
    await expect(page.locator("#inbound-from")).toHaveValue("");
    await expect(page.locator("#inbound-to")).toHaveValue("");
    await expect(page.locator("#inbound-subject")).toHaveValue("");
    await expect(page.locator("#inbound-body-plain")).toHaveValue("");
    await expect(page.locator("#inbound-body-html")).toHaveValue("");

    // Result panel should be gone
    await expect(page.locator("h2", { hasText: "Simulation Result" })).not.toBeVisible();
  });
});
