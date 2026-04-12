import { test, expect } from "./fixtures";

const DOMAIN = "trigger-e2e.example.com";

test.describe("Trigger Events Page", () => {
  // 1. 3-step workflow visibility — only domain selector visible initially
  test("only domain selector visible initially when no domains", async ({ page, api: _api }) => {
    // _api triggers the fixture's reset() so this test sees an empty DB.
    await page.goto("/trigger-events");
    await expect(page.locator("main h1")).toHaveText("Trigger Events");

    // Step 1 (domain selector) should be visible
    await expect(page.getByText("1. Select Domain")).toBeVisible();

    // "No domains configured" info should appear
    await expect(page.getByText("No domains configured")).toBeVisible();

    // Step 2 (message list) should NOT be visible
    await expect(page.getByText("2. Select Message")).not.toBeVisible();

    // Step 3 (trigger event) should NOT be visible
    await expect(page.getByText("3. Trigger Event")).not.toBeVisible();
  });

  // 2. Domain selector loads domains
  test("domain selector loads domains", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createDomain("second.example.com");

    await page.goto("/trigger-events");

    const select = page.locator("#trigger-domain-select");
    await expect(select).toBeVisible();

    // Should have options for both domains
    const options = select.locator("option");
    // Wait for domains to load (first domain auto-selected)
    await expect(select).not.toHaveValue("");
    await expect(options).toHaveCount(2);
  });

  // 3. Message list loads after domain selected
  test("message list loads after domain selected", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Test message for trigger",
      text: "Body",
    });

    await page.goto("/trigger-events");

    // Domain is auto-selected, so step 2 should appear
    await expect(page.getByText("2. Select Message")).toBeVisible();

    // Messages table should show the message
    const table = page.locator("table.data-table");
    await expect(table).toBeVisible();
    await expect(table.getByText(`sender@${DOMAIN}`)).toBeVisible();
    await expect(table.getByText("Test message for trigger")).toBeVisible();
  });

  // 4. Message search filter
  test("message search filter filters messages", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `alice@${DOMAIN}`,
      to: `bob@${DOMAIN}`,
      subject: "Alice hello",
      text: "From alice",
    });
    await api.sendMessage(DOMAIN, {
      from: `carol@${DOMAIN}`,
      to: `dave@${DOMAIN}`,
      subject: "Carol hello",
      text: "From carol",
    });

    await page.goto("/trigger-events");
    await expect(page.getByText("2. Select Message")).toBeVisible();

    // Both messages should be visible
    const table = page.locator("table.data-table");
    await expect(table.getByText("Alice hello")).toBeVisible();
    await expect(table.getByText("Carol hello")).toBeVisible();

    // Type in search
    const searchInput = page.getByPlaceholder("Search by from, to, subject, or ID...");
    await searchInput.fill("alice");

    // Alice's message should remain, Carol's should be filtered out
    await expect(table.getByText("Alice hello")).toBeVisible();
    await expect(table.getByText("Carol hello")).not.toBeVisible();
  });

  // 5. Select message — click select button, verify step 3 with message summary
  test("select message shows step 3 with message summary", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Selectable message",
      text: "Body text",
    });

    await page.goto("/trigger-events");
    await expect(page.getByText("2. Select Message")).toBeVisible();

    // Step 3 should NOT be visible yet
    await expect(page.getByText("3. Trigger Event")).not.toBeVisible();

    // Click the Select button
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Button should now say "Selected" with btn-primary class
    await expect(page.locator("button.btn-sm.btn-primary", { hasText: "Selected" })).toBeVisible();

    // Step 3 should now be visible with message summary
    await expect(page.getByText("3. Trigger Event")).toBeVisible();
    const summary = page.locator(".selected-message-summary");
    await expect(summary).toBeVisible();
    await expect(summary.getByText(`sender@${DOMAIN}`)).toBeVisible();
    await expect(summary.getByText(`recipient@${DOMAIN}`)).toBeVisible();
    await expect(summary.getByText("Selectable message")).toBeVisible();
  });

  // 6. Event type buttons — verify all 6 buttons present
  test("event type buttons all present", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Event type test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();
    await expect(page.getByText("3. Trigger Event")).toBeVisible();

    const eventButtons = page.locator(".event-type-buttons button");
    await expect(eventButtons).toHaveCount(6);

    await expect(page.locator("button.btn-event-deliver")).toHaveText("Deliver");
    await expect(page.locator("button.btn-event-fail")).toHaveText("Fail");
    await expect(page.locator("button.btn-event-open")).toHaveText("Open");
    await expect(page.locator("button.btn-event-click")).toHaveText("Click");
    await expect(page.locator("button.btn-event-unsubscribe")).toHaveText("Unsubscribe");
    await expect(page.locator("button.btn-event-complain")).toHaveText("Complain");
  });

  // 7. Trigger deliver event
  test("trigger deliver event succeeds", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Deliver test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Deliver event type
    await page.locator("button.btn-event-deliver").click();

    // Trigger button should appear
    const triggerBtn = page.locator("button.btn-trigger");
    await expect(triggerBtn).toBeVisible();
    await expect(triggerBtn).toHaveText("Trigger Deliver Event");

    // Click trigger
    await triggerBtn.click();

    // Verify success result
    const result = page.locator(".trigger-result.result-success");
    await expect(result).toBeVisible();
    await expect(result.getByText("Success")).toBeVisible();
  });

  // 8. Trigger fail event — shows severity and error fields
  test("fail event shows severity and error fields", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Fail fields test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Fail event type
    await page.locator("button.btn-event-fail").click();

    // Failure Options section should appear
    await expect(page.getByText("Failure Options")).toBeVisible();

    // Severity select should be visible
    const severitySelect = page.locator("#fail-severity");
    await expect(severitySelect).toBeVisible();

    // Error message input should be visible
    const errorInput = page.locator("#fail-error");
    await expect(errorInput).toBeVisible();
    await expect(errorInput).toHaveAttribute("placeholder", "e.g. 550 User not found");
  });

  // 9. Trigger fail event — fill severity + error, trigger, verify success
  test("trigger fail event with severity and error succeeds", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Fail trigger test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Fail event type
    await page.locator("button.btn-event-fail").click();

    // Fill in severity and error
    await page.locator("#fail-severity").selectOption("temporary");
    await page.locator("#fail-error").fill("550 User not found");

    // Click trigger
    await page.locator("button.btn-trigger").click();

    // Verify success result
    const result = page.locator(".trigger-result.result-success");
    await expect(result).toBeVisible();
    await expect(result.getByText("Success")).toBeVisible();
  });

  // 10. Trigger click event — shows URL field
  test("click event shows URL field", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Click fields test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Click event type
    await page.locator("button.btn-event-click").click();

    // Click Options section should appear
    await expect(page.getByText("Click Options")).toBeVisible();

    // URL input should be visible
    const urlInput = page.locator("#click-url");
    await expect(urlInput).toBeVisible();
    await expect(urlInput).toHaveAttribute("placeholder", "https://example.com/link");
  });

  // 11. Trigger click event — fill URL, trigger, verify success
  test("trigger click event with URL succeeds", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Click trigger test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Click event type
    await page.locator("button.btn-event-click").click();

    // Fill in URL
    await page.locator("#click-url").fill("https://example.com/tracked-link");

    // Click trigger
    await page.locator("button.btn-trigger").click();

    // Verify success result
    const result = page.locator(".trigger-result.result-success");
    await expect(result).toBeVisible();
    await expect(result.getByText("Success")).toBeVisible();
  });

  // 12. Trigger open event
  test("trigger open event succeeds", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Open trigger test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Open event type
    await page.locator("button.btn-event-open").click();

    // Click trigger
    await page.locator("button.btn-trigger").click();

    // Verify success result
    const result = page.locator(".trigger-result.result-success");
    await expect(result).toBeVisible();
    await expect(result.getByText("Success")).toBeVisible();
  });

  // 13. Trigger unsubscribe event
  test("trigger unsubscribe event succeeds", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Unsubscribe trigger test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Unsubscribe event type
    await page.locator("button.btn-event-unsubscribe").click();

    // Click trigger
    await page.locator("button.btn-trigger").click();

    // Verify success result
    const result = page.locator(".trigger-result.result-success");
    await expect(result).toBeVisible();
    await expect(result.getByText("Success")).toBeVisible();
  });

  // 14. Trigger complain event
  test("trigger complain event succeeds", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Complain trigger test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Complain event type
    await page.locator("button.btn-event-complain").click();

    // Click trigger
    await page.locator("button.btn-trigger").click();

    // Verify success result
    const result = page.locator(".trigger-result.result-success");
    await expect(result).toBeVisible();
    await expect(result.getByText("Success")).toBeVisible();
  });

  // 15. Error result displayed
  test("error result displayed when trigger fails", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.sendMessage(DOMAIN, {
      from: `sender@${DOMAIN}`,
      to: `recipient@${DOMAIN}`,
      subject: "Error trigger test",
      text: "Body",
    });

    await page.goto("/trigger-events");
    await page.locator("button.btn-sm", { hasText: "Select" }).first().click();

    // Click Deliver event type
    await page.locator("button.btn-event-deliver").click();

    // Intercept the trigger API call and return a 500 error
    await page.route("**/mock/events/**", (route) => {
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ message: "Internal server error" }),
      });
    });

    // Click trigger
    await page.locator("button.btn-trigger").click();

    // Verify error result
    const result = page.locator(".trigger-result.result-error");
    await expect(result).toBeVisible();
    await expect(result.getByText("Error", { exact: true })).toBeVisible();
  });
});
