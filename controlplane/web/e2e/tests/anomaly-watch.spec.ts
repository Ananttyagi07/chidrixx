import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { ADMIN_PASSWORD, ADMIN_USER, INGEST_TOKEN_FILE } from "../env";

test.beforeEach(async ({ context }) => {
  const res = await context.request.post("/api/v1/auth/login", {
    data: { username: ADMIN_USER, password: ADMIN_PASSWORD },
  });
  expect(res.ok()).toBeTruthy();
});

// Real proactive detection (controlplane/anomaly_watch.go), not the
// existing pull-only path: ingests two real snapshots (a real 10x jump)
// on a cluster ID no other spec file's fixture-value assertions touch --
// same_node path_class and no fix_hint, so it's invisible to the
// remediation-preview and placement-simulator tests' exact aggregates
// elsewhere in this suite. Waits for the real background watch loop
// (CHIDRIXX_ANOMALY_WATCH_INTERVAL=2s in this E2E run, see
// globalSetup.ts) to actually tick before checking, rather than
// asserting on a race.
test("a real proactive anomaly is detected and surfaced without the operator asking", async ({ page, context }) => {
  const token = readFileSync(INGEST_TOKEN_FILE, "utf-8").trim();
  const auth = Buffer.from(`agent:${token}`).toString("base64");
  const clusterID = "anomaly-watch-e2e-cluster";

  async function ingest(cost: number) {
    const res = await context.request.post("/api/v1/ingest", {
      headers: { Authorization: `Basic ${auth}` },
      data: {
        cluster_id: clusterID,
        findings: [
          {
            source: "ns/watched-service", destination: "ns/watched-dest",
            path_class: "same_node", confidence: "high",
            bytes_tx: 1000, bytes_rx: 0, cost_low_inr: cost, cost_high_inr: cost,
          },
        ],
      },
    });
    expect(res.ok()).toBeTruthy();
  }

  await ingest(2);
  await new Promise((r) => setTimeout(r, 1100)); // distinct real reported_at (second-granularity)
  await ingest(20); // a real 10x jump, well above the 2x anomaly threshold

  // Give the real background watch loop at least one real tick.
  await new Promise((r) => setTimeout(r, 2500));

  await page.goto("/");
  const bell = page.getByTitle("Proactively detected anomalies");
  await expect(bell).toBeVisible({ timeout: 10_000 });

  await bell.click();
  const panel = page.getByTestId("anomaly-alert-panel");
  await expect(panel.getByText(clusterID)).toBeVisible({ timeout: 10_000 });
  await expect(panel.getByText(/10\.0x/)).toBeVisible();

  await panel.getByRole("button", { name: "Dismiss" }).click();
  await expect(panel.getByText("No new anomalies since the last check.")).toBeVisible({ timeout: 5_000 });

  // Real round-trip confirmation, not just optimistic UI state.
  const res = await page.request.get("/api/v1/anomalies/alerts");
  const body: Array<{ cluster_id: string }> = await res.json();
  expect(body.find((a) => a.cluster_id === clusterID)).toBeUndefined();
});

test("an acknowledged alert stays gone across a later real watch tick, not just immediately", async ({ page }) => {
  // Runs after the test above, which already acknowledged this exact
  // cluster's alert. The real risk this guards against: the background
  // loop's next tick re-inserting (or somehow re-surfacing) the same
  // already-dismissed alert because the underlying snapshot is still
  // there and still "anomalous" -- ON CONFLICT DO NOTHING (keyed on
  // snapshot_reported_at) must make this a real permanent dismissal, not
  // one that silently reappears on the next real 2s tick.
  await new Promise((r) => setTimeout(r, 2500));
  await page.goto("/");
  const res = await page.request.get("/api/v1/anomalies/alerts");
  const body: Array<{ cluster_id: string }> = await res.json();
  expect(body.find((a) => a.cluster_id === "anomaly-watch-e2e-cluster")).toBeUndefined();
});
