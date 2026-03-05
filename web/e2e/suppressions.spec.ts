import { test, expect } from "./fixtures";

const DOMAIN = "suppress-test.example.com";

test.describe("Suppressions Page", () => {
  test("domain selector required — table not shown until domain selected", async ({ page }) => {
    // No domains created — navigate to page
    await page.goto("/suppressions");
    await expect(page.getByRole("heading", { name: "Suppressions" })).toBeVisible();

    // Tabs, table, search, and action buttons should NOT be visible
    await expect(page.locator(".tab-bar")).not.toBeVisible();
    await expect(page.locator(".data-table")).not.toBeVisible();
    await expect(page.getByPlaceholder("Search...")).not.toBeVisible();
    await expect(page.getByRole("button", { name: "Add Entry" })).not.toBeVisible();
    await expect(page.getByRole("button", { name: "Clear All" })).not.toBeVisible();

    // Info message should be shown
    await expect(page.getByText("No domains configured")).toBeVisible();
  });

  test.describe("Bounces tab", () => {
    test("empty state", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      // Domain should be auto-selected, tabs visible
      await expect(page.locator("button.tab-btn.active")).toHaveText("Bounces");
      await expect(page.getByText("No data available.")).toBeVisible();
    });

    test("add bounce — fill address, code, error message, submit, verify appears", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      // Open add form
      await page.getByRole("button", { name: "Add Entry" }).click();
      await expect(page.getByRole("button", { name: "Add Bounce" })).toBeVisible();

      // Fill in fields
      await page.getByPlaceholder("Email address").fill("bounce@example.com");
      await page.getByPlaceholder("Code (e.g. 550)").fill("421");
      await page.getByPlaceholder("Error message").fill("Mailbox full");

      // Submit
      await page.getByRole("button", { name: "Add Bounce" }).click();

      // Verify entry appears in table
      await expect(page.getByText("bounce@example.com")).toBeVisible();
      await expect(page.getByText("421")).toBeVisible();
      await expect(page.getByText("Mailbox full")).toBeVisible();
    });

    test("delete bounce — click delete, verify removed", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await api.addBounce(DOMAIN, "delete-me@example.com", "550", "Bad address");
      await page.goto("/suppressions");

      await expect(page.getByText("delete-me@example.com")).toBeVisible();

      // Accept the confirm dialog
      page.once("dialog", (dialog) => dialog.accept());
      await page.locator("button.btn-delete").click();

      await expect(page.getByText("delete-me@example.com")).not.toBeVisible();
      await expect(page.getByText("No data available.")).toBeVisible();
    });

    test("clear all — click clear all, confirm, verify all removed", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await api.addBounce(DOMAIN, "b1@example.com", "550", "err1");
      await api.addBounce(DOMAIN, "b2@example.com", "421", "err2");
      await page.goto("/suppressions");

      await expect(page.getByText("b1@example.com")).toBeVisible();
      await expect(page.getByText("b2@example.com")).toBeVisible();

      page.once("dialog", (dialog) => dialog.accept());
      await page.getByRole("button", { name: "Clear All" }).click();

      await expect(page.getByText("b1@example.com")).not.toBeVisible();
      await expect(page.getByText("b2@example.com")).not.toBeVisible();
      await expect(page.getByText("No data available.")).toBeVisible();
    });
  });

  test.describe("Complaints tab", () => {
    test("switch tab — click Complaints tab, verify active", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      await page.locator("button.tab-btn", { hasText: "Complaints" }).click();
      await expect(page.locator("button.tab-btn.active")).toHaveText("Complaints");
    });

    test("add complaint — fill address, submit, verify appears", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      // Switch to Complaints tab
      await page.locator("button.tab-btn", { hasText: "Complaints" }).click();
      await expect(page.locator("button.tab-btn.active")).toHaveText("Complaints");

      // Open add form
      await page.getByRole("button", { name: "Add Entry" }).click();
      await expect(page.getByRole("button", { name: "Add Complaint" })).toBeVisible();

      // Fill and submit
      await page.getByPlaceholder("Email address").fill("complainer@example.com");
      await page.getByRole("button", { name: "Add Complaint" }).click();

      // Verify appears
      await expect(page.getByText("complainer@example.com")).toBeVisible();
    });

    test("delete complaint — click delete, verify removed", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await api.addComplaint(DOMAIN, "remove-complaint@example.com");
      await page.goto("/suppressions");

      // Switch to Complaints tab
      await page.locator("button.tab-btn", { hasText: "Complaints" }).click();
      await expect(page.getByText("remove-complaint@example.com")).toBeVisible();

      page.once("dialog", (dialog) => dialog.accept());
      await page.locator("button.btn-delete").click();

      await expect(page.getByText("remove-complaint@example.com")).not.toBeVisible();
      await expect(page.getByText("No data available.")).toBeVisible();
    });
  });

  test.describe("Unsubscribes tab", () => {
    test("switch tab", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      await page.locator("button.tab-btn", { hasText: "Unsubscribes" }).click();
      await expect(page.locator("button.tab-btn.active")).toHaveText("Unsubscribes");
    });

    test("add unsubscribe — fill address and tag, submit, verify appears", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      // Switch to Unsubscribes tab
      await page.locator("button.tab-btn", { hasText: "Unsubscribes" }).click();

      // Open add form
      await page.getByRole("button", { name: "Add Entry" }).click();
      await expect(page.getByRole("button", { name: "Add Unsubscribe" })).toBeVisible();

      // Fill and submit
      await page.getByPlaceholder("Email address").fill("unsub@example.com");
      await page.getByPlaceholder("Tag (default: *)").fill("newsletter");
      await page.getByRole("button", { name: "Add Unsubscribe" }).click();

      // Verify appears
      await expect(page.getByText("unsub@example.com")).toBeVisible();
      await expect(page.getByText("newsletter")).toBeVisible();
    });

    test("delete unsubscribe — click delete, verify removed", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await api.addUnsubscribe(DOMAIN, "remove-unsub@example.com", "promo");
      await page.goto("/suppressions");

      // Switch to Unsubscribes tab
      await page.locator("button.tab-btn", { hasText: "Unsubscribes" }).click();
      await expect(page.getByText("remove-unsub@example.com")).toBeVisible();

      page.once("dialog", (dialog) => dialog.accept());
      await page.locator("button.btn-delete").click();

      await expect(page.getByText("remove-unsub@example.com")).not.toBeVisible();
      await expect(page.getByText("No data available.")).toBeVisible();
    });
  });

  test.describe("Allowlist tab", () => {
    test("switch tab", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      await page.locator("button.tab-btn", { hasText: "Allowlist" }).click();
      await expect(page.locator("button.tab-btn.active")).toHaveText("Allowlist");
    });

    test("add by address — select type address, fill value, submit, verify appears", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      // Switch to Allowlist tab
      await page.locator("button.tab-btn", { hasText: "Allowlist" }).click();

      // Open add form
      await page.getByRole("button", { name: "Add Entry" }).click();
      await expect(page.getByRole("button", { name: "Add to Allowlist" })).toBeVisible();

      // Type defaults to "address", fill value
      await page.getByPlaceholder("user@example.com").fill("allowed@example.com");
      await page.getByRole("button", { name: "Add to Allowlist" }).click();

      // Verify appears
      await expect(page.getByText("allowed@example.com")).toBeVisible();
      await expect(page.getByRole("cell", { name: "address" })).toBeVisible();
    });

    test("add by domain — select type domain, fill value, submit, verify appears", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await page.goto("/suppressions");

      // Switch to Allowlist tab
      await page.locator("button.tab-btn", { hasText: "Allowlist" }).click();

      // Open add form
      await page.getByRole("button", { name: "Add Entry" }).click();

      // Select "Domain" type
      await page.locator(".add-form select.select-input").selectOption("domain");

      // Fill domain value
      await page.getByPlaceholder("example.com").fill("trusted.com");
      await page.getByRole("button", { name: "Add to Allowlist" }).click();

      // Verify appears
      await expect(page.getByText("trusted.com")).toBeVisible();
      await expect(page.getByRole("cell", { name: "domain" })).toBeVisible();
    });

    test("delete entry — click delete, verify removed", async ({ page, api }) => {
      await api.createDomain(DOMAIN);
      await api.addAllowlistEntry(DOMAIN, "remove-me@example.com", "address");
      await page.goto("/suppressions");

      // Switch to Allowlist tab
      await page.locator("button.tab-btn", { hasText: "Allowlist" }).click();
      await expect(page.getByText("remove-me@example.com")).toBeVisible();

      page.once("dialog", (dialog) => dialog.accept());
      await page.locator("button.btn-delete").click();

      await expect(page.getByText("remove-me@example.com")).not.toBeVisible();
      await expect(page.getByText("No data available.")).toBeVisible();
    });
  });

  test("search filter — add multiple entries, type in search, verify client-side filtering", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.addBounce(DOMAIN, "alice@example.com", "550", "user unknown");
    await api.addBounce(DOMAIN, "bob@example.com", "421", "try again");
    await api.addBounce(DOMAIN, "carol@example.com", "550", "mailbox full");
    await page.goto("/suppressions");

    // All three should be visible
    await expect(page.getByText("alice@example.com")).toBeVisible();
    await expect(page.getByText("bob@example.com")).toBeVisible();
    await expect(page.getByText("carol@example.com")).toBeVisible();

    // Search for "alice"
    await page.getByPlaceholder("Search...").fill("alice");
    await expect(page.getByText("alice@example.com")).toBeVisible();
    await expect(page.getByText("bob@example.com")).not.toBeVisible();
    await expect(page.getByText("carol@example.com")).not.toBeVisible();

    // Search by error message
    await page.getByPlaceholder("Search...").fill("mailbox full");
    await expect(page.getByText("carol@example.com")).toBeVisible();
    await expect(page.getByText("alice@example.com")).not.toBeVisible();

    // Clear search — all should reappear
    await page.getByPlaceholder("Search...").fill("");
    await expect(page.getByText("alice@example.com")).toBeVisible();
    await expect(page.getByText("bob@example.com")).toBeVisible();
    await expect(page.getByText("carol@example.com")).toBeVisible();
  });

  test("pagination — verify pagination controls are present", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.addBounce(DOMAIN, "pag1@example.com", "550", "err");
    await page.goto("/suppressions");

    // Pagination buttons should exist
    await expect(page.getByRole("button", { name: "Previous" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Next" })).toBeVisible();

    // With a small data set, Next should be disabled (no more pages)
    await expect(page.getByRole("button", { name: "Next" })).toBeDisabled();
  });

  test("clear all per tab — verify clear all only clears active tab data", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    // Add data to both bounces and complaints
    await api.addBounce(DOMAIN, "bounce-keep@example.com", "550", "err");
    await api.addComplaint(DOMAIN, "complaint-keep@example.com");
    await page.goto("/suppressions");

    // Verify bounce is visible on Bounces tab
    await expect(page.getByText("bounce-keep@example.com")).toBeVisible();

    // Switch to Complaints tab and verify complaint is visible
    await page.locator("button.tab-btn", { hasText: "Complaints" }).click();
    await expect(page.getByText("complaint-keep@example.com")).toBeVisible();

    // Clear all complaints
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "Clear All" }).click();

    // Complaints should be gone
    await expect(page.getByText("complaint-keep@example.com")).not.toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();

    // Switch back to Bounces tab — bounces should still be there
    await page.locator("button.tab-btn", { hasText: "Bounces" }).click();
    await expect(page.getByText("bounce-keep@example.com")).toBeVisible();
  });
});
