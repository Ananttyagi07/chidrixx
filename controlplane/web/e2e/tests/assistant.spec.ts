import { test, expect } from "@playwright/test";
import { ADMIN_PASSWORD, ADMIN_USER } from "../env";

// The E2E harness deliberately never sets GROQ_API_KEY (same reasoning as
// Supabase auth in playwright.config.ts's comment): no real secret needed
// to run hermetically in CI. This proves the assistant fails honestly --
// a real 503 surfaced in the UI, not a crash or a silent fake reply --
// when it isn't configured. The real Groq-backed conversation is verified
// separately, by hand, against the actual API (see PROJECT_STATUS.md).
test.beforeEach(async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: ADMIN_PASSWORD },
  });
  expect(res.ok()).toBeTruthy();
});

test("Assistant page shows an honest not-configured state, not a crash", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Assistant", { exact: true }).click();
  await expect(page.getByText("Ask about your real cost data")).toBeVisible({ timeout: 10_000 });

  await page.getByText("What should I fix first, and why?").click();
  await expect(page.getByText(/isn't configured on this control plane/i)).toBeVisible({ timeout: 10_000 });
});

test("a direct POST to /api/v1/chat returns a real 503 when unconfigured", async ({ page }) => {
  const res = await page.request.post("/api/v1/chat", { data: { message: "hi" } });
  expect(res.status()).toBe(503);
});

// The anomaly narrator (controlplane/anomaly_narrator.go) shares the same
// GROQ_API_KEY gate as the chat assistant. globalSetup only ingests one
// snapshot, so there's no real anomaly (needs two) to click "Explain
// this" on through the UI -- covered directly at the API layer instead,
// same honest-503 contract.
test("a direct POST to /api/v1/anomalies/narrate returns a real 503 when unconfigured", async ({ page }) => {
  const res = await page.request.post("/api/v1/anomalies/narrate", { data: { cluster_id: "e2e-cluster" } });
  expect(res.status()).toBe(503);
});

// The AI evaluation panel (controlplane/aieval.go) reports real telemetry
// from real Groq calls -- with no GROQ_API_KEY configured in this E2E
// run, zero of those have ever happened, so the honest state is an empty
// one, not a fabricated "0%" success rate that would misleadingly imply
// real (failing) requests occurred.
test("the AI evaluation panel shows an honest empty state, not a fabricated 0%", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Assistant", { exact: true }).click();
  await expect(page.getByText("No AI requests recorded yet")).toBeVisible({ timeout: 10_000 });

  const res = await page.request.get("/api/v1/ai-eval/stats");
  const body = await res.json();
  expect(body.features).toEqual([]);
});
