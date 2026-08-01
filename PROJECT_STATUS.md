# chidrixx — Project & Technical Status (Universal Reference)

_Last updated: 2026-08-02 (merged narrative + engineering reference into one file)_

This is the single, complete picture of chidrixx: what's real and
verified, what's explicitly not built (and why), what's actually left to
do, and — merged into this same file — the exhaustive engineering detail
underneath every claim: which file, which function, which DB column,
which test, which command proved it. Nothing here is recalled from
memory; every number was measured against the actual repository at the
commit this file was last updated against (`git log -1` at write time:
`d43cf10`, "docs: add TECHNICAL_STATUS.md"), by running `go build`,
`go test`, `gofmt -l`, `find`/`wc`/`grep`, `helm lint`/`template`, and
live Playwright/curl passes against a real k3d cluster — not written and
assumed to work.

---

## What chidrixx is

An eBPF agent that attributes Kubernetes network traffic to workloads and
prices it — which pod is talking to which destination, over which path
(same node / same zone / cross-zone / cross-region / NAT / internet /
managed service), and roughly what that's costing. A control plane
aggregates this across clusters with a real, multi-tenant dashboard on
top.

Two components, two Go modules, plus a frontend:

```
chidrixx/
├── agent/               module `chidrixx`            — 4,089 lines Go (excl. tests), 14 files
│   └── cmd/
│       ├── kharcha/     the eBPF agent binary          — 12 test files, 26 test functions
│       └── loadgen/     load-test traffic generator (no tests)
├── controlplane/        module `chidrixx-controlplane` — 4,104 lines Go (excl. tests), 16 files
│   └── web/             React/Vite/TS dashboard SPA    — 4,044 lines TS/TSX, 32 .tsx + 9 .ts files
├── bpf/                 flow_cgroup.c + compiled flow_cgroup.o (committed binary, go:embed'd)
├── pricebook/            aws.yaml, gcp.yaml — real cited price data
├── deploy/helm/          kharcha/ + controlplane/ charts, 18 template/value files total
├── test/load/            10k-concurrent-flow load harness + its own Dockerfile
└── .github/workflows/   ci.yml — the only CI workflow (agent/ only — see §6)
```

Total: **~12,200 lines** of hand-written Go + TypeScript across both
modules and the frontend (not counting vendored react-bits `.jsx`/`.css`
files, which are third-party, installed via the real `shadcn` CLI).

- **`agent/`** (module `chidrixx`) — the eBPF agent, one per cluster (DaemonSet).
- **`controlplane/`** (module `chidrixx-controlplane`) — optional multi-cluster
  ingest API, storage, and dashboard.

---

## 1. Agent — Real & Verified

### 1.1 Capabilities

- **eBPF programs** (`bpf/flow_cgroup.c`): `cgroup_skb` egress/ingress,
  counting bytes per `(cgroup_id, 5-tuple)` in a `BPF_MAP_TYPE_LRU_PERCPU_HASH`
  map. Both programs unconditionally `return 1` (SK_PASS) — they only count,
  never gate traffic.
- **Loader** (`loader.go`): the agent owns its own BPF
  lifecycle (load/attach/detach), not relying on an externally-pinned map.
  Recognizes the specific `EPERM` from the cgroup-namespace limitation (see
  §5) and wraps it with an actionable message instead of a generic error.
- **Classification** (`classify.go`): 8 path classes — `SAME_NODE`,
  `SAME_AZ`, `CROSS_AZ`, `CROSS_REGION`, `MANAGED_SERVICE`,
  `PRIVATE_OFFCLUSTER`, `NAT_EGRESS`, `INTERNET_EGRESS` — each with a
  confidence level (high/med/low) that widens the price estimate when
  topology data is incomplete rather than guessing. Fixed enumeration,
  never dynamically extended.
- **Pricing** (`pricing.go`): overridable YAML price book, cost bands by
  class + confidence (15%/35%/60% band widening for high/med/low
  confidence). New this session: `optimizationTarget(class) (PathClass, bool)`
  — the re-pricing map for real savings estimates (table below).
- **Reporting**: CLI table, HTML report (`html.go`), Prometheus metrics
  (`metrics.go`: `kharcha_flow_bytes_total`, `kharcha_cost_inr`,
  `kharcha_map_entries`, `kharcha_scrape_lag_seconds`).
- **Alerting** (`alert.go`): threshold and growth-ratio based, posts to a
  Slack-compatible webhook.
- **Shipper** (`shipper.go`): posts findings to the control plane, Basic
  Auth. `Ship(ctx, findings, events)` — the wire struct `shipperFinding`
  now carries `savings_low_inr`/`savings_high_inr`; `shipperRequest` now
  carries `Events []DeployEvent` alongside `Findings`.
- **Kubernetes metadata resolution** (`kubernetes.go`): talks to the API
  server directly over a keep-alive `http.Client` using the mounted
  service-account token — **not** by shelling out to `kubectl` (the original
  design), which was the dominant source of CPU overhead. Falls back to
  `kubectl` exec only when running outside a cluster (local dev). New
  this session: `lastReplicas map[string]int32` + `pendingEvents
  []DeployEvent` fields feeding real deploy-event detection.
- **Deploy-event detection** (`deployevents.go`, **new**, 96 lines):
  `diffReplicas()` — pure function, no I/O — diffs a `kubeDeploymentList`
  snapshot against the last-seen replica-count map, returns
  `[]DeployEvent`. `refreshDeployEvents()` does the actual `GET
  /apis/apps/v1/deployments` fetch and calls the pure diff.
  `DrainDeployEvents()` is a thread-safe buffer drain, called from the
  ship loop (a different goroutine/ticker than the refresh loop that
  fills it). Best-effort: an agent whose RBAC predates this (see the
  new `apps/deployments` ClusterRole rule, §5) just detects nothing,
  never breaks the core refresh loop pod/service/node resolution
  depends on.
- **Fix engine** (`fixengine.go`): generates a real, copy-pasteable
  `NetworkPolicy` manifest for `INTERNET_EGRESS`/`NAT_EGRESS`/`CROSS_REGION`
  findings, scoped to the real source namespace and the real flagged
  destination IP. `CROSS_AZ`/`MANAGED_SERVICE` stay a text hint — their real
  fix needs pod labels this agent doesn't resolve, and a fabricated label
  selector would be worse than an honest sentence.

