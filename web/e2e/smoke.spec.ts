import { test, expect } from "./fixtures";

const pages = [
  { path: "/", heading: "Dashboard" },
  { path: "/messages", heading: "Messages" },
  { path: "/events", heading: "Events" },
  { path: "/domains", heading: "Domains" },
  { path: "/templates", heading: "Templates" },
  { path: "/mailing-lists", heading: "Mailing Lists" },
  { path: "/routes", heading: "Routes" },
  { path: "/suppressions", heading: "Suppressions" },
  { path: "/webhooks", heading: "Webhooks" },
  { path: "/settings", heading: "Settings" },
  { path: "/trigger-events", heading: "Trigger Events" },
  { path: "/simulate-inbound", heading: "Simulate Inbound" },
];

for (const { path, heading } of pages) {
  test(`${heading} page loads at ${path}`, async ({ page }) => {
    await page.goto(path);
    await expect(page.locator("main h1")).toHaveText(heading);
  });
}

test("sidebar navigation links work", async ({ page }) => {
  await page.goto("/");

  // Click through a few sidebar links and verify navigation
  await page.getByRole("link", { name: "Messages" }).click();
  await expect(page.locator("main h1")).toHaveText("Messages");

  await page.getByRole("link", { name: "Domains" }).click();
  await expect(page.locator("main h1")).toHaveText("Domains");

  await page.getByRole("link", { name: "Settings" }).click();
  await expect(page.locator("main h1")).toHaveText("Settings");

  await page.getByRole("link", { name: "Dashboard" }).click();
  await expect(page.locator("main h1")).toHaveText("Dashboard");
});
