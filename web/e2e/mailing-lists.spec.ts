import { test, expect } from "./fixtures";

test.describe("Mailing Lists Page", () => {
  // 1. Shows empty state — no mailing lists displayed
  test("shows empty state — no mailing lists displayed", async ({ page }) => {
    await page.goto("/mailing-lists");
    await expect(page.locator("main h1")).toHaveText("Mailing Lists");

    // DataTable should show empty message
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  // 2. Create mailing list — fill address, name, description, submit, verify appears in table
  test("create mailing list via UI form", async ({ page }) => {
    await page.goto("/mailing-lists");

    await page.getByPlaceholder("List address (e.g. devs@lists.example.com)").fill("devs@lists.example.com");
    await page.getByPlaceholder("Name").first().fill("Developers");
    await page.getByPlaceholder("Description").fill("Developer mailing list");
    await page.getByRole("button", { name: "Create List" }).click();

    // Verify the new list appears in the table
    await expect(page.getByText("devs@lists.example.com")).toBeVisible();
    await expect(page.getByText("Developers")).toBeVisible();
  });

  // 3. Create list with only address — name and description optional
  test("create list with only address", async ({ page }) => {
    await page.goto("/mailing-lists");

    await page.getByPlaceholder("List address (e.g. devs@lists.example.com)").fill("minimal@lists.example.com");
    await page.getByRole("button", { name: "Create List" }).click();

    // Verify the list appears with the address
    await expect(page.getByText("minimal@lists.example.com")).toBeVisible();
  });

  // 4. Lists show correct columns — address, name, members count, access level, created at
  test("lists show correct columns", async ({ page, api }) => {
    await api.createMailingList("cols@lists.example.com", "Column Test", "Test description");

    await page.goto("/mailing-lists");

    const table = page.locator("table.data-table").first();
    await expect(table).toBeVisible();

    // Verify column headers
    await expect(table.getByRole("columnheader", { name: "Address" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "Name" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "Members" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "Access Level" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "Created At" })).toBeVisible();

    // Verify row data
    await expect(table.getByText("cols@lists.example.com")).toBeVisible();
    await expect(table.getByText("Column Test")).toBeVisible();
  });

  // 5. View list detail — click address, verify detail panel with all fields
  test("view list detail", async ({ page, api }) => {
    await api.createMailingList("detail@lists.example.com", "Detail List", "A description for detail");

    await page.goto("/mailing-lists");

    // Click the address link to open detail panel
    await page.getByRole("link", { name: "detail@lists.example.com" }).click();

    // Detail panel should appear
    await expect(page.getByRole("heading", { name: "List Detail" })).toBeVisible();

    const detailPanel = page.locator(".detail-panel").first();
    await expect(detailPanel).toBeVisible();

    // Verify all detail fields
    await expect(detailPanel.getByText("detail@lists.example.com")).toBeVisible();
    await expect(detailPanel.getByText("Detail List")).toBeVisible();
    await expect(detailPanel.getByText("A description for detail")).toBeVisible();

    // Verify labels are present
    await expect(detailPanel.locator("label", { hasText: "Address" })).toBeVisible();
    await expect(detailPanel.locator("label", { hasText: "Name" })).toBeVisible();
    await expect(detailPanel.locator("label", { hasText: "Description" })).toBeVisible();
    await expect(detailPanel.locator("label", { hasText: "Access Level" })).toBeVisible();
    await expect(detailPanel.locator("label", { hasText: "Reply Preference" })).toBeVisible();
    await expect(detailPanel.locator("label", { hasText: "Members Count" })).toBeVisible();
  });

  // 6. Add member to list — fill email + name, add, verify member appears in members table
  test("add member to list", async ({ page, api }) => {
    await api.createMailingList("members@lists.example.com", "Members List");

    await page.goto("/mailing-lists");

    // Open the detail panel
    await page.getByRole("link", { name: "members@lists.example.com" }).click();
    await expect(page.getByRole("heading", { name: "List Detail" })).toBeVisible();

    // Verify Members section is present
    await expect(page.getByRole("heading", { name: "Members" })).toBeVisible();

    // Fill in the add member form
    await page.getByPlaceholder("Member email address").fill("alice@example.com");
    // Use last() for the Name placeholder since the create list form also has one
    await page.getByPlaceholder("Name").last().fill("Alice Smith");
    await page.getByRole("button", { name: "Add Member" }).click();

    // Verify member appears in the members table
    const detailPanel = page.locator(".detail-panel").first();
    await expect(detailPanel.getByText("alice@example.com")).toBeVisible();
    await expect(detailPanel.getByText("Alice Smith")).toBeVisible();
  });

  // 7. Member shows subscribed status — verify subscribed column shows yes/no
  test("member shows subscribed status", async ({ page, api }) => {
    await api.createMailingList("sub-status@lists.example.com", "Sub Status List");
    await api.addMemberToList("sub-status@lists.example.com", "subscribed-user@example.com", "Subscribed User");

    await page.goto("/mailing-lists");

    // Open the detail panel
    await page.getByRole("link", { name: "sub-status@lists.example.com" }).click();
    await expect(page.getByRole("heading", { name: "List Detail" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Members" })).toBeVisible();

    // The members table should show subscribed status as "Yes"
    const detailPanel = page.locator(".detail-panel").first();
    const membersTable = detailPanel.locator("table.data-table");
    await expect(membersTable).toBeVisible();

    await expect(membersTable.getByText("subscribed-user@example.com")).toBeVisible();
    // Subscribed column should show "Yes"
    await expect(membersTable.getByRole("cell", { name: "Yes" })).toBeVisible();
  });

  // 8. Remove member from list — click delete on member, verify removed
  test("remove member from list", async ({ page, api }) => {
    await api.createMailingList("remove-member@lists.example.com", "Remove Member List");
    await api.addMemberToList("remove-member@lists.example.com", "to-remove@example.com", "Remove Me");

    await page.goto("/mailing-lists");

    // Open the detail panel
    await page.getByRole("link", { name: "remove-member@lists.example.com" }).click();
    await expect(page.getByRole("heading", { name: "List Detail" })).toBeVisible();

    // Verify the member is present
    const detailPanel = page.locator(".detail-panel").first();
    await expect(detailPanel.getByText("to-remove@example.com")).toBeVisible();

    // Handle confirm dialog
    page.once("dialog", (dialog) => dialog.accept());

    // Click the delete button for the member (the one with title "Remove member")
    await detailPanel.locator("button.btn-delete[title='Remove member']").click();

    // Verify the member is removed
    await expect(detailPanel.getByText("to-remove@example.com")).not.toBeVisible();
  });

  // 9. Delete mailing list — click delete on list, confirm, verify removed
  test("delete mailing list", async ({ page, api }) => {
    await api.createMailingList("to-delete@lists.example.com", "Delete Me");

    await page.goto("/mailing-lists");
    await expect(page.getByText("to-delete@lists.example.com")).toBeVisible();

    // Handle confirm dialog
    page.once("dialog", (dialog) => dialog.accept());

    // Click the delete button on the list row (title "Delete list")
    await page.locator("button.btn-delete[title='Delete list']").click();

    // Verify the list is removed
    await expect(page.getByText("to-delete@lists.example.com")).not.toBeVisible();
    await expect(page.getByText("No data available.")).toBeVisible();
  });

  // 10. Pagination for lists — verify pagination with many lists
  test("pagination for lists", async ({ page, api }) => {
    test.setTimeout(60000);

    // Create 12 mailing lists
    for (let i = 1; i <= 12; i++) {
      await api.createMailingList(
        `list-${String(i).padStart(3, "0")}@lists.example.com`,
        `List ${String(i).padStart(3, "0")}`,
      );
    }

    // Intercept lists API calls and set limit=5 to force pagination
    await page.route("**/v3/lists/pages**", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("limit")) {
        url.searchParams.set("limit", "5");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/mailing-lists");

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

    // On the last page (12 lists, 5 per page = 3 pages, last has 2)
    await expect(rows).toHaveCount(2, { timeout: 10000 });
    await expect(nextBtn).toBeDisabled();
    await expect(prevBtn).toBeEnabled();

    // Go back with Previous
    await prevBtn.click();
    await expect(rows.first()).toBeVisible({ timeout: 10000 });
  });

  // 11. Pagination for members — verify pagination with many members
  test("pagination for members", async ({ page, api }) => {
    test.setTimeout(90000);

    await api.createMailingList("paginated-members@lists.example.com", "Paginated Members");

    // Add 12 members
    for (let i = 1; i <= 12; i++) {
      await api.addMemberToList(
        "paginated-members@lists.example.com",
        `member-${String(i).padStart(3, "0")}@example.com`,
        `Member ${String(i).padStart(3, "0")}`,
      );
    }

    // Intercept members API calls and set limit=5 to force pagination
    await page.route("**/v3/lists/*/members/pages**", (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("limit")) {
        url.searchParams.set("limit", "5");
      }
      route.continue({ url: url.toString() });
    });

    await page.goto("/mailing-lists");

    // Open the detail panel
    await page.getByRole("link", { name: "paginated-members@lists.example.com" }).click();
    await expect(page.getByRole("heading", { name: "List Detail" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Members" })).toBeVisible();

    const detailPanel = page.locator(".detail-panel").first();
    const membersTable = detailPanel.locator("table.data-table");
    await expect(membersTable).toBeVisible({ timeout: 10000 });

    // Should show first page with 5 member rows
    const memberRows = membersTable.locator("tbody tr");
    await expect(memberRows).toHaveCount(5, { timeout: 10000 });

    // The members pagination buttons are the last set on the page
    const nextBtn = page.getByRole("button", { name: "Next" }).last();
    const prevBtn = page.getByRole("button", { name: "Previous" }).last();
    await expect(nextBtn).toBeEnabled();

    // Record first page content
    const firstPageText = await memberRows.first().textContent();

    // Go to next page
    await nextBtn.click();

    // Wait for next page to load
    await expect(memberRows.first()).toBeVisible({ timeout: 10000 });

    // Previous should be enabled on page 2
    await expect(prevBtn).toBeEnabled();

    // Content should have changed
    const secondPageText = await memberRows.first().textContent();
    expect(secondPageText).not.toBe(firstPageText);
  });
});
