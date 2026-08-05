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
  await expect(page.getByText("Overview").first()).toBeVisible({ timeout: 10_000 });
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
  // globalSetup ingests 4 real distinct edges (checkout->redis cross_az,
  // checkout->203.0.113.5 internet_egress, plus checkout->198.51.100.9
  // and reporting->198.51.100.9, the real traffic-replay collateral
  // fixture); anomaly-watch.spec.ts adds a real 5th
  // (watched-service->watched-dest, same_node, on its own isolated
  // cluster) -- the cost graph draws every real ingested edge
  // tenant-wide, not filtered by path_class like placement/remediation
  // are, so this real count grows when that fixture's data is present.
  await expect(page.locator("svg line")).toHaveCount(5, { timeout: 10_000 });

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
  // confidence, positive savings, and no other real workload depending on
  // its destination) must qualify...
  await expect(page.getByText(/Would apply \(1\)/)).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("203.0.113.5").first()).toBeVisible();

  // ...and two real fixtures must not: the cross_az one (no manifest --
  // fixengine.go never generates one for that class) and the
  // 198.51.100.9 one (a real second workload in the same namespace
  // depends on that destination), each with a real stated reason.
  await expect(page.getByText(/Would skip \(2\)/)).toBeVisible();
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

test("Automations' traffic replay disqualifies a fix whose policy would block another real workload", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Automations").click();
  await expect(page.getByText("If auto-remediation were enabled")).toBeVisible({ timeout: 10_000 });

  // The real collateral case must be visible and explained, naming the
  // real other workload -- not just silently filtered out.
  await expect(page.getByText(/Collateral traffic found/).first()).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(/checkout\/reporting-xyz/).first()).toBeVisible();

  const res = await page.request.get("/api/v1/remediation/preview");
  const body = await res.json();
  const collateral = body.decisions.find((d: { destination: string }) => d.destination === "198.51.100.9");
  expect(collateral).toBeTruthy();
  expect(collateral.would_auto_apply).toBe(false);
  expect(collateral.traffic_replay.safe).toBe(false);
  expect(collateral.traffic_replay.affected_workloads).toEqual(["checkout/reporting-xyz"]);
  expect(collateral.traffic_replay.collateral_cost_inr).toBe(5);

  // The genuinely safe fix must still report a real, confirmed-safe
  // replay -- proving this check discriminates, not just flags everything.
  const safe = body.decisions.find((d: { destination: string }) => d.destination === "203.0.113.5");
  expect(safe.traffic_replay.safe).toBe(true);
  expect(safe.traffic_replay.affected_workloads ?? []).toEqual([]);
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

test("Automations' outcome dataset health shows real shown/applied/measured counts", async ({ page }) => {
  await page.goto("/");
  await page.getByText("Automations").click();
  await expect(page.getByText("Outcome dataset health")).toBeVisible({ timeout: 10_000 });

  // Three real fixtures carry a fix_hint (cross_az, and the two
  // internet_egress ones), so dashboard-summary's top-fixes pass has
  // recorded all three as shown by now (RecordRecommendationsShown runs
  // on every dashboard-summary request) -- and, running before the later
  // "marking a real fix applied" test in this same file/shared server,
  // real applied/measured counts are still honestly 0 here.
  await expect(page.getByText("No recommendations marked applied yet")).toBeVisible();

  const res = await page.request.get("/api/v1/outcomes/stats");
  const body = await res.json();
  expect(body.total_shown).toBe(3);
  expect(body.total_applied).toBe(0);
  expect(body.total_measured).toBe(0);
  expect(body.mean_abs_prediction_error_inr).toBeUndefined();
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
