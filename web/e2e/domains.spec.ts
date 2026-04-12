import { test, expect } from "./fixtures";

test.describe("Domains CRUD", () => {
  test("shows empty state initially", async ({ page }) => {
    await page.goto("/domains");
    await expect(page.locator("main h1")).toHaveText("Domains");
    await expect(page.getByText("0 total")).toBeVisible();
  });

  test("create a domain and see it in the list", async ({ page, api }) => {
    await api.createDomain("e2e-test.example.com");

    await page.goto("/domains");
    await expect(page.getByText("1 total")).toBeVisible();
    await expect(page.getByText("e2e-test.example.com")).toBeVisible();
  });

  test("create multiple domains", async ({ page, api }) => {
    await api.createDomain("first.example.com");
    await api.createDomain("second.example.com");

    await page.goto("/domains");
    await expect(page.getByText("2 total")).toBeVisible();
    await expect(page.getByText("first.example.com")).toBeVisible();
    await expect(page.getByText("second.example.com")).toBeVisible();
  });

  test("create domain via UI form", async ({ page, api: _api }) => {
    // _api triggers the fixture's reset() so this test sees an empty DB.
    await page.goto("/domains");

    await page.getByPlaceholder("Enter domain name").fill("ui-created.example.com");
    await page.getByRole("button", { name: "Add Domain" }).click();

    await expect(page.getByText("ui-created.example.com")).toBeVisible();
    await expect(page.getByText("1 total")).toBeVisible();
  });

  test("view domain detail", async ({ page, api }) => {
    await api.createDomain("detail-test.example.com");

    await page.goto("/domains");
    await page.getByRole("link", { name: "detail-test.example.com" }).click();

    // Detail panel should open
    await expect(page.getByText("Domain Detail")).toBeVisible();
    await expect(page.getByText("Sending DNS Records")).toBeVisible();
  });

  test("delete domain via UI", async ({ page, api }) => {
    await api.createDomain("to-delete.example.com");

    await page.goto("/domains");
    await expect(page.getByText("to-delete.example.com")).toBeVisible();

    // Handle the confirm dialog
    page.on("dialog", (dialog) => dialog.accept());
    await page.locator("button.btn-delete").click();

    await expect(page.getByText("to-delete.example.com")).not.toBeVisible();
    await expect(page.getByText("0 total")).toBeVisible();
  });
});
