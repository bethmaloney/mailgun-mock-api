import { test, expect } from "./fixtures";

const DOMAIN = "tpl-e2e.example.com";

test.describe("Templates Page", () => {
  // 1. Shows empty state — no templates for selected domain
  test("shows empty state when no domains exist", async ({ page }) => {
    await page.goto("/templates");
    await expect(page.locator("main h1")).toHaveText("Templates");

    // Domain selector should show "No domains available"
    await expect(page.locator("#domain-select")).toBeVisible();
    await expect(
      page.locator("#domain-select option", { hasText: "No domains available" }),
    ).toBeAttached();

    // Info message when no domain is selected
    await expect(
      page.getByText("No domains configured. Create a domain first to view templates."),
    ).toBeVisible();

    // DataTable should NOT be rendered
    await expect(page.locator("table.data-table")).not.toBeVisible();
  });

  // 2. Domain selector populated — verify domains appear in dropdown
  test("domain selector populated with created domains", async ({ page, api }) => {
    await api.createDomain("alpha-tpl.example.com");
    await api.createDomain("beta-tpl.example.com");

    await page.goto("/templates");
    await expect(page.locator("main h1")).toHaveText("Templates");

    const domainSelect = page.locator("#domain-select");
    await expect(domainSelect).toBeVisible();

    // Both domains should appear as options
    await expect(
      domainSelect.locator("option", { hasText: "alpha-tpl.example.com" }),
    ).toBeAttached();
    await expect(
      domainSelect.locator("option", { hasText: "beta-tpl.example.com" }),
    ).toBeAttached();
  });

  // 3. Lists templates — create templates via API, verify table shows name, description, created_at
  test("lists templates in the table", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createTemplate(DOMAIN, {
      name: "welcome-email",
      description: "Welcome message",
      template: "<h1>Welcome</h1>",
    });
    await api.createTemplate(DOMAIN, {
      name: "reset-password",
      description: "Password reset",
      template: "<p>Reset your password</p>",
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible();

    // Verify template names are shown
    await expect(table.getByText("welcome-email")).toBeVisible();
    await expect(table.getByText("reset-password")).toBeVisible();

    // Verify descriptions are shown
    await expect(table.getByText("Welcome message")).toBeVisible();
    await expect(table.getByText("Password reset")).toBeVisible();

    // Verify Created At column header exists
    await expect(table.getByRole("columnheader", { name: "Created At" })).toBeVisible();
  });

  // 4. View template detail — click template name, verify detail panel with name, description, active version
  test("view template detail", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createTemplate(DOMAIN, {
      name: "detail-test",
      description: "Detail test template",
      template: "<h1>Hello {{name}}</h1>",
      tag: "v1",
      engine: "handlebars",
      comment: "first version",
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible();

    // Click on template name link
    await table.locator("a.template-link", { hasText: "detail-test" }).click();

    // Detail panel should appear
    await expect(page.getByRole("heading", { name: "Template Detail" })).toBeVisible();

    // Verify template info in detail panel
    const detailPanel = page.locator(".detail-panel").first();
    await expect(detailPanel.getByText("detail-test")).toBeVisible();
    await expect(detailPanel.getByText("Detail test template")).toBeVisible();

    // Verify active version section
    await expect(page.getByRole("heading", { name: "Active Version" })).toBeVisible();
    const activeVersionSection = detailPanel.locator(".detail-section").filter({ hasText: "Active Version" });
    await expect(activeVersionSection.getByText("v1")).toBeVisible();
    await expect(activeVersionSection.getByText("handlebars")).toBeVisible();
    await expect(activeVersionSection.getByText("first version")).toBeVisible();
  });

  // 5. View versions list in detail — verify versions table with tag, engine, active, comment
  test("view versions list in detail panel", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createTemplate(DOMAIN, {
      name: "versioned-tpl",
      description: "Template with versions",
      template: "<h1>V1</h1>",
      tag: "v1",
      engine: "handlebars",
      comment: "version one",
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible();

    // Click template name to open detail
    await table.locator("a.template-link", { hasText: "versioned-tpl" }).click();

    // Verify Versions heading is present
    await expect(page.getByRole("heading", { name: "Versions" })).toBeVisible();

    const versionsSection = page.locator(".detail-section").filter({ hasText: "Versions" });
    await expect(versionsSection).toBeVisible();

    // Verify versions table headers
    const versionsTable = versionsSection.locator("table.data-table");
    await expect(versionsTable.getByRole("columnheader", { name: "Tag" })).toBeVisible();
    await expect(versionsTable.getByRole("columnheader", { name: "Engine" })).toBeVisible();
    await expect(versionsTable.getByRole("columnheader", { name: "Active" })).toBeVisible();
    await expect(versionsTable.getByRole("columnheader", { name: "Comment" })).toBeVisible();

    // Verify the version row data
    await expect(versionsSection.getByText("v1")).toBeVisible();
    await expect(versionsSection.getByText("handlebars")).toBeVisible();
    await expect(versionsSection.getByText("version one")).toBeVisible();
  });

  // 6. View version detail — click version tag, verify version detail with template body
  test("view version detail", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createTemplate(DOMAIN, {
      name: "version-detail-tpl",
      description: "For version detail test",
      template: "<h1>Hello {{user}}</h1>",
      tag: "v1",
      engine: "handlebars",
      comment: "initial version",
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible();

    // Click template name to open detail
    await table.locator("a.template-link", { hasText: "version-detail-tpl" }).click();

    // Wait for detail panel and versions to load
    await expect(page.getByRole("heading", { name: "Template Detail" })).toBeVisible();

    const versionsSection = page.locator(".detail-section").filter({ hasText: "Versions" });
    await expect(versionsSection).toBeVisible();

    // Click version tag link
    await versionsSection.locator("a.template-link", { hasText: "v1" }).click();

    // Version detail should show
    await expect(page.getByRole("heading", { name: /Version Content: v1/ })).toBeVisible();

    // Verify template body is displayed
    const bodyContent = page.locator("pre.body-content");
    await expect(bodyContent).toBeVisible();
    await expect(bodyContent).toContainText("Hello");
  });

  // 7. Delete template — click delete, confirm, verify removed
  test("delete template via UI", async ({ page, api }) => {
    await api.createDomain(DOMAIN);
    await api.createTemplate(DOMAIN, {
      name: "to-delete",
      description: "Will be deleted",
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible();
    await expect(table.getByText("to-delete")).toBeVisible();

    // Handle the confirm dialog (once, so it doesn't persist)
    page.once("dialog", (dialog) => dialog.accept());

    // Click delete button
    await table.getByRole("button", { name: "Delete" }).click();

    // Template should be removed from the list
    await expect(table.getByText("to-delete")).not.toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  // 8. Pagination for templates — verify pagination with many templates
  test("pagination for templates list", async ({ page, api }) => {
    test.setTimeout(60000);
    await api.createDomain(DOMAIN);

    // Create 12 templates
    for (let i = 1; i <= 12; i++) {
      await api.createTemplate(DOMAIN, {
        name: `tpl-${String(i).padStart(3, "0")}`,
        description: `Template number ${i}`,
      });
    }

    // Intercept templates API calls and set limit=5 to force pagination
    await page.route("**/v3/*/templates**", (route) => {
      const url = new URL(route.request().url());
      // Only add limit to the templates list endpoint, not to sub-resources like versions
      if (!url.pathname.includes("/versions") && !url.searchParams.has("limit")) {
        url.searchParams.set("limit", "5");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible({ timeout: 10000 });

    // Should show first page with 5 rows
    const rows = table.locator("tbody tr");
    await expect(rows).toHaveCount(5, { timeout: 10000 });

    // Next button should be enabled
    const nextBtn = page.getByRole("button", { name: "Next" }).first();
    const prevBtn = page.getByRole("button", { name: "Previous" }).first();
    await expect(nextBtn).toBeEnabled();

    // Record first page content
    const firstPageText = await rows.first().textContent();

    // Go to next page
    await nextBtn.click();

    // Wait for next page to load
    await expect(rows.first()).toBeVisible({ timeout: 10000 });

    // Previous should be enabled on page 2
    await expect(prevBtn).toBeEnabled();

    // Content should have changed
    const secondPageText = await rows.first().textContent();
    expect(secondPageText).not.toBe(firstPageText);

    // Navigate to last page
    await nextBtn.click();
    await expect(rows.first()).toBeVisible({ timeout: 10000 });

    // On the last page (12 templates, 5 per page = 3 pages, last has 2)
    await expect(rows).toHaveCount(2, { timeout: 10000 });
    await expect(nextBtn).toBeDisabled();
    await expect(prevBtn).toBeEnabled();

    // Go back with Previous
    await prevBtn.click();
    await expect(rows.first()).toBeVisible({ timeout: 10000 });
  });

  // 9. Pagination for versions — verify pagination with many versions
  test("pagination for versions list", async ({ page, api }) => {
    test.setTimeout(90000);
    await api.createDomain(DOMAIN);

    // Create template with initial version
    await api.createTemplate(DOMAIN, {
      name: "many-versions",
      description: "Template with many versions",
      template: "<h1>Initial</h1>",
      tag: "v01",
    });

    // Create 11 additional versions (total 12 including initial)
    for (let i = 2; i <= 12; i++) {
      await api.createTemplateVersion(DOMAIN, "many-versions", {
        template: `<h1>Version ${i}</h1>`,
        tag: `v${String(i).padStart(2, "0")}`,
        comment: `version ${i}`,
      });
    }

    // Intercept versions API calls and set limit=5 to force pagination
    await page.route("**/v3/*/templates/*/versions**", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("limit")) {
        url.searchParams.set("limit", "5");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/templates");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible({ timeout: 10000 });

    // Click template name to open detail
    await table.locator("a.template-link", { hasText: "many-versions" }).click();

    // Wait for detail panel
    await expect(page.getByRole("heading", { name: "Template Detail" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Versions" })).toBeVisible();

    const versionsSection = page.locator(".detail-section").filter({ hasText: "Versions" });
    const versionsTable = versionsSection.locator("table.data-table");

    // Verify versions loaded with pagination (5 per page)
    const versionRows = versionsTable.locator("tbody tr");
    await expect(versionRows).toHaveCount(5, { timeout: 10000 });

    // The versions pagination buttons are the last set on the page
    const versionsNext = page.getByRole("button", { name: "Next" }).last();
    const versionsPrev = page.getByRole("button", { name: "Previous" }).last();

    await expect(versionsNext).toBeEnabled();

    // Navigate to next page of versions
    await versionsNext.click();
    await expect(versionRows.first()).toBeVisible({ timeout: 10000 });
    await expect(versionsPrev).toBeEnabled();
  });
});
