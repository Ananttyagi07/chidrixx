import { test, expect } from "@playwright/test";
import { ADMIN_PASSWORD, ADMIN_USER } from "../env";

// Real cookie-session login against the real backend (POST
// /api/v1/auth/login) -- context.request shares its cookie jar with the
// browser context, so this exercises the actual auth code, not a stub.
async function loginAsAdmin(context: import("@playwright/test").BrowserContext) {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: ADMIN_PASSWORD },
  });
  expect(res.ok(), `login failed: ${res.status()} ${await res.text()}`).toBeTruthy();
}

test("a valid session cookie reaches the real dashboard, not the landing page", async ({ page, context }) => {
  await loginAsAdmin(context);
  await page.goto("/");
  await expect(page.getByText("Overview")).toBeVisible({ timeout: 10_000 });
});

test("no session shows the real landing page", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByText("View live dashboard")).toBeVisible();
});

test("wrong password is rejected with a real 401, not silently accepted", async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: "definitely-not-the-real-password" },
  });
  expect(res.status()).toBe(401);
});

test("logout clears the real session and returns to landing", async ({ page, context }) => {
  await loginAsAdmin(context);
  await page.goto("/");
  await expect(page.getByText("Overview")).toBeVisible({ timeout: 10_000 });

  await page.getByText("Log out").click();
  await expect(page.getByText("View live dashboard")).toBeVisible({ timeout: 5_000 });

  // The real backend session must actually be gone, not just the UI state.
  const res = await context.request.get("/api/v1/auth/me");
  expect(res.status()).toBe(401);
});
