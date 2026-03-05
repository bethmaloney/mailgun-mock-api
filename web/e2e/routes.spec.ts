import { test, expect } from "./fixtures";

test.describe("Routes Page", () => {
  test("shows empty state — no routes displayed", async ({ page }) => {
    await page.goto("/routes");
    await expect(page.locator("main h1")).toHaveText("Routes");
    await expect(page.getByText("0 total")).toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  test("toggle create form — click Add Route to show form, click Cancel to hide", async ({ page }) => {
    await page.goto("/routes");

    // Initially the form should not be visible
    await expect(page.getByText("Create Route")).not.toBeVisible();

    // Click "Add Route" to show the form
    await page.getByRole("button", { name: "Add Route" }).click();
    await expect(page.getByText("Create Route")).toBeVisible();
    await expect(page.getByLabel("Priority")).toBeVisible();
    await expect(page.getByLabel("Expression")).toBeVisible();
    await expect(page.getByLabel("Actions (comma-separated)")).toBeVisible();
    await expect(page.getByLabel("Description")).toBeVisible();

    // Button should now say "Cancel"
    await expect(page.getByRole("button", { name: "Cancel" })).toBeVisible();

    // Click "Cancel" to hide the form
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByText("Create Route")).not.toBeVisible();

    // Button should say "Add Route" again
    await expect(page.getByRole("button", { name: "Add Route" })).toBeVisible();
  });

  test("create route via UI — fill form, submit, verify appears", async ({ page }) => {
    await page.goto("/routes");

    // Open the create form
    await page.getByRole("button", { name: "Add Route" }).click();

    // Fill in the form fields
    await page.getByLabel("Priority").fill("10");
    await page.getByLabel("Expression").fill("match_recipient('test@example.com')");
    await page.getByLabel("Actions (comma-separated)").fill("forward('http://localhost:9090'), stop()");
    await page.getByLabel("Description").fill("Test route for e2e");

    // Submit
    await page.getByRole("button", { name: "Create" }).click();

    // Form should close and route should appear (increased timeout for API round-trip)
    await expect(page.getByText("1 total")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("match_recipient('test@example.com')")).toBeVisible();
    await expect(page.getByText("Test route for e2e")).toBeVisible();
  });

  test("routes table has correct columns — priority, expression, actions, description, created at", async ({ page, api }) => {
    await api.createRoute({
      priority: 5,
      expression: "match_header('X-Test', 'yes')",
      action: ["forward('http://example.com')"],
      description: "Column test route",
    });

    await page.goto("/routes");
    await expect(page.getByText("1 total")).toBeVisible();

    // Check table headers
    const headers = page.locator("table thead th");
    await expect(headers.nth(0)).toHaveText("Priority");
    await expect(headers.nth(1)).toHaveText("Expression");
    await expect(headers.nth(2)).toHaveText("Actions");
    await expect(headers.nth(3)).toHaveText("Description");
    await expect(headers.nth(4)).toHaveText("Created At");
  });

  test("view route detail — click expression, verify detail panel", async ({ page, api }) => {
    await api.createRoute({
      priority: 3,
      expression: "match_recipient('detail@example.com')",
      action: ["forward('http://hook.example.com')", "stop()"],
      description: "Detail test route",
    });

    await page.goto("/routes");
    await expect(page.getByText("1 total")).toBeVisible();

    // Click the expression link to open detail
    await page.locator("a.route-link").click();

    // Verify detail panel
    await expect(page.getByRole("heading", { name: "Route Detail" })).toBeVisible();

    // Check detail fields
    await expect(page.locator(".detail-panel").getByText("ID")).toBeVisible();
    // Use exact match for priority to avoid matching the ID hex string that may contain '3'
    await expect(page.locator(".detail-panel span").filter({ hasText: /^3$/ })).toBeVisible();
    await expect(page.locator(".detail-panel").getByText("Detail test route")).toBeVisible();
    await expect(page.locator(".detail-panel").getByText("Created At")).toBeVisible();

    // Check expression section
    await expect(page.locator("pre").getByText("match_recipient('detail@example.com')")).toBeVisible();

    // Check actions section
    await expect(page.locator(".detail-panel").getByRole("heading", { name: "Actions" })).toBeVisible();
    await expect(page.locator("code").getByText("forward('http://hook.example.com')")).toBeVisible();
    await expect(page.locator("code").getByText("stop()")).toBeVisible();

    // Close button should work
    await page.getByRole("button", { name: "Close" }).click();
    await expect(page.getByRole("heading", { name: "Route Detail" })).not.toBeVisible();
  });

  test("delete route — click delete, confirm, verify removed", async ({ page, api }) => {
    await api.createRoute({
      priority: 1,
      expression: "match_recipient('delete-me@example.com')",
      action: ["stop()"],
      description: "To be deleted",
    });

    await page.goto("/routes");
    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("match_recipient('delete-me@example.com')")).toBeVisible();

    // Handle the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());
    await page.locator("button.btn-delete").click();

    // Route should be removed
    await expect(page.getByText("match_recipient('delete-me@example.com')")).not.toBeVisible();
    await expect(page.getByText("0 total")).toBeVisible();
  });

  test("pagination — verify next/previous with many routes", async ({ page, api }) => {
    // Create 31 routes sequentially to exceed the default page size of 30
    // (sequential to avoid SQLite concurrent write issues)
    for (let i = 0; i < 31; i++) {
      await api.createRoute({
        priority: i,
        expression: `match_recipient('user${i}@example.com')`,
        action: ["stop()"],
        description: `Route number ${i}`,
      });
    }

    await page.goto("/routes");
    await expect(page.getByText("31 total")).toBeVisible();

    // First page: Previous should be disabled, Next should be enabled
    await expect(page.getByRole("button", { name: "Previous" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Next" })).toBeEnabled();

    // Navigate to page 2
    await page.getByRole("button", { name: "Next" }).click();

    // Second page: Previous should be enabled, Next should be disabled (only 1 item left)
    await expect(page.getByRole("button", { name: "Previous" })).toBeEnabled();
    await expect(page.getByRole("button", { name: "Next" })).toBeDisabled();

    // Navigate back to page 1
    await page.getByRole("button", { name: "Previous" }).click();
    await expect(page.getByRole("button", { name: "Previous" })).toBeDisabled();
    await expect(page.getByRole("button", { name: "Next" })).toBeEnabled();
  });
});
