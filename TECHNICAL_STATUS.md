# chidrixx — Technical Status (Deep Reference)

_Last updated: 2026-08-01_

This is the sibling document to `PROJECT_STATUS.md`. That file is the
narrative "what's real, what's left, what's deferred" read. This one is
the exhaustive engineering reference underneath it: every file, every
function, every DB column, every API route, every test, every dependency
— so nothing about the current state has to be taken on faith or
reconstructed from memory. Where `PROJECT_STATUS.md` says "real and
verified," this document says exactly which file, which function, which
test, and which command proved it.

Every number in this document was measured against the actual repository
at the commit this file was written against (`git log -1`: `6c518e5`
"docs: update PROJECT_STATUS.md for the 6 new roadmap-review features"),
not recalled.

---

## 0. Repository shape

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
└── .github/workflows/   ci.yml — the only CI workflow
```

Total: **~12,200 lines** of hand-written Go + TypeScript across both
modules and the frontend (not counting vendored react-bits `.jsx`/`.css`
files, which are third-party).

---

## 1. Agent module (`chidrixx`) — file-by-file

| File | Lines¹ | What it does |
|---|---|---|
| `main.go` | — | CLI entrypoint. 15 flags (see §1.1). Wires BPF loader → aggregator → shipper → alerter → metrics server → two ticker loops (15s scrape/ship, `k8sRefreshInterval`-default-10s metadata refresh). |
| `loader.go` | — | Owns BPF program lifecycle: load `flow_cgroup.o`, attach to cgroup, detach on shutdown. Recognizes the specific cgroup-namespace `EPERM` and wraps it with an actionable message. |
| `classify.go` | — | `Classify(ClassifyInput) (PathClass, confidence)`. 8 `PathClass` values (§1.2), each with high/med/low confidence depending on how much topology data was resolvable. |
| `pricing.go` | — | `PriceBook` struct + YAML loader. `CostINR(class, confidence, bytes) (low, high float64)` — confidence widens the band (15%/35%/60%). New this session: `optimizationTarget(class) (PathClass, bool)` — the re-pricing map for real savings estimates (§1.3). |
| `report.go` | — | `Aggregate` — the cumulative-since-start in-memory store of `Finding`s, keyed by (source, destination). `Add()` is the hot path: resolves source/dest metadata, classifies, prices, accumulates bytes/cost/**savings** (new). `Finding` struct has 24 fields including the new `SavingsLowINR`/`SavingsHighINR`. |
| `fixengine.go` | — | `generateFixManifest(class, namespace, destIP) string` — real `NetworkPolicy` YAML for INTERNET_EGRESS/NAT_EGRESS/CROSS_REGION only (CROSS_AZ/MANAGED_SERVICE stay a text hint — no pod-label data to build a real selector). |
| `kubernetes.go` | — | `KubernetesResolver` — talks to the K8s API (in-cluster token or `kubectl` fallback) for pods/services/endpointslices/nodes. New this session: `lastReplicas map[string]int32` + `pendingEvents []DeployEvent` fields feeding deploy-event detection (see `deployevents.go`). |
| `deployevents.go` **(new)** | 96 | `diffReplicas()` — pure function, no I/O — diffs a `kubeDeploymentList` snapshot against the last-seen replica-count map, returns `[]DeployEvent`. `refreshDeployEvents()` does the actual `GET /apis/apps/v1/deployments` fetch + calls the pure diff. `DrainDeployEvents()` — thread-safe buffer drain, called from the ship loop (different goroutine/ticker than the refresh loop that fills it). |
| `shipper.go` | — | `Shipper.Ship(ctx, findings, events)` — POSTs `shipperRequest{ClusterID, Findings, Events}` as JSON, Basic Auth (`agent`/token). Wire struct `shipperFinding` now has `savings_low_inr`/`savings_high_inr` fields. |
| `alert.go` | — | Threshold + growth-ratio alerting → Slack-compatible webhook. |
| `workload.go` | — | `WorkloadIdentity` — wraps a cgroup ID + optional resolved `KubeWorkload`; `DisplayName()` → `"namespace/pod"` or raw cgroup path. |
| `html.go` | — | Static HTML report renderer (self-contained, no JS framework). |
| `metrics.go` | — | 4 Prometheus metrics (§1.4), served via `net/http` on `-metrics-addr`. |
| `loadgen/main.go` | — | Standalone load-generator binary for the 10k-flow test harness; not part of the shipped agent. |

¹ Line counts only given for files added/materially changed this
session; the rest are unchanged from prior sessions and already
documented in `PROJECT_STATUS.md` §1–2.

### 1.1 Agent CLI flags (`main.go`, all 15)

`-bpf-object`, `-cgroup-path`, `-pricebook` (default `pricebook/aws.yaml`),
`-managed-cidrs`, `-node-has-public-ip`, `-node-name`, `-html-out`,
`-metrics-addr`, `-alert-webhook`, `-alert-threshold-inr`,
`-alert-growth-ratio`, `-controlplane-url`, `-cluster-id`,
`-k8s-refresh-interval` (default 10s). `CHIDRIXX_AUTH_TOKEN` is read from
the environment, not a flag (so it never shows up in `ps`/container
specs).

### 1.2 PathClass enumeration (`classify.go`)

`SAME_NODE`, `SAME_AZ`, `CROSS_AZ`, `CROSS_REGION`, `MANAGED_SERVICE`,
`PRIVATE_OFFCLUSTER`, `NAT_EGRESS`, `INTERNET_EGRESS` — 8 values, fixed,
never dynamically extended.

### 1.3 optimizationTarget map (`pricing.go`, new this session)

| From class | Target class | Real-world action the fix hint describes |
|---|---|---|
| `CROSS_AZ` | `SAME_AZ` | co-locate the two workloads in one zone |
| `CROSS_REGION` | `CROSS_AZ` | keep traffic inside one region |
| `NAT_EGRESS` | `PRIVATE_OFFCLUSTER` | add a VPC/private (Gateway) endpoint |
| `MANAGED_SERVICE` | `SAME_AZ` | pin client + managed endpoint to one zone |
| `INTERNET_EGRESS` | *(none)* | real fix is usage reduction (cache/compress), not a cheaper class — deliberately no savings number |
| `SAME_NODE` / `SAME_AZ` / `PRIVATE_OFFCLUSTER` | *(none)* | already the cheapest tier, nothing to optimize toward |

### 1.4 Prometheus metrics (`metrics.go`)

`kharcha_flow_bytes_total` (CounterVec), `kharcha_cost_inr` (GaugeVec),
`kharcha_map_entries` (Gauge), `kharcha_scrape_lag_seconds` (Gauge).

### 1.5 Agent dependencies (`go.mod`, exact versions)

Direct: `github.com/cilium/ebpf v0.22.0`,
`github.com/prometheus/client_golang v1.24.1`, `gopkg.in/yaml.v3 v3.0.1`.
5 indirect deps, all transitive requirements of the above (perks,
xxhash, goautoneg, client_model, common, procfs, x/sys, protobuf) — no
surprise dependencies, no dependency added without a direct reason.

### 1.6 Agent test inventory — 26 test functions across 12 files

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

Run: `cd agent && go test ./...` — currently **all passing**,
~3s wall time (excludes privileged BPF tests which need CAP_BPF, those
run in CI on GitHub-hosted VMs).

---

## 2. Control plane module (`chidrixx-controlplane`) — file-by-file

| File | What it does |
|---|---|
| `main.go` | Entrypoint + 3 CLI subcommands (`create-tenant`, `create-user`, `create-token` — all new/extended this session) + `bootstrapDefaultTenant()` (idempotent, only fires when `TenantCount()==0`) + the 9-route mux (§2.1). |
| `store.go` | `Store` wraps one `*sql.DB` (SQLite, `SetMaxOpenConns(1)` — single-writer by design). 8 tables (§2.2). Every query method takes `tenantID int64` as its first real parameter. |
| `model.go` | Wire structs: `Finding` (14 fields incl. new `SavingsLowINR`/`SavingsHighINR`), `DeployEvent` (new), `IngestRequest` (now carries `Events []DeployEvent`). |
| `api.go` | `handleIngest` (now also calls `store.IngestDeployEvents` best-effort), `handleFindingsAPI`. |
| `summary_api.go` | `handleDashboardSummary` — the one big aggregate call the SPA's first paint needs. Response now includes `SpendByTeam` (new). |
| `anomaly.go` | `detectAnomalies()` — 2x-growth-ratio detection between a cluster's last two snapshots. Now also calls `RecentDeployEvents()` in a 30-minute lookback and attaches `LikelyCause *DeployEvent`. |
| `budget_api.go` | `handleBudget` — GET/POST, POST gated by `requireAdmin`. |
| `auth.go` | Middleware: `requireAPIToken` (agent ingest, Basic Auth password = per-tenant token), `requireSession` (browser, cookie), `requireAdmin` (role gate). |
| `auth_api.go` | `handleLogin`/`handleLogout`/`handleMe` — real bcrypt-verified sessions, `chidrixx_session` cookie, 24h TTL. |
| `tenant.go` | `Tenant`/`User`/`Session` structs, `CreateTenant` (atomic: tenant + admin + token in one tx), `CreateUser`, **new**: `CreateAPIToken` (token rotation for an existing tenant), `AuthenticateUser` (bcrypt, dummy-hash timing guard for unknown usernames), `AuthenticateAPIToken` (SHA-256 lookup), `CreateSession`/`GetSession`/`DeleteSession`. |
| `team.go` **(new)** | `TeamOwnership` CRUD (`SetTeamOwnership`/`ListTeamOwnership`/`DeleteTeamOwnership`), `extractNamespace()` (RFC-1123-label regex, tells a real `namespace/pod` apart from a cgroup-path fallback), `computeSpendByTeam()` (pure grouping function). |
| `team_api.go` **(new)** | `handleTeams` — GET (any role) / POST+DELETE (admin only via `teamsRoute`). |
| `deployevent.go` **(new)** | `IngestDeployEvents()`, `RecentDeployEvents(tenantID, clusterID, since, until)`. |
| `workloadgrowth.go` **(new)** | `WorkloadCostGrowth(tenantID, topN)` — ranks workloads by real first→last-snapshot delta across full retained history; correlates each with `RecentDeployEvents` in its own namespace over its own trend window. |
| `workloadgrowth_api.go` **(new)** | `handleWorkloadGrowth` — its own route, not folded into dashboard-summary (scans full history, not just latest-per-cluster). |
| `webassets.go` | `go:embed`'s `web/dist`, serves the SPA + static assets, no auth (no secrets baked into the bundle). |

### 2.1 API routes (9 real endpoints + the static SPA shell route, exact)

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

### 2.2 SQLite schema — 8 tables, exact DDL shape

| Table | Key columns | Notes |
|---|---|---|
| `flow_aggregate` | `id` PK, `tenant_id`, `cluster_id`, `reported_at`, `src_workload`, `dst_workload_or_endpoint`, `path_class`, `confidence`, `bytes_tx`, `bytes_rx`, `cost_low_inr`, `cost_high_inr`, `fix_hint`, `fix_manifest`, `cloud`, `region`, **`savings_low_inr`, `savings_high_inr`** (new) | Index on `(cluster_id, reported_at)`. Never pruned — every snapshot ever ingested is retained. |
| `settings` | `(tenant_id, key)` composite PK, `value` | Rebuilt mid-session from a single-column `key` PK — see bug #6 in `PROJECT_STATUS.md`. |
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

### 2.3 Control plane dependencies (`go.mod`, exact versions)

Direct: `golang.org/x/crypto v0.54.0` (bcrypt), `modernc.org/sqlite
v1.54.0` (pure-Go, no cgo). 8 indirect deps, all `modernc.org/sqlite`'s
own transitive requirements (libc, mathutil, memory, bigfft, etc.) plus
`google/uuid` and `go-isatty`.

### 2.4 Control plane test inventory — 58 test functions across 10 files

| File | Approx. tests | Covers |
|---|---|---|
| `store_test.go` | 20 | Ingest/query round-trips, the fix-manifest/savings round-trips, old-schema migration (both `flow_aggregate` and `settings`), **`TestTenantIsolationAcrossEveryReadPath`** (the single most important test in the repo — two tenants, deliberately reused cluster ID and settings key, checked against every read path) |
| `auth_test.go` | 10 | API-token and session middleware, cross-tenant token isolation, admin-role gating, **new**: `TestCreateAPITokenMintsARealWorkingSecondToken` |
| `anomaly_test.go` | 5 | Growth-ratio detection, single-snapshot/zero-baseline skips, **new**: deploy-event correlation inside/outside the lookback window |
| `team_test.go` + `team_api_test.go` **(new)** | 12 | Namespace-extraction regex (real vs. cgroup-path), spend grouping + fallback, store CRUD + tenant isolation, API handler + role gating |
| `deployevent_test.go` **(new)** | 2 | Ingest/query round-trip, tenant+cluster isolation |
| `workloadgrowth_test.go` **(new)** | 5 | Real-delta ranking, single-snapshot exclusion, topN capping, tenant isolation, deploy-event correlation by namespace |
| `budget_api_test.go` | 7 | GET/POST, negative rejection, wrong-method, missing-tenant-context, admin-only POST |
| `api_test.go` | 4 | Ingest+findings round trip, missing cluster_id, wrong method, missing tenant context |
| `summary_api_test.go` | 2 | Full dashboard-summary shape (now asserts `SpendByTeam`), nil-slice→`[]` guard |

Run: `cd controlplane && go test ./...` — **all 58 passing**, ~13–28s
wall time depending on cache (several tests use real 1.1s sleeps to get
distinct `reported_at` second-granularity timestamps).

---

## 3. Frontend (`controlplane/web`) — component inventory

32 `.tsx` files + 9 `.ts` files, 4,044 lines. React 18 + Vite 5 +
TypeScript + Tailwind + Recharts + Framer Motion + GSAP. No component
library — every visual element (donut chart, trend chart, cost graph,
force layout) is hand-built, matching the project's stated preference
for owning its own rendering rather than pulling in a chart/graph
dependency.

### 3.1 Pages (sidebar-routed, 15 total)

Overview (default), Insights, Explorer, Workloads, **Cost Graph (new)**,
**Teams (new)**, Costs & Usage, Budgets, Savings Advisor, Forecasting,
Anomalies, **History (new)**, Reports, Automations, Settings.

### 3.2 New components this session

| File | Role |
|---|---|
| `CostGraphPage.tsx` | Node-link topology, builds nodes/edges client-side from `/api/v1/findings` (no new backend call) |
| `graphLayout.ts` | Dependency-free force-relaxation layout (~100 lines): pairwise repulsion + edge spring + center-pull, 300 fixed iterations, no animation loop |
| `HistoryPage.tsx` | Fetches `/api/v1/workload-growth`, renders ranked list + per-workload sparkline + correlated-event note |
| `TeamsPage.tsx` | Fetches/mutates `/api/v1/teams`, renders spend-by-team + namespace-ownership CRUD form |
| `PredictiveDriverCard.tsx` | Fetches `/api/v1/workload-growth`, reuses `holtForecast()` from `forecast.ts` to determine trend direction, shows the top-growth workload as "likely driver" only when trending up |
| `LoginPage.tsx` | Real username/password form → `/api/v1/auth/login` |

### 3.3 Modified components this session

`AnomalyCard.tsx` (renders `likely_cause` correlation note),
`TopFixesTable.tsx` (new "Potential savings" column), `App.tsx`
(potentialSavings stat now sums real savings not full cost; session
state machine: `checking → landing → login → authed`), `Sidebar.tsx`
(real username/role + working Log out button, replacing the old
hardcoded "Admin / Shared token access"), `SettingsPage.tsx`,
`BudgetCard.tsx` (role-gated edit UI), `FeaturePages.tsx`
(`ForecastingPage` now also renders `PredictiveDriverCard`),
`TrendProjectionCard.tsx` (Holt's method, replacing plain OLS —
prior session), `types.ts` (every new wire shape:
`DeployEvent`/`TeamOwnership`/`TeamSpend`/`WorkloadGrowth`/
`WorkloadCostPoint`, plus `savings_low_inr`/`savings_high_inr` on
`Finding`).

### 3.4 Frontend build output (measured, current)

```
dist/assets/index-*.js    930.67 kB  (277.29 kB gzip)
dist/assets/index-*.css    19.62 kB  (5.35 kB gzip)
```

Single JS bundle, no code-splitting — Vite's own build warns about this
(>500kB chunk). This is punch-list item in `PROJECT_STATUS.md` §6,
unchanged this session.

### 3.5 Frontend dependencies (`package.json`, exact)

Runtime: `@fontsource-variable/geist`, `framer-motion`, `gsap`, `motion`,
`react` + `react-dom` (18.x), `recharts`. Dev: the standard
Vite+React+TS+Tailwind toolchain, nothing exotic. Vendored (not in
`package.json`, committed as source): `DecryptedText.jsx`,
`RotatingText.jsx` + `.css`, `VariableProximity.jsx` + `.css` —
react-bits components pulled in via the real `shadcn` CLI, not
hand-copied.

---

## 4. Deployment — Helm chart inventory (18 files, both charts)

### 4.1 `deploy/helm/kharcha` (agent DaemonSet)

`Chart.yaml`, `values.yaml`, `values-gcp.yaml` (new — real GCP price-book
override), `templates/daemonset.yaml`, `templates/clusterrole.yaml`
(**new rule this session**: `apps/deployments` get/list/watch, for
deploy-event detection), `templates/clusterrolebinding.yaml`,
`templates/serviceaccount.yaml`, `templates/configmap-pricebook.yaml`
(key renamed `aws.yaml` → generic `pricebook.yaml` this session),
`templates/_helpers.tpl`.

### 4.2 `deploy/helm/controlplane`

`Chart.yaml`, `values.yaml` (auth section rewritten this session:
`adminUser`/`adminPasswordSecretName`/`adminPasswordSecretKey`/`generate`,
replacing the old shared-token fields), `templates/deployment.yaml`
(`CHIDRIXX_ADMIN_USER`/`CHIDRIXX_ADMIN_PASSWORD` env vars, replacing
`CHIDRIXX_AUTH_TOKEN`), `templates/auth-secret.yaml` (generates/reuses an
admin-password secret, `lookup`-checked so upgrades don't rotate it),
`templates/service.yaml`, `templates/ingress.yaml`, `templates/pvc.yaml`,
`templates/NOTES.txt` (rewritten: walks through fetching the bootstrap
password + one-time-logged ingest token + the `create-tenant`/
`create-user` exec commands), `templates/_helpers.tpl`.

### 4.3 Live cluster state (k3d, as of this document)

- Namespace `chidrixx`: `chidrixx-controlplane` Deployment, revision 7
  (Helm), image `chidrixx-controlplane:dev`, PVC-backed SQLite.
- Namespace `kharcha`: `kharcha-kharcha` DaemonSet, revision 5 (Helm),
  image `chidrixx-agent:dev`, real ingest token issued via
  `create-token` and stored in the `kharcha-controlplane-token` secret.
- Real production data present: 2 clusters, 170+ findings, real
  `spend_by_team` (`Unassigned` — no namespace mappings configured on
  the live tenant yet), real cloud/region on the newer cluster's
  findings, `unknown`/`unknown` on the pre-upgrade cluster's (honest
  fallback, not broken).

---

## 5. CI (`.github/workflows/ci.yml`) — verified against the actual file, not assumed

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
Every verification of the control plane in this document and in
`PROJECT_STATUS.md` was run by hand, locally, this session (and prior
ones). This is a bigger, more concrete gap than a passing mention: a
change to `controlplane/` today could break `go build`, break a test, or
break the Helm chart, and nothing would catch it before a human runs it
manually. Fixing this is mechanical (mirror the three `agent/` jobs with
`working-directory: controlplane`, add `docker-build-controlplane` and
`helm-lint-controlplane` targets) but it hasn't been done.

Separately: none of this session's live-Playwright verification scripts
(the ones that started a real server, ingested real data, clicked
through the UI, and asserted on rendered text) are committed anywhere —
they were written to a scratch directory, run once each, and deleted.
That's `PROJECT_STATUS.md` §6 item 5, and it compounds with the gap
above: there is currently no automated regression coverage at all for
anything a browser would see, for either module.

---

## 6. What's left — technical detail behind `PROJECT_STATUS.md`'s punch list

### 6.1 GHCR package visibility (5 min, you)
Confirmed via direct `gh api -X PATCH .../visibility` testing: 404 on
GitHub's own documented endpoint despite a working `GET` and correct
`write:packages` scope. This is a web-UI-only control on GitHub's side,
not something fixable from a script. 4 packages affected:
`chidrixx-agent`, `chidrixx-controlplane`, `charts/kharcha`,
`charts/chidrixx-controlplane`.

### 6.2 A second real cloud in production (5 min per agent, you)
The plumbing is 100% real and tested (`pricebook/gcp.yaml`,
`values-gcp.yaml`, `SpendByCloud()`, the "Spend by provider" donut) —
what's missing is simply a second live agent actually running
`-pricebook=pricebook/gcp.yaml` (or `values-gcp.yaml` via Helm) against
a real GCP-hosted (or GCP-priced) workload. Until then, production
correctly shows 100% AWS, which is accurate, not a bug.

### 6.3 A deeper forecasting model (large, both)
Current: Holt's linear (double exponential smoothing), `forecast.ts`,
parameters grid-searched over α,β ∈ {0.1..0.9} minimizing real in-sample
SSE, real 80% prediction interval from residual variance. What a
"deeper" model would need: (a) enough retained history to fit a
seasonal component meaningfully — the data is agent-refresh-cadence
snapshots, not calendar-aligned, so a real seasonal model needs a
decision on how to bucket time first; (b) a genuine choice between
ARIMA/Prophet/a small neural sequence model, each with different data
volume requirements this system hasn't validated it has.

### 6.4 Frontend code-splitting (small, technical debt)
930KB single bundle. `React.lazy()` + dynamic `import()` per sidebar
page would cut initial load meaningfully — Overview + Sidebar are the
only things needed on first paint; the other 14 pages could each be
their own chunk. Not done because it's pure infra work with no feature
value, correctly deprioritized versus this session's feature work.

### 6.5 Formalize the Playwright verification scripts (medium, technical debt)
Every feature in this session (and prior ones) was verified with a
throwaway Python Playwright script: start a real server, ingest real
data, click through the UI, assert on real rendered text, screenshot,
delete. That discipline is real and has been followed consistently —
but none of it is committed. Promoting the best of these into a real
`test/e2e/` suite that runs in CI would turn "verified once, by hand"
into "verified on every future change," which matters more now that
there are 15 pages and 9 API routes instead of the original handful.

### 6.6 Business/GTM (deprioritized, you)
No pricing page, no persona docs, no launch plan — explicitly
deprioritized per prior direction ("technical completeness came
first").

---

## 7. The 11 explicitly-deferred roadmap items — technical detail

(Narrative reasoning is in `PROJECT_STATUS.md` §8; this is what each
would actually require, technically, if picked up later.)

| Item | What it would technically require |
|---|---|
| GitHub/commit correlation | A GitHub App (webhook subscription to `push`/`deployment` events) or polling the REST API with a PAT; a new `git_event` table mirroring `deploy_event`'s shape; correlation logic identical in spirit to `anomaly.go`'s `LikelyCause`. |
| Multi-cloud *discovery* (auto-detect resources) | Real IAM credentials per cloud account (AWS `DescribeInstances`-class calls, GCP Asset Inventory API, Azure Resource Graph) — an entirely new credential-management surface, not a code change to the existing price-book mechanism. |
| FinOps chargeback/billing | A real ledger/invoicing data model — allocation rules, rollover periods, multi-currency — genuinely adjacent to `settings`/`budget` today but an order of magnitude more complex. |
| Slack/Jira/PagerDuty/ServiceNow/Datadog/Grafana | The webhook mechanism (`agent/cmd/kharcha/alert.go`) already POSTs to any Slack-*compatible* URL — the real gap is per-vendor OAuth app registration + API client code, one per integration, each its own small project. |
| Self-service SaaS signup | A public-facing account-creation flow with email verification, password reset, and payment processing (Stripe or similar) — none of which exist; today's `create-tenant`/`create-user` CLI is deliberately operator-only. |
| Business KPIs (cost per API call/customer/model) | Needs request-level instrumentation this eBPF agent doesn't have (it counts bytes on a 5-tuple, not HTTP request boundaries) — a different data-collection layer, likely an eBPF uprobe or sidecar, not a dashboard change. |
| AI infrastructure cost tracking (GPU/embedding/inference) | New telemetry sources entirely: GPU utilization/cost (e.g. `nvidia-smi`/DCGM), model-serving request logs, vector-DB query counts — none of which this network-layer agent observes today. |
| Cost-graph latency | Real RTT/socket-timing instrumentation via eBPF (e.g. hooking `tcp_connect`/`tcp_recvmsg` timestamps) — materially more invasive than the current pure byte-counting `cgroup_skb` hooks, a real scope expansion of the agent's kernel footprint. |
| Cross-customer benchmarking | An anonymized, aggregated dataset across many real tenants — structurally impossible with a single-operator/single-tenant-today deployment; would need real paying multi-tenant customers first. |
| Security/exfiltration detection (DNS/unknown-IP alerts) | Technically the closest of the eleven — the agent already resolves destination IPs and could flag ones outside a known-good set. Not built because it's a different product category (security monitoring, not cost attribution) and starting it silently would be scope creep nobody explicitly asked to prioritize. |
| Deeper predictive optimization | Same technical dependency as §6.3 above (a real seasonal/ARIMA model) — the current `PredictiveDriverCard` composes existing real signals (Holt forecast + workload growth + deploy events) rather than adding new modeling. |

---

## 8. Honesty audit — every "not real" claim in this codebase, in one place

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
  testing, not assumed; see §6.1.

---

_This document and `PROJECT_STATUS.md` should be read together: that one
answers "what should I do next and why," this one answers "show me
exactly where, in the code, that claim comes from."_
