import { test, expect } from "@playwright/test";
import { ADMIN_PASSWORD, ADMIN_USER } from "../env";

test.beforeEach(async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: ADMIN_PASSWORD },
  });
  expect(res.ok()).toBeTruthy();
});

// globalSetup ingests exactly one real snapshot, so the deep forecast
// (controlplane/forecast.go) has genuinely too little history to compute
// -- this proves the real "not enough data" path renders honestly rather
// than crashing or showing a fabricated number, and that a real API
// round trip is what drives it, not client-side state.
test("Deep forecast honestly reports too little history with only one real snapshot", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Forecasting", { exact: true }).click();
  await expect(page.getByText("Deep forecast", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(/Not enough real history for this cluster yet/i)).toBeVisible({ timeout: 10_000 });

  const res = await page.request.get("/api/v1/forecast?cluster_id=e2e-cluster");
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(body.available).toBe(false);
});

test("a direct GET to /api/v1/forecast without cluster_id returns a real 400", async ({ page }) => {
  const res = await page.request.get("/api/v1/forecast");
  expect(res.status()).toBe(400);
});
