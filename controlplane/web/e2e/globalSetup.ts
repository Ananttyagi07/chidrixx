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
import { ADMIN_PASSWORD, ADMIN_USER, BASE_URL, BINARY_PATH, DB_PATH, E2E_DIR, PID_FILE, PORT, TENANT_NAME, VIEWER_PASSWORD, VIEWER_USER } from "./env";

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

  console.log("[e2e] provisioning real viewer user...");
  execFileSync(BINARY_PATH, [
    "create-user", "--db", DB_PATH, "--tenant-id", tenantID,
    "--username", VIEWER_USER, "--password", VIEWER_PASSWORD, "--role", "viewer",
  ]);

  console.log("[e2e] starting real server...");
  const server = spawn(BINARY_PATH, ["-addr", `:${PORT}`, "-db", DB_PATH], {
    stdio: ["ignore", "pipe", "pipe"],
    detached: true,
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
      ],
    }),
  });
  if (!ingestRes.ok) {
    throw new Error(`test ingest failed: HTTP ${ingestRes.status}`);
  }

  console.log("[e2e] setup complete");
}
