// Real end-to-end setup, not a mock: builds the actual controlplane
// binary from source, provisions a real tenant + admin + viewer via the
// real create-tenant/create-user CLI subcommands (the exact same code
// path a real operator uses), starts the real server, and ingests real
// findings via a real HTTP POST to /api/v1/ingest -- the same discipline
// every manual verification pass this project has used all along,
// committed as a real, repeatable suite instead of a one-off script.
import { execFileSync, spawn } from "node:child_process";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { findGo } from "./goBinary";
import { ADMIN_PASSWORD, ADMIN_USER, BASE_URL, BINARY_PATH, DB_PATH, E2E_DIR, INGEST_TOKEN_FILE, PID_FILE, PORT, TENANT_NAME, VIEWER_PASSWORD, VIEWER_USER } from "./env";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const CONTROLPLANE_DIR = path.resolve(__dirname, "../..");

async function waitForServer(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.status < 500) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`server at ${url} did not become ready within ${timeoutMs}ms`);
}

export default async function globalSetup() {
  rmSync(E2E_DIR, { recursive: true, force: true });
  mkdirSync(E2E_DIR, { recursive: true });

  const go = findGo();

  console.log("[e2e] building controlplane binary...");
  execFileSync(go, ["build", "-o", BINARY_PATH, "."], { cwd: CONTROLPLANE_DIR, stdio: "inherit" });

  console.log("[e2e] provisioning real tenant + admin...");
  const createTenantOut = execFileSync(
    BINARY_PATH,
    ["create-tenant", "--db", DB_PATH, "--name", TENANT_NAME, "--admin-user", ADMIN_USER, "--admin-password", ADMIN_PASSWORD],
    { encoding: "utf-8" },
  );
  const tenantIdMatch = createTenantOut.match(/id=(\d+)/);
  const ingestTokenMatch = createTenantOut.match(/ingest token[^:]*:\s*(\S+)/);
  if (!tenantIdMatch || !ingestTokenMatch) {
    throw new Error(`could not parse create-tenant output:\n${createTenantOut}`);
  }
  const tenantID = tenantIdMatch[1];
  const ingestToken = ingestTokenMatch[1];
  // Written to disk so individual spec files (anomaly-watch.spec.ts) can
  // make their own real, isolated /api/v1/ingest calls -- e.g. to build a
  // fresh anomaly on a cluster ID no other test's fixture-value
  // assertions depend on, rather than mutating the shared "e2e-cluster"
  // fixture every other spec file relies on.
  writeFileSync(INGEST_TOKEN_FILE, ingestToken);

  console.log("[e2e] provisioning real viewer user...");
  execFileSync(BINARY_PATH, [
    "create-user", "--db", DB_PATH, "--tenant-id", tenantID,
    "--username", VIEWER_USER, "--password", VIEWER_PASSWORD, "--role", "viewer",
  ]);

  console.log("[e2e] starting real server...");
  const server = spawn(BINARY_PATH, ["-addr", `:${PORT}`, "-db", DB_PATH], {
    stdio: ["ignore", "pipe", "pipe"],
    detached: true,
    // A real 2s proactive-anomaly-watch tick (main.go's 5-minute default,
    // overridden here) -- so the E2E suite can observe a real background
    // detection cycle within a test's timeout instead of waiting 5 real
    // minutes.
    env: { ...process.env, CHIDRIXX_ANOMALY_WATCH_INTERVAL: "2s" },
  });
  server.stdout?.on("data", (d) => process.stdout.write(`[e2e:server] ${d}`));
  server.stderr?.on("data", (d) => process.stderr.write(`[e2e:server] ${d}`));
  writeFileSync(PID_FILE, String(server.pid));

  await waitForServer(BASE_URL, 10_000);

  console.log("[e2e] ingesting real test findings...");
  const auth = Buffer.from(`agent:${ingestToken}`).toString("base64");
  const ingestRes = await fetch(`${BASE_URL}/api/v1/ingest`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Basic ${auth}` },
    body: JSON.stringify({
      cluster_id: "e2e-cluster",
      findings: [
        {
          source: "checkout/checkout-abc", destination: "redis/redis-master",
          path_class: "cross_az", confidence: "high",
          bytes_tx: 5_000_000_000, bytes_rx: 0,
          cost_low_inr: 30, cost_high_inr: 40,
          fix_hint: "co-locate these two workloads in the same zone",
          savings_low_inr: 30, savings_high_inr: 40,
          cloud: "aws", region: "ap-south-1",
        },
        {
          // A real manifest-eligible finding (INTERNET_EGRESS, matching
          // fixengine.go's real scope) so the remediation-preview E2E
          // test exercises the real "would apply" path, not only "would
          // skip" -- the cross_az fixture above never gets a real
          // manifest, by design (see remediation_test.go's comment).
          // The manifest text below is the real, complete shape
          // fixengine.go's networkPolicyDenyDestination actually emits
          // (podSelector/policyTypes/egress/ipBlock/except included), not
          // a truncated stand-in -- traffic_replay.go really parses this
          // to find the blocked destination, so an abbreviated fixture
          // would be testing a manifest the product never generates.
          source: "checkout/checkout-abc", destination: "203.0.113.5",
          path_class: "internet_egress", confidence: "high",
          bytes_tx: 2_000_000_000, bytes_rx: 0,
          cost_low_inr: 15, cost_high_inr: 20,
          fix_hint: "deny this destination if it's not required",
          fix_manifest: 'apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: deny-203-0-113-5\n  namespace: checkout\nspec:\n  podSelector: {}   # narrow this to the specific workload\'s own labels\n  policyTypes: [Egress]\n  egress:\n    - to:\n        - ipBlock:\n            cidr: 0.0.0.0/0\n            except: ["203.0.113.5/32"]\n',
          savings_low_inr: 15, savings_high_inr: 20,
          cloud: "aws", region: "ap-south-1",
        },
        {
          // A real collateral-risk case for traffic_replay.go: same real
          // shape as the qualifying fix above, but a SECOND real workload
          // in the same real namespace (below) also talks to this exact
          // destination. Because the generated policy uses
          // podSelector: {} it would block that one too, so this must be
          // disqualified despite clearing every other policy bar.
          source: "checkout/checkout-abc", destination: "198.51.100.9",
          path_class: "internet_egress", confidence: "high",
          bytes_tx: 1_000_000_000, bytes_rx: 0,
          cost_low_inr: 8, cost_high_inr: 12,
          fix_hint: "deny this destination if it's not required",
          fix_manifest: 'apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: deny-198-51-100-9\n  namespace: checkout\nspec:\n  podSelector: {}   # narrow this to the specific workload\'s own labels\n  policyTypes: [Egress]\n  egress:\n    - to:\n        - ipBlock:\n            cidr: 0.0.0.0/0\n            except: ["198.51.100.9/32"]\n',
          savings_low_inr: 8, savings_high_inr: 12,
          cloud: "aws", region: "ap-south-1",
        },
        {
          // The real collateral itself: a genuinely different workload in
          // the same real namespace, depending on the same real
          // destination. Deliberately carries no fix_hint, so it is not
          // its own remediation decision -- it exists purely as the real
          // traffic that makes the fix above unsafe to auto-apply.
          source: "checkout/reporting-xyz", destination: "198.51.100.9",
          path_class: "internet_egress", confidence: "high",
          bytes_tx: 400_000_000, bytes_rx: 0,
          cost_low_inr: 3, cost_high_inr: 5,
          cloud: "aws", region: "ap-south-1",
        },
      ],
    }),
  });
  if (!ingestRes.ok) {
    throw new Error(`test ingest failed: HTTP ${ingestRes.status}`);
  }

  console.log("[e2e] setup complete");
}
