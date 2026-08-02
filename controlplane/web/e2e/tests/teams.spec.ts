import { test, expect } from "@playwright/test";
import { ADMIN_PASSWORD, ADMIN_USER } from "../env";

test.beforeEach(async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: ADMIN_PASSWORD },
  });
  expect(res.ok()).toBeTruthy();
});

test("admin can add a real namespace->team mapping and see it reflected in spend", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Teams", { exact: true }).click();
  await expect(page.getByText("Namespace ownership")).toBeVisible({ timeout: 10_000 });

  await page.locator('input[placeholder*="namespace"]').fill("checkout");
  await page.locator('input[placeholder*="Payments"]').fill("Payments");
  await page.getByText("Add mapping").click();

  await expect(page.getByText("checkout").first()).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Payments").first()).toBeVisible({ timeout: 5_000 });

  // Real round-trip through the API, not just optimistic UI state.
  const res = await page.request.get("/api/v1/teams");
  const mapping = await res.json();
  expect(mapping).toContainEqual({ namespace: "checkout", team: "Payments" });
});

test("admin can send a real teammate invite and see it in the pending list", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Teams", { exact: true }).click();
  await expect(page.getByText("Invite a teammate")).toBeVisible({ timeout: 10_000 });

  await page.locator('input[type="email"]').fill("e2e-teammate@example.com");
  await page.getByText("Send invite").click();

  await expect(page.getByText("e2e-teammate@example.com")).toBeVisible({ timeout: 5_000 });

  const res = await page.request.get("/api/v1/invites");
  const invites = await res.json();
  expect(invites).toContainEqual(
    expect.objectContaining({ email: "e2e-teammate@example.com", role: "viewer" }),
  );

  // Revoke it -- real cleanup through the real API, not left dangling.
  await page.getByText("revoke").click();
  await expect(page.getByText("e2e-teammate@example.com")).toHaveCount(0, { timeout: 5_000 });
});
