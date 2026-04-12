import { test, expect } from "./fixtures";

const API_BASE = "http://localhost:8026";

test.describe("API Keys Page", () => {
  test("empty state visible on first load", async ({ page, api: _api }) => {
    await page.goto("/api-keys");
    await expect(page.getByRole("heading", { name: "API Keys" })).toBeVisible();
    await expect(
      page.getByText(
        "No API keys yet. Test apps won't be able to call the Mailgun API surface until you create one.",
      ),
    ).toBeVisible();
  });

  test("create a key with name ci-runner", async ({ page, api: _api }) => {
    await page.goto("/api-keys");

    await page.getByPlaceholder("Key name (e.g. my-test-app)").fill("ci-runner");
    await page.getByRole("button", { name: "Create Key" }).click();

    // Verify a row appears in the table with the correct name and prefix
    await expect(page.getByRole("cell", { name: "ci-runner" })).toBeVisible();
    await expect(page.locator("td").filter({ hasText: /^mock_/ })).toBeVisible();
  });

  test("just-created panel shows full key value and copy button", async ({ page, api: _api }) => {
    await page.goto("/api-keys");

    await page.getByPlaceholder("Key name (e.g. my-test-app)").fill("panel-test");
    await page.getByRole("button", { name: "Create Key" }).click();

    // Verify the green created panel is visible
    const panel = page.locator(".created-panel");
    await expect(panel).toBeVisible();

    // Panel contains a key value starting with mock_
    await expect(panel.getByText(/mock_/)).toBeVisible();

    // Copy button is visible
    await expect(panel.getByRole("button", { name: "Copy" })).toBeVisible();

    // Dismiss button hides the panel
    await panel.getByRole("button", { name: "Dismiss" }).click();
    await expect(panel).not.toBeVisible();
  });

  test("delete a key with confirmation", async ({ page, api: _api }) => {
    // Create a key via the API directly
    const res = await fetch(`${API_BASE}/mock/api-keys`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "to-delete" }),
    });
    expect(res.status).toBe(201);

    await page.goto("/api-keys");

    // Verify key is visible
    const row = page.getByRole("row", { name: /to-delete/ });
    await expect(row).toBeVisible();

    // Accept the confirm dialog
    page.once("dialog", (dialog) => dialog.accept());

    // Click the delete button within the specific row
    await row.locator('.btn-delete[title="Delete key"]').click();

    // Verify the key disappears
    await expect(page.getByRole("cell", { name: "to-delete" })).not.toBeVisible();
    await expect(
      page.getByText("No API keys yet."),
    ).toBeVisible({ timeout: 5000 });
  });

  test("create two keys, confirm both appear", async ({ page, api: _api }) => {
    // Create the first key via the API
    await fetch(`${API_BASE}/mock/api-keys`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "key-alpha" }),
    });

    await page.goto("/api-keys");
    await expect(page.getByRole("cell", { name: "key-alpha" })).toBeVisible();

    // Create the second key via the UI
    await page.getByPlaceholder("Key name (e.g. my-test-app)").fill("key-beta");
    await page.getByRole("button", { name: "Create Key" }).click();

    // Verify both keys appear in the table
    await expect(page.getByRole("cell", { name: "key-alpha" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "key-beta" })).toBeVisible();
  });
});
