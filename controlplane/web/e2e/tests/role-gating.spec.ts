import { test, expect } from "@playwright/test";
import { VIEWER_PASSWORD, VIEWER_USER } from "../env";

test.beforeEach(async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: VIEWER_USER, password: VIEWER_PASSWORD },
  });
  expect(res.ok()).toBeTruthy();
});

test("a viewer sees the real role in the sidebar and no budget-edit control", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByText("Overview")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("viewer", { exact: true })).toBeVisible();

  await page.getByText("Budgets", { exact: true }).click();
  await expect(page.getByText("Ask an admin to set one.")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Set a budget")).toHaveCount(0);
});

test("a viewer's direct POST to /api/v1/budget is rejected server-side with a real 403", async ({ page }) => {
  const res = await page.request.post("/api/v1/budget", {
    data: { budget_inr: 999999 },
  });
  expect(res.status()).toBe(403);
});

test("a viewer's direct POST to /api/v1/invites is rejected -- team management stays admin-only", async ({ page }) => {
  const res = await page.request.post("/api/v1/invites", {
    data: { email: "someone@example.com", role: "viewer" },
  });
  expect(res.status()).toBe(403);
});

test("a viewer's direct GET to /api/v1/invites is also rejected -- admin-only includes visibility", async ({ page }) => {
  const res = await page.request.get("/api/v1/invites");
  expect(res.status()).toBe(403);
});
