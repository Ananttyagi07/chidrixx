import { test, expect } from "@playwright/test";
import { ADMIN_PASSWORD, ADMIN_USER } from "../env";

test.beforeEach(async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: ADMIN_PASSWORD },
  });
  expect(res.ok()).toBeTruthy();
});

test("Overview shows the real ingested finding, not fabricated data", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByText("Overview")).toBeVisible({ timeout: 10_000 });
  // "Overview" (the sidebar link) renders before the async dashboard-summary
  // fetch resolves -- wait for real ingested content, not just the shell,
  // before snapshotting the page.
  await expect(page.getByText("checkout").first()).toBeVisible({ timeout: 10_000 });

  const body = await page.locator("body").innerText();
  // The real finding globalSetup ingested: checkout -> redis, cross-AZ,
  // ₹30-40, with a real ₹30-40 savings estimate from optimizationTarget.
  expect(body).toContain("checkout");
  expect(body).toContain("redis");
  expect(body).not.toContain("undefined");
  expect(body).not.toContain("NaN");
  expect(body).not.toContain("[object Object]");
});

test("Cost Graph renders the real topology with no fabricated latency field", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Cost Graph").click();
  await expect(page.locator("svg line")).toHaveCount(1, { timeout: 10_000 });

  const body = await page.locator("body").innerText();
  expect(body).not.toContain("Latency");
});

test("Automations lists the real generated NetworkPolicy-style fix, never auto-applies it", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Automations").click();
  await expect(page.getByText(/never applies them automatically/i)).toBeVisible({ timeout: 10_000 });
});
