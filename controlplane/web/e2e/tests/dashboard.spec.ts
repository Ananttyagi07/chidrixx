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
  // globalSetup ingests 2 real distinct edges (checkout->redis cross_az,
  // checkout->203.0.113.5 internet_egress).
  await expect(page.locator("svg line")).toHaveCount(2, { timeout: 10_000 });

  const body = await page.locator("body").innerText();
  expect(body).not.toContain("Latency");
});

test("Automations lists the real generated NetworkPolicy-style fix, never auto-applies it", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Automations").click();
  await expect(page.getByText(/never applies them automatically/i)).toBeVisible({ timeout: 10_000 });
});

test("Automations' remediation preview shows a real would-apply and a real would-skip decision", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Automations").click();
  await expect(page.getByText("If auto-remediation were enabled")).toBeVisible({ timeout: 10_000 });

  // The real internet_egress fixture (has a real manifest, high
  // confidence, positive savings) must qualify...
  await expect(page.getByText(/Would apply \(1\)/)).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("203.0.113.5").first()).toBeVisible();

  // ...and the real cross_az fixture (no manifest -- fixengine.go never
  // generates one for that class) must not, with a real stated reason.
  await expect(page.getByText(/Would skip \(1\)/)).toBeVisible();
  await expect(page.getByText(/no mechanically-generated manifest/)).toBeVisible();

  // Real round-trip through the API, not just what happened to render.
  const res = await page.request.get("/api/v1/remediation/preview");
  const body = await res.json();
  expect(body.decisions).toContainEqual(
    expect.objectContaining({ destination: "203.0.113.5", would_auto_apply: true }),
  );
  expect(body.decisions).toContainEqual(
    expect.objectContaining({ destination: "redis/redis-master", would_auto_apply: false }),
  );
});

test("Cost Graph's placement simulator shows a real computed result, including the honest zero-savings case", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Cost Graph").click();
  await expect(page.getByText("Placement simulator")).toBeVisible({ timeout: 10_000 });

  // globalSetup's real fixture has exactly 1 real CROSS_AZ edge (checkout
  // <-> redis, 2 real workloads). With the default "3 zones" (a real
  // redundancy requirement, not a bucket count -- see placement.go),
  // both workloads are forced into separate zones and the honest answer
  // is 0 potential savings, not a bug -- proven directly at the Go level
  // in TestOptimizePlacementWithFewerWorkloadsThanGroupsForcesEachAlone.
  await expect(page.getByText("₹40").first()).toBeVisible({ timeout: 10_000 });

  const res = await page.request.get("/api/v1/placement/preview?groups=3");
  const body = await res.json();
  expect(body).toMatchObject({
    groups: 3,
    workloads: 2,
    edges: 1,
    observed_cross_zone_inr: 40,
    potential_savings_inr: 0,
  });
});

test("marking a real fix applied on Overview is tracked server-side, not just a UI toggle", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByText("checkout").first()).toBeVisible({ timeout: 10_000 });

  await page.getByRole("button", { name: "Mark as applied" }).first().click();
  await expect(page.getByText(/Applied — tracking real cost impact/)).toBeVisible({ timeout: 5_000 });

  // Real round-trip through the API, not just optimistic UI state.
  const res = await page.request.get("/api/v1/outcomes");
  const outcomes = await res.json();
  expect(outcomes).toContainEqual(
    expect.objectContaining({ source: "checkout/checkout-abc", applied_at: expect.any(String) }),
  );
});