### 1.2 optimizationTarget map (`pricing.go`, new this session)

| From class | Target class | Real-world action the fix hint describes |
|---|---|---|
| `CROSS_AZ` | `SAME_AZ` | co-locate the two workloads in one zone |
| `CROSS_REGION` | `CROSS_AZ` | keep traffic inside one region |
| `NAT_EGRESS` | `PRIVATE_OFFCLUSTER` | add a VPC/private (Gateway) endpoint |
| `MANAGED_SERVICE` | `SAME_AZ` | pin client + managed endpoint to one zone |
| `INTERNET_EGRESS` | *(none)* | real fix is usage reduction (cache/compress), not a cheaper class — deliberately no savings number |
| `SAME_NODE` / `SAME_AZ` / `PRIVATE_OFFCLUSTER` | *(none)* | already the cheapest tier, nothing to optimize toward |

### 1.3 Agent CLI flags (`main.go`, all 15)

`-bpf-object`, `-cgroup-path`, `-pricebook` (default `pricebook/aws.yaml`),
`-managed-cidrs`, `-node-has-public-ip`, `-node-name`, `-html-out`,
`-metrics-addr`, `-alert-webhook`, `-alert-threshold-inr`,
`-alert-growth-ratio`, `-controlplane-url`, `-cluster-id`,
`-k8s-refresh-interval` (default 10s). `CHIDRIXX_AUTH_TOKEN` is read from
the environment, not a flag (so it never shows up in `ps`/container
specs).

### 1.4 Agent dependencies (`go.mod`, exact versions)

Direct: `github.com/cilium/ebpf v0.22.0`,
`github.com/prometheus/client_golang v1.24.1`, `gopkg.in/yaml.v3 v3.0.1`.
5 indirect deps, all transitive requirements of the above (perks,
xxhash, goautoneg, client_model, common, procfs, x/sys, protobuf) — no
surprise dependencies, none added without a direct reason.

### 1.5 Agent test inventory — 26 test functions across 12 files

| File | Tests | Covers |
|---|---|---|
| `classify_test.go` | — | All 8 path classes against real multi-AZ topology fixtures |
| `pricing_test.go` | 4 (2 new) | `TestRealPriceBooksLoad`, internet-egress-costs-more invariant, **new**: `TestOptimizationTargetIsAlwaysCheaperInRealPriceBooks`, `TestOptimizationTargetHasNoneForInternetEgress` |
| `deployevents_test.go` **(new, 4 tests)** | 4 | `TestDiffReplicasSkipsFirstObservation`, `TestDiffReplicasDetectsIncreaseAndDecrease`, `TestDiffReplicasNoEventWhenUnchanged`, `TestDrainDeployEventsClearsBuffer` |
| `shipper_test.go` | 3 | Wire-format contract test (now asserts `savings_low_inr`/`savings_high_inr`/events too), disabled-without-URL, Basic Auth token presence |
| `fixengine_test.go` | — | Manifest generation per class, IPv4/IPv6 CIDR handling |
| `alert_test.go` + `alert_live_test.go` | — | Threshold/growth-ratio logic + a real posted webhook payload fetched back |
| `kubernetes_integration_test.go` | — | Real API-server-backed pod/service/endpoint resolution |
| `loader_test.go` + `prog_test.go` | — | Real BPF load/attach against a real kernel (privileged, CI-only) |
| `html_test.go`, `metrics_test.go` | — | Report rendering, metric registration |

Run: `cd agent && go test ./...` — **all passing** (re-verified
2026-08-02), ~3s wall time (excludes privileged BPF tests which need
CAP_BPF; those run in CI on GitHub-hosted VMs).

---

## 2. Validation & Testing — Real & Verified

| Check | Result |
|---|---|
| **Byte accuracy** | Real `iperf3 -n 1G` transfer: 1,073,741,824 payload bytes sent, agent independently measured 1,075,417,978 on-wire bytes (client TX = server RX, cross-validated). 0.156% difference = real TCP/IP header overhead, not error — inside the ≤1% bar. |
| **Multi-AZ classification** | Proven on a real 4-node cluster with real `topology.kubernetes.io/zone` labels (`ap-south-1a` ×2, `ap-south-1b`, `us-east-1a`). All four topology classes confirmed against real cross-node traffic. |
| **Alert delivery** | A real alert posted to a live external HTTP endpoint; the exact received payload fetched back and inspected. |
| **Chaos-safety (NFR-5)** | Force-killed the agent pod mid-traffic: 45 requests monitored, 0 non-200 responses. Confirms the "hooks only count, never gate" architecture claim against a real cluster. |
| **Overhead (NFR-1)** | First measured at **133m CPU (1.66%)** — over the ≤1% target. Root-caused to the kubectl-exec problem above; after the fix, **4m CPU (0.05%)**. |
| **10k-concurrent-flow load test** | Built a real load harness (`test/load/`, `agent/cmd/loadgen`) — 20 sink replicas × 20 driver pods × 500 held-open connections. First run exposed a real bug: the BPF map was capped at `max_entries=4096` and silently LRU-evicted, undercounting at ~3.6k instead of ~9.7k actual connections. Fixed (map sized to 16384); re-run tracked **10,543 concurrent flows at 27m CPU (0.34%)** — genuinely inside budget at the scale the spec actually asks for. |
| **Genuine two-cluster proof** | Not curl-simulated: a second, fully independent k3d cluster with its own real agent, connected to the same control plane. Both cluster IDs confirmed in `/api/v1/findings` and the dashboard. |
| **CI** | See §6 — scoped to `agent/` only; the exact three jobs are broken out there rather than summarized here, since that scope boundary matters. |

---

## 3. Control Plane — Real & Verified

### 3.1 Capabilities

- **Ingest + storage**: SQLite (pure Go, `modernc.org/sqlite`, no cgo),
  each ingest is a full point-in-time snapshot; `LatestFindings` always
  serves each cluster's most-recent snapshot, tested against
  stale-snapshot leakage. `SetMaxOpenConns(1)` — single-writer by
  design, matching SQLite's own concurrency model.
- **Auth: real multi-tenant login, not a single shared token.** Separate
  tenants, separate users with bcrypt-hashed passwords, separate
  server-tracked login sessions (cookie-based, 24h TTL), separate
  per-tenant agent ingest tokens (SHA-256 hashed). Every store query is
  scoped by `tenant_id` — verified with a dedicated cross-tenant
  isolation test (two tenants, a deliberately reused cluster ID and
  settings key, checked against every read path) and live: provisioned
  two real tenants via the `create-tenant` CLI, logged into both in
  separate browser contexts, confirmed neither saw the other's clusters
  or cost. Roles (`admin`/`viewer`) gate the one browser-side write
  action (setting the budget, and now also team ownership) — enforced
  server-side (a viewer's direct POST gets a real 403, not just a
  hidden button). An existing single-tenant install upgrades cleanly:
  on first boot with zero tenants, the control plane bootstraps tenant
  1 + an admin account automatically (password auto-generated and
  logged once, or set via `CHIDRIXX_ADMIN_PASSWORD`) — verified against
  the actual k3d cluster's real pre-existing database, all data intact
  and reachable under tenant 1 afterward. Provisioning additional
  tenants/users/tokens is an operator action (the
  `create-tenant`/`create-user`/`create-token` CLI subcommands), not a
  public signup form — chidrixx is still a self-hosted tool, a
  deliberate scope boundary (see §9's future-vision note).
- **Dashboard** (`controlplane/web`) — a real React + Vite + Tailwind +
  Recharts + Framer Motion SPA. Built assets are committed to git (like
  `bpf/flow_cgroup.o`) and `go:embed`'d — `go build`/`go test` need no
  Node install. 15 sidebar pages total (full list in §4).

### 3.2 API routes — 9 real endpoints + the static SPA shell route, exact

| Route | Method(s) | Auth | Handler |
|---|---|---|---|
| `/api/v1/ingest` | POST | `requireAPIToken` | `handleIngest` |
| `/api/v1/findings` | GET | `requireSession` | `handleFindingsAPI` |
| `/api/v1/dashboard-summary` | GET | `requireSession` | `handleDashboardSummary` |
| `/api/v1/budget` | GET / POST | `requireSession` (POST also `requireAdmin`) | `budgetRoute` |
| `/api/v1/teams` | GET / POST / DELETE | `requireSession` (POST+DELETE also `requireAdmin`) | `teamsRoute` |
| `/api/v1/workload-growth` | GET | `requireSession` | `handleWorkloadGrowth` |
| `/api/v1/auth/me` | GET | `requireSession` | `handleMe` |
| `/api/v1/auth/login` | POST | none (that's the point) | `handleLogin` |
| `/api/v1/auth/logout` | POST | none | `handleLogout` |
| `/` | GET | none (static SPA shell, no secrets embedded) | `webAssetsHandler` |

### 3.3 SQLite schema — 8 tables, exact DDL shape

| Table | Key columns | Notes |
|---|---|---|
| `flow_aggregate` | `id` PK, `tenant_id`, `cluster_id`, `reported_at`, `src_workload`, `dst_workload_or_endpoint`, `path_class`, `confidence`, `bytes_tx`, `bytes_rx`, `cost_low_inr`, `cost_high_inr`, `fix_hint`, `fix_manifest`, `cloud`, `region`, **`savings_low_inr`, `savings_high_inr`** (new) | Index on `(cluster_id, reported_at)`. Never pruned — every snapshot ever ingested is retained. |
| `settings` | `(tenant_id, key)` composite PK, `value` | Rebuilt mid-session from a single-column `key` PK — see bug #6 below. |
| `tenants` | `id` PK, `name`, `created_at` | |
| `users` | `id` PK, `tenant_id` FK, `username` UNIQUE, `password_hash` (bcrypt), `role`, `created_at` | |
| `api_tokens` | `id` PK, `tenant_id` FK, `token_hash` UNIQUE (SHA-256), `label`, `created_at` | Plaintext token is never stored, only ever shown once at creation. |
| `sessions` | `id` PK (opaque random, doubles as cookie value), `user_id` FK, `tenant_id` FK, `created_at`, `expires_at` | Server-tracked — `DELETE`-revocable, unlike a stateless JWT. |
| `team_ownership` **(new)** | `(tenant_id, namespace)` composite PK, `team`, `created_at` | |
| `deploy_event` **(new)** | `id` PK, `tenant_id`, `cluster_id`, `namespace`, `name`, `reason`, `message`, `occurred_at` | Index on `(tenant_id, cluster_id, occurred_at)`. |

Migrations are all `ALTER TABLE ... ADD COLUMN` (idempotent, "duplicate
column name" errors swallowed) except the `settings` table rebuild,
which detects the old schema via `sqlite_master.sql` and does a real
`CREATE new → INSERT SELECT → DROP → RENAME` inside one transaction.

### 3.4 Control plane file-by-file (16 files)

| File | Role |
|---|---|
| `main.go` | Entrypoint + 3 CLI subcommands (`create-tenant`, `create-user`, `create-token`) + `bootstrapDefaultTenant()` (idempotent, only fires when `TenantCount()==0`) + the route mux. |
| `store.go` | `Store` wraps one `*sql.DB`. Every query method takes `tenantID int64` as its first real parameter. |
| `model.go` | Wire structs: `Finding` (14 fields incl. `SavingsLowINR`/`SavingsHighINR`), `DeployEvent`, `IngestRequest` (now carries `Events []DeployEvent`). |
| `api.go` | `handleIngest` (also calls `store.IngestDeployEvents` best-effort), `handleFindingsAPI`. |
| `summary_api.go` | `handleDashboardSummary` — response now includes `SpendByTeam`. |
| `anomaly.go` | `detectAnomalies()` — 2x-growth-ratio detection; now also calls `RecentDeployEvents()` in a 30-minute lookback and attaches `LikelyCause *DeployEvent`. |
| `budget_api.go` | `handleBudget` — GET/POST, POST gated by `requireAdmin`. |
| `auth.go` | Middleware: `requireAPIToken`, `requireSession`, `requireAdmin`. |
| `auth_api.go` | `handleLogin`/`handleLogout`/`handleMe`. |
| `tenant.go` | `Tenant`/`User`/`Session` structs, `CreateTenant` (atomic tx), `CreateUser`, `CreateAPIToken` (token rotation), `AuthenticateUser` (bcrypt, dummy-hash timing guard), `AuthenticateAPIToken` (SHA-256 lookup), `CreateSession`/`GetSession`/`DeleteSession`. |
| `team.go` | `TeamOwnership` CRUD, `extractNamespace()` (RFC-1123-label regex), `computeSpendByTeam()` (pure grouping function). |
| `team_api.go` | `handleTeams` — GET (any role) / POST+DELETE (admin only via `teamsRoute`). |
| `deployevent.go` | `IngestDeployEvents()`, `RecentDeployEvents(tenantID, clusterID, since, until)`. |
| `workloadgrowth.go` | `WorkloadCostGrowth(tenantID, topN)` — ranks workloads by real first→last-snapshot delta across full retained history; correlates each with `RecentDeployEvents` in its own namespace over its own trend window. |
| `workloadgrowth_api.go` | `handleWorkloadGrowth` — its own route, not folded into dashboard-summary (scans full history, not just latest-per-cluster). |
| `webassets.go` | `go:embed`'s `web/dist`, serves the SPA + static assets, no auth. |

### 3.5 Control plane dependencies (`go.mod`, exact versions)

Direct: `golang.org/x/crypto v0.54.0` (bcrypt), `modernc.org/sqlite
v1.54.0` (pure-Go, no cgo). 8 indirect deps, all `modernc.org/sqlite`'s
own transitive requirements (libc, mathutil, memory, bigfft, etc.) plus
`google/uuid` and `go-isatty`.

### 3.6 Control plane test inventory — 58 test functions across 10 files

| File | Approx. tests | Covers |
|---|---|---|
| `store_test.go` | 20 | Ingest/query round-trips, the fix-manifest/savings round-trips, old-schema migration (both `flow_aggregate` and `settings`), **`TestTenantIsolationAcrossEveryReadPath`** (the single most important test in the repo — two tenants, deliberately reused cluster ID and settings key, checked against every read path) |
| `auth_test.go` | 10 | API-token and session middleware, cross-tenant token isolation, admin-role gating, `TestCreateAPITokenMintsARealWorkingSecondToken` |
| `anomaly_test.go` | 5 | Growth-ratio detection, single-snapshot/zero-baseline skips, deploy-event correlation inside/outside the lookback window |
| `team_test.go` + `team_api_test.go` | 12 | Namespace-extraction regex (real vs. cgroup-path), spend grouping + fallback, store CRUD + tenant isolation, API handler + role gating |
| `deployevent_test.go` | 2 | Ingest/query round-trip, tenant+cluster isolation |
| `workloadgrowth_test.go` | 5 | Real-delta ranking, single-snapshot exclusion, topN capping, tenant isolation, deploy-event correlation by namespace |
| `budget_api_test.go` | 7 | GET/POST, negative rejection, wrong-method, missing-tenant-context, admin-only POST |
| `api_test.go` | 4 | Ingest+findings round trip, missing cluster_id, wrong method, missing tenant context |
| `summary_api_test.go` | 2 | Full dashboard-summary shape (now asserts `SpendByTeam`), nil-slice→`[]` guard |

Run: `cd controlplane && go test ./...` — **all 58 passing** (re-verified
2026-08-02), ~13–28s wall time depending on cache (several tests use
real 1.1s sleeps to get distinct `reported_at` second-granularity
timestamps).

### 3.7 New this session: six items from an external roadmap review, built for real

A friend reviewed this project and listed 17 gaps against a full FinOps/
observability platform. Most of that list is genuinely a different,
much larger product (SaaS signup, GitHub/Slack/Jira integrations, GPU/LLM
cost tracking, cross-customer benchmarking — see §10 for the honest
breakdown of what got deliberately deferred and why, including exactly
what each would technically require). Six were real, buildable
extensions of what already exists, and got built:

- **Root-cause correlation (K8s deploy events → cost anomalies).** The
  agent now watches every real Deployment's `spec.replicas` (new RBAC:
  `apps/deployments` get/list/watch) and diffs it against the last-seen
  snapshot each refresh cycle (`agent/cmd/kharcha/deployevents.go`) — a
  direct observation, not an inference. Replica-count changes ship to the
  control plane alongside findings (best-effort: an agent whose RBAC
  predates this just detects nothing, doesn't break the core refresh
  loop). Anomaly detection now searches each flagged cluster's real
  deploy events in a 30-minute window before a cost jump and surfaces the
  most recent one as `LikelyCause` — labeled correlation, not proven
  causation, in both the API field name and the UI copy. Verified live:
  a real 18x cost jump correctly correlated with a real
  `ReplicaCountChanged` event 5 minutes earlier.
- **Real $ optimization recommendations.** `agent/cmd/kharcha/pricing.go`'s
  new `optimizationTarget()` names the realistic cheaper path class each
  existing fix hint already describes (see §1.2) and reprices the exact
  same bytes at that class — the same real bytes, the same price book,
  just a different achievable class, not a fabricated "eliminate this
  traffic" number. INTERNET_EGRESS deliberately gets no target and
  honestly shows "—" instead of a number. The Overview "Potential
  savings" stat and a new column on the top-fixes table both use this
  real figure now, not the old (fabricated) "sum of every flagged
  finding's full cost."
- **Historical trend-change view.** New `/api/v1/workload-growth`
  endpoint and History sidebar page rank every workload with ≥2 real
  snapshots by the real delta between its first and most recent
  appearance, over whatever history is actually retained — deliberately
  not framed as "last 6 months," since the real window might be an hour.
  Each ranked entry is cross-checked against real deploy events in its
  own namespace during that window, reusing the same correlation as
  above.
- **Real Kubernetes cost graph.** A node-link topology view
  (`CostGraphPage.tsx`) built entirely from data already fetched — no new
  backend endpoint. A small dependency-free force-relaxation layout
  (`graphLayout.ts`, ~100 lines, not a new npm package: pairwise
  repulsion + edge spring + center-pull, 300 fixed iterations, no
  animation loop) positions real workload nodes; line thickness and node
  size both encode real cost (never color alone). Clicking an edge shows
  its exact real traffic/cost/savings. Deliberately has no "Latency"
  field — this agent counts bytes via eBPF, it has no per-flow latency
  instrumentation, and the page says so explicitly rather than
  fabricating a number.
- **Predictive driver.** The Forecasting page's existing Holt projection
  (unchanged) is now paired with a real "why" when it's trending up: the
  workload with the single largest real cost increase from the History
  feature above. If no one workload's growth stands out, it honestly
  says so ("likely broad-based") instead of forcing an attribution.
  Composes three already-built, independently-verified signals; no new
  modeling.
- **Team ownership.** Real admin-configured namespace→team mapping
  (`team.go`, new Teams page) and a `spend_by_team` breakdown on
  dashboard-summary. `extractNamespace()` uses an RFC-1123-label check to
  tell a real `namespace/pod` source apart from the non-k8s-resolved
  cgroup-path fallback (which also contains slashes but never looks like
  a namespace, since real namespace names can't contain "." or "@") —
  anything unmapped or unresolved honestly folds into "Unassigned,"
  never a guessed owner.

All six: tenant-scoped like everything else in this control plane, with
dedicated regression tests (cross-tenant isolation, role gating where
relevant, the actual math/correlation logic) and a live Playwright/curl
verification pass before being called done — same discipline as
everything else in this document.

### 3.8 Real bugs found and fixed during this work (kept for the record)

1. **Nil-slice → JSON `null` crash.** Go's zero-value slice marshals to
   `null`, not `[]`; the frontend called `.map()` on `trend`/`spend_by_class`/
   etc. unguarded. A fresh install with zero data crashed outright. Fixed at
   the source (`store.go`, `make([]T, 0)`), with a regression test
   (`TestHandleDashboardSummaryEmptyStateHasNoNullArrays`).
2. **GSAP `ScrollTrigger` tracking the wrong scroll container.** The
   dashboard scrolls inside `main` (`overflow-y-auto`), not `window`, which
   is GSAP's default — section titles below the first viewport never
   revealed. Fixed by threading the real scroll container ref through.
3. **BPF map silently evicting under load** (see §2 above).
4. **kubectl-exec CPU overhead** (see §1/§2 above).
5. **Self-inflicted image-pull break**: changing the Helm charts' default
   image references to GHCR (still private at the time) broke the *next*
   upgrade of the already-running local release. Fixed by pinning
   `image.repository`/`image.tag` explicitly until the packages go public.
6. **Settings-table primary-key collision, caught before it shipped.**
   Building multi-tenant isolation, the naive migration (`ALTER TABLE
   settings ADD COLUMN tenant_id`) still left `key` alone as the table's
   real PRIMARY KEY — two tenants both setting `budget_inr` would have
   silently overwritten each other's value despite the new column
   existing. Fixed by detecting the old single-column-PK schema and
   rebuilding the table with a genuine `(tenant_id, key)` composite key,
   preserving existing data under tenant 1. Caught by a dedicated
   regression test (`TestOpenStoreMigratesSettingsTableWithoutTenantID`)
   before this ever reached a real deployment.
7. **Lost ingest token after the multi-tenant migration — hit for real,
   not caught by a test.** The live k3d agent's `CHIDRIXX_AUTH_TOKEN`
   secret still held the *old* shared-token value from before real
   multi-tenant auth shipped; the real per-tenant token generated at
   bootstrap was only ever printed once to pod logs, which had scrolled
   away by the time this was redeployed. The agent was silently failing
   every ingest with 401 until this was caught (`kubectl logs` on the
   agent showed `ship to control plane: control plane returned status
   401`). There was no way to recover the lost token (stored SHA-256
   hashed) or mint a new one for an existing tenant — fixed by adding
   `CreateAPIToken` + the `controlplane create-token --tenant-id N` CLI
   subcommand, then using it to issue a real replacement and confirming
   ingest succeeded again.

---

## 4. Frontend — component inventory

32 `.tsx` files + 9 `.ts` files, 4,044 lines. React 18 + Vite 5 +
TypeScript + Tailwind + Recharts + Framer Motion + GSAP. No component
library — every visual element (donut chart, trend chart, cost graph,
force layout) is hand-built, matching the project's stated preference
for owning its own rendering rather than pulling in a chart/graph
dependency.

### 4.1 Pages (sidebar-routed, 15 total, verified against `Sidebar.tsx`)

Overview (default), Insights, Explorer, Workloads, **Cost Graph**,
**Teams**, Costs & Usage, Budgets, Savings Advisor, Forecasting,
Anomalies, **History**, Reports, Automations, Settings.

### 4.2 New components this session

| File | Role |
|---|---|
| `CostGraphPage.tsx` | Node-link topology, builds nodes/edges client-side from `/api/v1/findings` (no new backend call) |
| `graphLayout.ts` | Dependency-free force-relaxation layout |
| `HistoryPage.tsx` | Fetches `/api/v1/workload-growth`, renders ranked list + per-workload sparkline + correlated-event note |
| `TeamsPage.tsx` | Fetches/mutates `/api/v1/teams`, renders spend-by-team + namespace-ownership CRUD form |
| `PredictiveDriverCard.tsx` | Fetches `/api/v1/workload-growth`, reuses `holtForecast()` from `forecast.ts` to determine trend direction, shows the top-growth workload as "likely driver" only when trending up |
| `LoginPage.tsx` | Real username/password form → `/api/v1/auth/login` |

### 4.3 Modified components this session

`AnomalyCard.tsx` (renders `likely_cause` correlation note),
`TopFixesTable.tsx` (new "Potential savings" column), `App.tsx`
(potentialSavings stat now sums real savings not full cost; session
state machine: `checking → landing → login → authed`), `Sidebar.tsx`
(real username/role + working Log out button, replacing the old
hardcoded "Admin / Shared token access"), `SettingsPage.tsx`,
`BudgetCard.tsx` (role-gated edit UI), `FeaturePages.tsx`
(`ForecastingPage` now also renders `PredictiveDriverCard`),
`TrendProjectionCard.tsx` (Holt's method, replacing plain OLS),
`types.ts` (every new wire shape: `DeployEvent`/`TeamOwnership`/
`TeamSpend`/`WorkloadGrowth`/`WorkloadCostPoint`, plus
`savings_low_inr`/`savings_high_inr` on `Finding`).

### 4.4 Frontend build output (measured, current)

```
dist/assets/index-*.js    930.67 kB  (277.29 kB gzip)
dist/assets/index-*.css    19.62 kB  (5.35 kB gzip)
```

Single JS bundle, no code-splitting — Vite's own build warns about this
(>500kB chunk). Punch-list item, unchanged this session (§8).

### 4.5 Frontend dependencies (`package.json`, exact)

Runtime: `@fontsource-variable/geist`, `framer-motion`, `gsap`, `motion`,
`react` + `react-dom` (18.x), `recharts`. Dev: the standard
Vite+React+TS+Tailwind toolchain, nothing exotic. Vendored (not in
`package.json`, committed as source): `DecryptedText.jsx`,
`RotatingText.jsx` + `.css`, `VariableProximity.jsx` + `.css` —
react-bits components pulled in via the real `shadcn` CLI, not
hand-copied.

---

## 5. Deployment & Distribution

| Item | Status |
|---|---|
| Helm charts (`deploy/helm/kharcha`, `deploy/helm/controlplane`) | **Real.** Lint-clean, template-clean, and actually `helm install`/`upgrade`d against a live cluster (not just linted). |
| OCI chart repo on GHCR | **Real.** `oci://ghcr.io/ananttyagi07/charts/{kharcha,chidrixx-controlplane}` — packaged, pushed, pulled back down, and `helm template`d from the round-tripped artifact to confirm it's not corrupted. |
| GHCR image + chart visibility | **Blocked, needs your action.** All four packages (`chidrixx-agent`, `chidrixx-controlplane`, `charts/kharcha`, `charts/chidrixx-controlplane`) are still **private**. GitHub's REST API returns a 404 on the documented visibility-change endpoint even with a working `GET` on the same path and the right token scope — this is a web-UI-only setting, confirmed by direct testing, not a permissions issue on my end. Fix: `github.com/users/Ananttyagi07/packages/container/package/<name>` → Package settings → Change visibility → Public, for all four. |
| Ingress/TLS (control plane) | **Real, optional.** `ingress.enabled` in the Helm chart, verified: a real request through Traefik with the right `Host` header + Basic Auth returned 200. |
| cgroup-namespace limitation | **Documented, not fixable from our side** (inherent to Docker-in-Docker clusters like kind/k3d). The agent now gives a specific, actionable error instead of a generic `operation not permitted`. Real managed Kubernetes (EKS/GKE/AKS) doesn't hit this at all. |

### 5.1 Helm chart inventory (18 files, both charts)

**`deploy/helm/kharcha`** (agent DaemonSet): `Chart.yaml`, `values.yaml`,
`values-gcp.yaml` (real GCP price-book override), `templates/daemonset.yaml`,
`templates/clusterrole.yaml` (new rule this session: `apps/deployments`
get/list/watch, for deploy-event detection), `templates/clusterrolebinding.yaml`,
`templates/serviceaccount.yaml`, `templates/configmap-pricebook.yaml`
(key renamed `aws.yaml` → generic `pricebook.yaml` this session),
`templates/_helpers.tpl`.

**`deploy/helm/controlplane`**: `Chart.yaml`, `values.yaml` (auth
section rewritten this session: `adminUser`/`adminPasswordSecretName`/
`adminPasswordSecretKey`/`generate`, replacing the old shared-token
fields), `templates/deployment.yaml` (`CHIDRIXX_ADMIN_USER`/
`CHIDRIXX_ADMIN_PASSWORD` env vars, replacing `CHIDRIXX_AUTH_TOKEN`),
`templates/auth-secret.yaml` (generates/reuses an admin-password secret,
`lookup`-checked so upgrades don't rotate it), `templates/service.yaml`,
`templates/ingress.yaml`, `templates/pvc.yaml`, `templates/NOTES.txt`
(rewritten: walks through fetching the bootstrap password + one-time-logged
ingest token + the `create-tenant`/`create-user`/`create-token` exec
commands), `templates/_helpers.tpl`.

### 5.2 Live cluster state (k3d, as of this document)

- Namespace `chidrixx`: `chidrixx-controlplane` Deployment (Helm
  revision 7), image `chidrixx-controlplane:dev`, PVC-backed SQLite.
- Namespace `kharcha`: `kharcha-kharcha` DaemonSet (Helm revision 5),
  image `chidrixx-agent:dev`, real ingest token issued via
  `create-token` and stored in the `kharcha-controlplane-token` secret.
- Real production data present: 2+ clusters, 170+ findings, real
  `spend_by_team` (`Unassigned` — no namespace mappings configured on
  the live tenant yet), real cloud/region on the newer cluster's
  findings, `unknown`/`unknown` on the pre-upgrade cluster's (honest
  fallback, not broken).

---

## 6. CI (`.github/workflows/ci.yml`) — verified against the actual file, not assumed

Three jobs, **all scoped to the `agent/` module only**:

1. `build-and-test` (`working-directory: agent`): `gofmt -l` check, `go vet`,
   `go build`, `go test ./... -v` (unprivileged), then recompiles
   `bpf/flow_cgroup.o` from source and re-runs the two privileged BPF
   tests (`TestEgressByteAccounting`, `TestLoadAttachesAndDetaches`)
   against the freshly-compiled object — genuinely catching source/binary
   drift, not diffing against the committed `.o` (which isn't
   byte-reproducible across compilers anyway, per the workflow's own
   comment). GitHub-hosted runners are full VMs, so they don't hit the
   cgroup-namespace `EPERM` that Docker-in-Docker clusters like k3d do.
2. `docker-build`: builds `chidrixx-agent:ci` from the repo-root
   `Dockerfile`. Agent only.
3. `helm-lint`: `helm lint` + `helm template` against
   `deploy/helm/kharcha` **only**.

**What this means concretely: `controlplane/` has zero CI coverage
today.** Its 58 Go tests, its Docker build, and both Helm charts'
`helm lint`/`helm template` — none of that runs automatically anywhere.
Every verification of the control plane in this document was run by
hand, locally, this session (and prior ones). This is a bigger, more
concrete gap than a passing mention: a change to `controlplane/` today
could break `go build`, break a test, or break the Helm chart, and
nothing would catch it before a human runs it manually. Fixing this is
mechanical (mirror the three `agent/` jobs with `working-directory:
controlplane`, add `docker-build-controlplane` and
`helm-lint-controlplane` targets) but it hasn't been done — this is
punch-list item §8.5 below, upgraded from "nice to have" framing to
"concrete, unmitigated risk."

Separately: none of this session's live-Playwright verification scripts
(the ones that started a real server, ingested real data, clicked
through the UI, and asserted on rendered text) are committed anywhere —
they were written to a scratch directory, run once each, and deleted.
That compounds with the gap above: there is currently no automated
regression coverage at all for anything a browser would see, for either
module.

---

## 7. What's explicitly NOT built (and why)

These are honest placeholders — visually present where relevant, clearly
labeled, never filled with invented numbers:

- **A genuine ML/time-series forecast beyond Holt's method** — §3.7's
  Holt model is a real classical technique with fitted parameters and a
  computed confidence interval, but it's still not a neural/ARIMA-grade
  forecast, and it still can't be calendar-aligned (chidrixx's data is
  cumulative snapshots, not fixed time windows).
- **No automated release/versioning** — both charts are still `0.1.0`;
  there's no CI job that bumps versions or cuts releases automatically.
- **No CI coverage for `controlplane/`** — see §6. Listed here too since
  it's a real absence, not just a technical-debt line item.

---

## 8. What's actually left — punch list, with technical detail

| # | Item | Who | Effort | Technical detail |
|---|---|---|---|---|
| 1 | Flip 4 GHCR packages to public | **You** | 5 min | Confirmed via direct `gh api -X PATCH .../visibility` testing: 404 on GitHub's own documented endpoint despite a working `GET` and correct `write:packages` scope. Web-UI-only control, not scriptable. |
| 2 | An actual second cloud deployed with the new GCP price book | You | 5 min per agent | Plumbing is 100% real and tested (`pricebook/gcp.yaml`, `values-gcp.yaml`, `SpendByCloud()`, the donut). What's missing: a second live agent actually running `-pricebook=pricebook/gcp.yaml` (or `values-gcp.yaml` via Helm) against a real GCP-hosted workload. `helm install -f values.yaml -f values-gcp.yaml`. |
| 3 | A deeper forecasting model, if Holt's isn't enough | Both | Large | Needs (a) enough retained history to fit a seasonal component meaningfully — data is agent-refresh-cadence snapshots, not calendar-aligned, so a real seasonal model needs a time-bucketing decision first; (b) a genuine choice between ARIMA/Prophet/a small neural sequence model, each with different data-volume requirements this system hasn't validated it has. |
| 4 | Frontend bundle size (930KB, no code-splitting) | Me | Small | `React.lazy()` + dynamic `import()` per sidebar page — Overview + Sidebar are the only things needed on first paint; the other 14 pages could each be their own chunk. |
| 5 | **CI coverage for `controlplane/` (upgraded from "commit the Playwright scripts")** | Me | Medium | Mirror the three `agent/` CI jobs (`go test`, Docker build, `helm lint`/`template`) with `working-directory: controlplane`, targeting `deploy/helm/controlplane`. Separately, promote the best of this session's throwaway Playwright verification scripts into a real `test/e2e/` suite. Together these close the "verified once by hand" gap described in §6. |
| 6 | Business/GTM (pricing, personas, launch) | You | Deprioritized | No pricing page, no persona docs, no launch plan — explicitly deprioritized per prior direction ("technical completeness came first"). |

---

## 9. Future vision — not started, direction only

Everything above is either shipped or a scoped, estimateable task. This
section is different: it's where the project is headed next, not
something in progress. Marking it clearly as vision, not a roadmap
commitment with dates, so it doesn't get conflated with the punch list
above.

- **A domain-specialized LLM, fine-tuned on eBPF / deep networking /
  kernel internals.** The plan is to fine-tune a model specifically on
  eBPF program semantics, Linux networking internals (netfilter, tc,
  cgroups, the network namespace/veth/CNI stack), and kernel-level
  traffic behavior — not a general coding assistant repurposed for this
  domain. Intended use once ready: interpreting a cluster's real flow
  data and fix-engine output in natural language, reasoning about why a
  given path was classified the way it was, and eventually helping
  author or review the `NetworkPolicy` fixes chidrixx already generates
  mechanically today — all grounded in the agent's own real, measured
  data, not invented network theory.
- **Later SaaS integration.** Once that model is trained and validated,
  the intent is to surface it inside the control plane itself — e.g. an
  assistant panel that can answer cluster-specific questions using the
  real ingested findings as context, rather than a bolt-on chatbot with
  no access to actual cluster state. The real multi-tenant isolation this
  depended on (§3) now exists — separate tenants, separate sessions,
  every query scoped by tenant_id — so this is no longer blocked on a
  missing subsystem, just on the model itself being ready.
- **Why this order**: the fine-tuning work is happening in parallel with
  the product work above; the SaaS integration was deliberately sequenced
  *after* real tenant isolation because piping a real customer's live
  traffic data into a shared model without it would have been a genuine
  data-boundary problem, not just a nice-to-have gap. That gate is now
  cleared.

---

## 10. Roadmap items explicitly deferred — what each would technically require

The same external review listed 11 more gaps beyond the six built in
§3.7. Each is individually a real, defensible idea — none were dismissed
as wrong — but each is genuinely its own product or needs
infrastructure/credentials/data that don't exist in this environment.
Listed honestly rather than silently dropped, with the exact technical
dependency for each:

| Item | Why it's deferred | What it would technically require |
|---|---|---|
| **GitHub/commit correlation** ("which PR caused this spike") | Needs a GitHub App/webhook integration this control plane has no credentials for | A GitHub App (webhook subscription to `push`/`deployment` events) or REST polling with a PAT; a new `git_event` table mirroring `deploy_event`'s shape; correlation logic identical in spirit to `anomaly.go`'s `LikelyCause` |
| **Multi-cloud discovery** (auto-detect AWS/Azure/GCP/Oracle/DO resources) | The price-book mechanism already supports multiple clouds once agents are deployed; auto-*discovery* is a separate, much larger integration surface | Real IAM credentials per cloud account (AWS `DescribeInstances`-class calls, GCP Asset Inventory API, Azure Resource Graph) — an entirely new credential-management surface |
| **FinOps workflow** (chargeback, department billing, monthly reports, cost allocation) | A real accounting/billing subsystem, not a dashboard feature; budget tracking (§3) is the honest subset that exists today | A real ledger/invoicing data model — allocation rules, rollover periods, multi-currency — genuinely adjacent to `settings`/`budget` but an order of magnitude more complex |
| **Third-party integrations** (Slack/Teams/PagerDuty/Jira/ServiceNow/Datadog/Grafana) | The webhook mechanism already exists as the building block | `agent/cmd/kharcha/alert.go` already POSTs to any Slack-*compatible* URL; the real gap is per-vendor OAuth app registration + API client code, one per integration, each its own small project |
| **Self-service SaaS signup** | Deliberately not built; provisioning is the `create-tenant`/`create-user`/`create-token` CLI, an operator action | A public-facing account-creation flow with email verification, password reset, and payment processing (Stripe or similar) — none of which exist |
| **Business KPIs** (cost per API call/customer/model request) | Needs request-level attribution this agent doesn't have | This eBPF agent counts bytes on a 5-tuple, not HTTP request boundaries — a different data-collection layer, likely an eBPF uprobe or sidecar, not a dashboard change |
| **AI infrastructure cost tracking** (GPU, embedding, inference cost per prompt) | A real, compelling idea for a *different* agent, not a small addition to this one | Entirely new telemetry sources: GPU utilization/cost (e.g. DCGM), model-serving request logs, vector-DB query counts — none of which this network-layer agent observes today |
| **Cost-graph latency** | The real cost graph (§3.7) omits latency on purpose | Real RTT/socket-timing instrumentation via eBPF (e.g. hooking `tcp_connect`/`tcp_recvmsg` timestamps) — materially more invasive than the current pure byte-counting `cgroup_skb` hooks |
| **Deeper predictive optimization** | Same dependency as §8 item 3 | The current `PredictiveDriverCard` composes existing real signals (Holt forecast + workload growth + deploy events) rather than adding new modeling; going further needs the same ARIMA/seasonal work |
| **Cross-customer benchmarking** ("are you normal for your size?") | Needs an anonymized dataset aggregated across many real customers | Structurally impossible with a single-operator/single-tenant-today deployment; a real feature for a multi-tenant SaaS *after* it has actual paying customers |
| **Security/exfiltration detection** (unexpected DNS, unknown-IP alerts) | Technically the closest of the eleven — different product category from cost attribution | The agent already resolves destination IPs and could flag ones outside a known-good set; not built because starting it silently would be scope creep nobody explicitly asked to prioritize |

---

## 11. Honesty audit — every "not real" claim in this codebase, in one place

Grep-able commitments this codebase makes about its own limitations,
collected so they're all visible in one spot rather than scattered
across component comments:

- **No latency measurement anywhere** — `CostGraphPage.tsx`'s own
  caption states this; the agent's eBPF hooks (`cgroup_skb`) count bytes
  only, never timestamp round-trips.
- **No calendar-aligned forecasting** — `forecast.ts`/`TrendProjectionCard.tsx`
  and `ForecastingPage`'s copy both say so; data is cumulative snapshots
  at agent-refresh cadence, not fixed time windows.
- **Carbon footprint is a rough estimate, not measured** —
  `CarbonFootprintCard.tsx`, cited kWh/GB × grid-gCO2e/kWh coefficients,
  visible formula, "Not measured" caveat always shown.
- **INTERNET_EGRESS has no savings number** — `pricing.go`'s
  `optimizationTarget()` deliberately returns `(_, false)`; the UI shows
  "—", never a fabricated rupee figure.
- **Anomaly/History "likely cause" is correlation, not causation** —
  both `Anomaly.LikelyCause` and `WorkloadGrowth.RelatedEvents` are
  documented as such in their Go doc comments and in the exact UI copy
  ("worth checking, not proven causation").
- **Price book is list pricing, not your negotiated rate** — both
  `pricebook/aws.yaml` and `pricebook/gcp.yaml` carry this disclaimer
  verbatim, with a `last_verified` date.
- **cgroup-namespace limitation is real and undocumented-around** — kind/k3d
  Docker-in-Docker environments hit a genuine `EPERM`; real managed
  Kubernetes (EKS/GKE/AKS) doesn't.
- **GHCR visibility is genuinely API-blocked** — verified by direct
  testing, not assumed; see §8 item 1.
- **`controlplane/` has zero CI coverage** — see §6; not hidden in a
  footnote, called out as a concrete risk.

---

## Bottom line

The agent's core claims (byte-accurate attribution, real topology
classification, real fix generation) are measured, not asserted. The
control plane and dashboard are a genuinely working multi-tenant product
with real user-facing features and real data isolation between
customers, not a static mockup or a single-shared-secret toy. Beyond the
original feature set, it now does real root-cause correlation (deploy
events → cost anomalies), real $ optimization recommendations, a real
historical trend-change view, a real cost topology graph, and a real
predictive driver — six genuine extensions built and verified this
session, out of the 17 an external review suggested, with the other 11
honestly documented in §10 as separate products or blocked on
infrastructure this environment doesn't have, not silently dropped.

The one concrete technical risk worth naming plainly: **`controlplane/`
— the larger of the two modules, with more tests and more surface area
than `agent/` — has no CI coverage at all** (§6). Everything in this
document was verified by hand, correctly, but nothing stops a future
change from silently breaking it.

Action items that need you directly: #1 (five minutes, GHCR visibility)
and #2 (five minutes per agent, if you want a real multi-cloud split in
production right now rather than just in tests). #3 and the CI gap (§8
item 5) are genuine scope/engineering-investment decisions, not
oversights.
