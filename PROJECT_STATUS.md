# chidrixx — Project & Technical Status (Universal Reference)

_Last updated: 2026-08-05 (a real deterministic traffic-replay safety check for generated NetworkPolicies shipped — §3.22 — including a real live performance regression that was caught by measurement and reverted rather than shipped, a real Helm probe-headroom fix, a real JSX rendering bug, and a real committed-E2E-fixture correction; all line/file/test counts below re-measured against the current tree)_

This is the single, complete picture of chidrixx: what's real and
verified, what's explicitly not built (and why), what's actually left to
do, and — merged into this same file — the exhaustive engineering detail
underneath every claim: which file, which function, which DB column,
which test, which command proved it. Nothing here is recalled from
memory; every number in this update was freshly re-measured against the
actual repository, not carried forward from an earlier pass — commit at
write time: `git log -1` → `99f9548`, "controlplane: real offline
placement simulator (idea #2, safe increment)". Verified by running
`go build`, `go test`, `go test -race`, `gofmt -l`, `find`/`wc`/`grep`,
`docker build`, `helm lint`/`template`, live Playwright/curl passes, and
— for the live-cluster claims specifically — `kubectl top`, `EXPLAIN
QUERY PLAN`, and a fresh `kubectl cp` + `PRAGMA integrity_check` pull of
the actual production database — all against both a real k3d cluster
and a real running `controlplane`
server, not written and
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
├── agent/               module `chidrixx`            — 3,159 lines Go (excl. tests), 14 files
│   └── cmd/
│       ├── kharcha/     the eBPF agent binary          — 12 test files, 26 test functions
│       └── loadgen/     load-test traffic generator (no tests)
├── controlplane/        module `chidrixx-controlplane` — 6,071 lines Go (excl. tests), 40 files
│   ├── web/             React/Vite/TS dashboard SPA    — 5,909 lines TS/TSX, 39 .tsx + 10 .ts files
│   └── e2e/             committed Playwright E2E suite — 11 .ts files (§3.10)
├── bpf/                 flow_cgroup.c + compiled flow_cgroup.o (committed binary, go:embed'd)
├── pricebook/            aws.yaml, gcp.yaml — real cited price data
├── deploy/helm/          kharcha/ + controlplane/ charts, 18 template/value files total
├── test/load/            10k-concurrent-flow load harness + its own Dockerfile
└── .github/workflows/   ci.yml — 7 jobs, both modules (§6)
```

Total: **~15,100 lines** of hand-written Go + TypeScript across both
modules and the frontend (not counting vendored react-bits `.jsx`/`.css`
files, which are third-party, installed via the real `shadcn` CLI; not
counting the separate 11-file E2E suite). All four counts above were
re-measured directly against the current tree while writing this
update, not carried forward from an earlier pass.

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

### 3.2 API routes — 21 real endpoints + the static SPA shell route, exact

| Route | Method(s) | Auth | Handler |
|---|---|---|---|
| `/api/v1/ingest` | POST | `requireAPIToken` | `handleIngest` |
| `/api/v1/findings` | GET | `requireSession` | `handleFindingsAPI` |
| `/api/v1/dashboard-summary` | GET | `requireSession` | `handleDashboardSummary` |
| `/api/v1/budget` | GET / POST | `requireSession` (POST also `requireAdmin`) | `budgetRoute` |
| `/api/v1/teams` | GET / POST / DELETE | `requireSession` (POST+DELETE also `requireAdmin`) | `teamsRoute` |
| `/api/v1/invites` | GET / POST / DELETE | `requireSession` + `requireAdmin` (all methods, including GET) | `handleInvites` |
| `/api/v1/workload-growth` | GET | `requireSession` | `handleWorkloadGrowth` |
| `/api/v1/outcomes` | GET | `requireSession` | `handleOutcomes` |
| `/api/v1/outcomes/apply` | POST | `requireSession` | `handleMarkOutcomeApplied` |
| `/api/v1/outcomes/stats` | GET | `requireSession` | `handleOutcomeStats` — real aggregate shown/applied/measured counts + mean prediction error (§3.19) |
| `/api/v1/chat` | POST | `requireSession` | `handleChat` (503 if `GROQ_API_KEY` unset) |
| `/api/v1/anomalies/narrate` | POST | `requireSession` | `handleNarrateAnomaly` (503 if `GROQ_API_KEY` unset) |
| `/api/v1/ai-eval/stats` **(new)** | GET | `requireSession` | `handleAIEvalStats` — real per-feature success rate/latency/tool success/token cost for the chat assistant and anomaly narrator (§3.20) |
| `/api/v1/anomalies/alerts` **(new)** | GET | `requireSession` | `handleAnomalyAlerts` — real, unacknowledged, proactively-detected anomalies (§3.21) |
| `/api/v1/anomalies/alerts/acknowledge` **(new)** | POST | `requireSession` | `handleAcknowledgeAnomalyAlert` — dismiss one real alert |
| `/api/v1/forecast` | GET | `requireSession` | `handleForecast` — `?cluster_id=X`, real backtested model selection (§3.14) |
| `/api/v1/remediation/preview` | GET | `requireSession` | `handleRemediationPreview` — real dry-run auto-remediation policy evaluation, never mutates anything (§3.16); each qualifying fix's generated manifest is additionally replayed against real historical traffic for collateral impact (§3.22) |
| `/api/v1/placement/preview` | GET | `requireSession` | `handlePlacementPreview` — real offline graph-partitioning simulation, `?groups=N` (§3.17) |
| `/api/v1/auth/me` | GET | `requireSession` | `handleMe` |
| `/api/v1/auth/login` | POST | none (that's the point) | `handleLogin` |
| `/api/v1/auth/logout` | POST | none | `handleLogout` |
| `/` | GET | none (static SPA shell, no secrets embedded) | `webAssetsHandler` |

### 3.3 SQLite schema — 12 tables, exact DDL shape

| Table | Key columns | Notes |
|---|---|---|
| `flow_aggregate` | `id` PK, `tenant_id`, `cluster_id`, `reported_at`, `src_workload`, `dst_workload_or_endpoint`, `path_class`, `confidence`, `bytes_tx`, `bytes_rx`, `cost_low_inr`, `cost_high_inr`, `fix_hint`, `fix_manifest`, `cloud`, `region`, `savings_low_inr`, `savings_high_inr` | Index on `(cluster_id, reported_at)`. Raw rows older than the retention window (default 30 real days) are now folded into `flow_aggregate_daily` and deleted by a real background compactor — see §3.18. No longer unboundedly retained by design. |
| `flow_aggregate_daily` **(new)** | `id` PK, `tenant_id`, `cluster_id`, `day`, `src_workload`, `dst_workload_or_endpoint`, `path_class`, `bytes_tx`, `bytes_rx`, `cost_low_inr`, `cost_high_inr`, `savings_low_inr`, `savings_high_inr`, `sample_count` | Unique on `(tenant_id, cluster_id, day, src_workload, dst_workload_or_endpoint, path_class)`. The real cold-tier rollup §3.18's compactor writes into — one row per real day instead of one per real ingest cycle. `sample_count` records how many raw rows were folded in, for honest transparency about granularity lost. |
| `settings` | `(tenant_id, key)` composite PK, `value` | Rebuilt mid-session from a single-column `key` PK — see bug #6 below. |
| `tenants` | `id` PK, `name`, `created_at` | |
| `users` | `id` PK, `tenant_id` FK, `username` UNIQUE, `password_hash` (bcrypt), `role`, `created_at` | |
| `api_tokens` | `id` PK, `tenant_id` FK, `token_hash` UNIQUE (SHA-256), `label`, `created_at` | Plaintext token is never stored, only ever shown once at creation. |
| `sessions` | `id` PK (opaque random, doubles as cookie value), `user_id` FK, `tenant_id` FK, `created_at`, `expires_at` | Server-tracked — `DELETE`-revocable, unlike a stateless JWT. |
| `team_ownership` | `(tenant_id, namespace)` composite PK, `team`, `created_at` | |
| `deploy_event` | `id` PK, `tenant_id`, `cluster_id`, `namespace`, `name`, `reason`, `message`, `occurred_at` | Index on `(tenant_id, cluster_id, occurred_at)`. |
| `invites` | `id` PK, `tenant_id` FK, `email` UNIQUE, `role`, `created_at` | Upserted by email (`ON CONFLICT(email) DO UPDATE`); deleted atomically on acceptance (`AcceptInvite`). |
| `recommendation_outcomes` | `id` PK, `tenant_id`, `cluster_id`, `source`, `destination`, `path_class` (unique together), `fix_hint`, `predicted_savings_low/high_inr`, `cost_before_inr`, `first_shown_at`, `last_shown_at`, `applied_at`, `cost_after_inr`, `measured_at` | See §3.11. `cost_before_inr`/predicted savings freeze once `applied_at` is set. |
| `ai_eval_events` **(new)** | `id` PK, `tenant_id`, `feature`, `occurred_at`, `latency_ms`, `success`, `hit_round_limit`, `rounds`, `tool_call_count`, `tool_call_errors`, `prompt_tokens`, `completion_tokens`, `error_message` | Index on `(tenant_id, feature)`. One real row per real chat turn or anomaly-narration call — see §3.20. |
| `anomaly_alerts` **(new)** | `id` PK, `tenant_id`, `cluster_id`, `detected_at`, `snapshot_reported_at`, `previous_cost_inr`, `current_cost_inr`, `growth_ratio`, `acknowledged_at` | Unique on `(tenant_id, cluster_id, snapshot_reported_at)` — the real dedup key that stops the same still-true anomaly from being re-alerted every tick. See §3.21. |

`users` also gained a `supabase_user_id TEXT` column **(new)**, nullable
(empty for CLI-provisioned accounts, which keep using
`password_hash`/bcrypt), with a partial unique index
(`idx_users_supabase_user_id ... WHERE supabase_user_id IS NOT NULL`) —
SQLite's `ALTER TABLE ADD COLUMN` can't declare `UNIQUE` directly, so the
uniqueness is enforced via the index instead.

Migrations are all `ALTER TABLE ... ADD COLUMN` (idempotent, "duplicate
column name" errors swallowed) except the `settings` table rebuild,
which detects the old schema via `sqlite_master.sql` and does a real
`CREATE new → INSERT SELECT → DROP → RENAME` inside one transaction.

### 3.4 Control plane file-by-file (39 files)

| File | Role |
|---|---|
| `main.go` | Entrypoint + 3 CLI subcommands (`create-tenant`, `create-user`, `create-token`) + `bootstrapDefaultTenant()` (idempotent, only fires when `TenantCount()==0`) + the route mux. |
| `store.go` | `Store` wraps one `*sql.DB`. Every query method takes `tenantID int64` as its first real parameter. |
| `model.go` | Wire structs: `Finding` (14 fields incl. `SavingsLowINR`/`SavingsHighINR`), `DeployEvent`, `IngestRequest` (now carries `Events []DeployEvent`). |
| `api.go` | `handleIngest` (also calls `store.IngestDeployEvents` best-effort), `handleFindingsAPI`. |
| `summary_api.go` | `handleDashboardSummary` — response now includes `SpendByTeam`. |
| `anomaly.go` | `detectAnomalies()` — 2x-growth-ratio detection; also calls `RecentDeployEvents()` in a 30-minute lookback and attaches `LikelyCause *DeployEvent`; `Anomaly` now also carries `SnapshotReportedAt` — the real per-snapshot identity `anomaly_watch.go` (§3.21) uses to dedupe alerts. |
| `budget_api.go` | `handleBudget` — GET/POST, POST gated by `requireAdmin`. |
| `auth.go` | Middleware: `requireAPIToken`, `requireSession`, `requireAdmin`. |
| `auth_api.go` | `handleLogin`/`handleLogout`/`handleMe`. |
| `tenant.go` | `Tenant`/`User`/`Session` structs, `CreateTenant` (atomic tx), `CreateUser`, `CreateAPIToken` (token rotation), `AuthenticateUser` (bcrypt, dummy-hash timing guard), `AuthenticateAPIToken` (SHA-256 lookup), `CreateSession`/`GetSession`/`DeleteSession`, `AllTenantIDs()` (new — every background loop that must sweep all real tenants, e.g. §3.21, uses this). |
| `team.go` | `TeamOwnership` CRUD, `extractNamespace()` (RFC-1123-label regex), `computeSpendByTeam()` (pure grouping function). |
| `team_api.go` | `handleTeams` — GET (any role) / POST+DELETE (admin only via `teamsRoute`). |
| `deployevent.go` | `IngestDeployEvents()`, `RecentDeployEvents(tenantID, clusterID, since, until)`. |
| `workloadgrowth.go` | `WorkloadCostGrowth(tenantID, topN)` — ranks workloads by real first→last-snapshot delta across full retained history (now `UNION ALL`s `flow_aggregate` with `flow_aggregate_daily`, §3.18, so compaction can't quietly shift what "first appearance" means); correlates each with `RecentDeployEvents` in its own namespace over its own trend window. |
| `workloadgrowth_api.go` | `handleWorkloadGrowth` — its own route, not folded into dashboard-summary (scans full history, not just latest-per-cluster). |
| `webassets.go` | `go:embed`'s `web/dist`, serves the SPA + static assets, no auth. |
| `supabase_auth.go` | `SupabaseAuthenticator.VerifyToken` — real `GET /auth/v1/user` call, not local JWKS verification. |
| `invite.go` | `Invite` CRUD, `AcceptInvite`, `ResolveOrProvisionSupabaseUser` — invite-before-new-tenant precedence. |
| `invite_api.go` | `handleInvites` — GET/POST/DELETE, all admin-only including GET. |
| `outcome.go` **(new)** | `RecommendationOutcome`, `RecordRecommendationsShown` (upsert, freezes on applied), `MarkRecommendationApplied`, `measurePendingOutcomes` (the real before/after measurement), `ListRecommendationOutcomes`. |
| `outcome_api.go` **(new)** | `handleOutcomes` (GET), `handleMarkOutcomeApplied` (POST). |
| `groq.go` **(new)** | `GroqClient` — OpenAI-compatible chat-completions HTTP client, no SDK dependency. `Complete()` now also returns a real `CompletionUsage` (prompt/completion tokens) parsed from Groq's own `usage` field, for §3.20. |
| `chat_tools.go` **(new)** | `buildChatTools` — the 5 real tenant-scoped tools; `parseLenientInt` (works around a real Groq/Llama schema-validation quirk, see §3.12). |
| `chat_api.go` | `handleChat`, `runChatLoop` (the tool-calling loop, bounded retry) — now returns a real `chatLoopResult` (rounds, tool-call count/errors, token usage, whether it hit the round limit), recorded via `RecordAIEvalEvent` after every real call (§3.20). |
| `anomaly_narrator.go` **(new)** | `narrateAnomaly` — single-completion (no tool-calling) explanation of one real, already-computed `Anomaly`; also returns real `CompletionUsage` now, recorded the same way (§3.20). |
| `anomaly_narrator_api.go` | `handleNarrateAnomaly` — `POST /api/v1/anomalies/narrate`, re-derives the anomaly fresh server-side. |
| `forecast.go` **(new)** | `holtFit`/`holtForecastAhead` (damped-trend generalized Holt), `fitBestHolt`, `backtestMAE` (rolling-origin validation), `ComputeDeepForecast` (the model-selection entry point). |
| `forecast_api.go` | `handleForecast` — `GET /api/v1/forecast?cluster_id=X`. |
| `summary.go` | `computeSummary`/`computeSpendByClass`/`computeSpendByCloud` — pure Go aggregation over already-fetched findings, replacing 3 redundant SQL scans (§3.15). |
| `remediation.go` **(new)** | `RemediationPolicy`, `EvaluateRemediation` — real dry-run closed-loop remediation policy engine (§3.16); plus `SimulateTrafficReplay`, which runs §3.22's real safety check over qualifying decisions only. |
| `remediation_api.go` | `handleRemediationPreview` — `GET /api/v1/remediation/preview`. |
| `traffic_replay.go` **(new)** | `parseGeneratedNetworkPolicy`, `EvaluateTrafficReplay` — real deterministic replay of a generated NetworkPolicy against this tenant's own real historical traffic, to find collateral impact before ever calling a fix safe to auto-apply (§3.22). |
| `placement.go` **(new)** | `buildPlacementGraph`, `OptimizePlacement` — real Kernighan-Lin graph partitioning with multi-restart (§3.17). |
| `placement_api.go` **(new)** | `handlePlacementPreview` — `GET /api/v1/placement/preview?groups=N`. |
| `compaction.go` **(new)** | `CompactFindingsOlderThan` (real day-bucketed rollup + atomic delete), `StartCompactionLoop` (background ticker) — the real retention/compaction fix, §3.18. |
| `outcome_stats.go` **(new)** | `OutcomeDatasetStats` — real shown/applied/measured counts + mean prediction error over `ListRecommendationOutcomes`, §3.19. |
| `aieval.go` **(new)** | `RecordAIEvalEvent`, `AIEvalStats` — real per-feature AI telemetry aggregation, §3.20. |
| `aieval_api.go` **(new)** | `handleAIEvalStats` — `GET /api/v1/ai-eval/stats`. |
| `anomaly_watch.go` **(new)** | `WatchTenantForNewAnomalies`, `StartAnomalyWatchLoop`, `UnacknowledgedAnomalyAlerts`, `AcknowledgeAnomalyAlert` — real proactive anomaly detection, §3.21. |
| `anomaly_watch_api.go` **(new)** | `handleAnomalyAlerts` (GET), `handleAcknowledgeAnomalyAlert` (POST). |

### 3.5 Control plane dependencies (`go.mod`, exact versions)

Direct: `golang.org/x/crypto v0.54.0` (bcrypt), `modernc.org/sqlite
v1.54.0` (pure-Go, no cgo), and — new with §3.22 — `gopkg.in/yaml.v3
v3.0.1`, for really parsing the generated NetworkPolicy manifests rather
than hand-rolled string matching (the same library the agent module
already uses for its price books, so it's not a new library to the repo,
only to this module). 8 indirect deps, all `modernc.org/sqlite`'s own
transitive requirements (libc, mathutil, memory, bigfft, etc.) plus
`google/uuid` and `go-isatty`. The Groq client (§3.12) adds zero new
dependencies — it's a plain `net/http` client against Groq's
OpenAI-compatible REST API, not an SDK.

### 3.6 Control plane test inventory — 209 test functions across 31 files

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
| `supabase_auth_test.go` | 5 | Token verification against a fake Supabase server, bearer-vs-cookie fallback, provision-exactly-once |
| `invite_test.go` + `invite_api_test.go` | 14 | Upsert-replace semantics, role validation, revoke, join-via-invite, tenant isolation, admin-only-including-GET |
| `outcome_test.go` + `outcome_api_test.go` | 14 | Shown/freeze-on-applied upsert, idempotent apply, real cost-after measurement (both "fixed" and "flow gone" cases), tenant isolation |
| `outcome_stats_test.go` + additions to `outcome_api_test.go` **(new)** | 6 | All-zero honest state with no data, shown-without-applied leaves the mean prediction error `nil` (not fabricated 0), a real computed mean prediction error against a known predicted-vs-actual gap, tenant isolation at both the store and API layer |
| `chat_test.go` | 14 | Groq client against a fake server (including real token-usage parsing), the tool-calling loop (success/unknown-tool/max-rounds/retry, now also asserting real `Rounds`/`ToolCallCount`/`ToolCallErrors`/`HitRoundLimit`), tenant-scoped tool isolation, `parseLenientInt`, the full HTTP handler, and that `handleChat` records a real ai-eval event on both success and Groq failure |
| `anomaly_narrator_test.go` | 6 | Prompt grounding (with/without a real likely cause), 503/404/tenant-isolation at the API layer; the end-to-end test now also asserts `handleNarrateAnomaly` records a real ai-eval event |
| `forecast_test.go` | 11 | Synthetic linear series picks plain Holt / synthetic plateauing series picks damped Holt (both asserted via real measured MAE), honest zero-fold fallback with too little history, damped-never-exceeds-undamped invariant, a real wall-clock budget at 4,200-point production scale, API layer (400/tenant-isolation/end-to-end) |
| `summary_test.go` | 5 | Pure Go aggregation functions cross-checked against the same fixtures/expected values as the original SQL-based `Summary`/`SpendByClass`/`SpendByCloud` tests in `store_test.go` |
| `remediation_test.go` + `remediation_api_test.go` | 9 | Each qualifying/disqualifying policy reason checked individually, a real no-mutation guarantee, API-layer tenant isolation |
| `placement_test.go` + `placement_api_test.go` | 15 | Synthetic graphs with analytically-known-correct answers (disconnected pairs, a triangle, a 4-workload/2-pair case proving the algorithm finds the mathematically best co-location, not just *a* local optimum), a real wall-clock budget at 100-workload scale, API layer |
| `compaction_test.go` | 8 | Real sum/`sample_count` correctness folding multiple raw rows into one rollup, recent rows left untouched, idempotent re-run over already-compacted data, separate real calendar days never merged, tenant isolation, the `WorkloadCostGrowth`-survives-compaction integration test, a real no-op guard when nothing is old enough yet |
| `aieval_test.go` + `aieval_api_test.go` **(new)** | 6 | Empty-with-no-events state, real per-feature aggregation (success rate, avg latency, tool success rate, token sums) against known inputs, tenant isolation at both the store and API layer, wrong-method rejection |
| `anomaly_watch_test.go` + `anomaly_watch_api_test.go` **(new)** | 14 | A real new alert recorded and idempotent across repeated ticks with no new data, a genuinely new alert after a fresh anomalous snapshot, no alert when growth is below threshold, acknowledge removes it from the unacknowledged list (idempotently), 404 for an unknown alert ID, tenant isolation, `AllTenantIDs`, API-layer coverage of all of the above |
| `traffic_replay_test.go` **(new)** + additions to `remediation_test.go`/`remediation_api_test.go`/`store_test.go` | 12 | Real manifest parsing (IPv4 and IPv6, mask stripped; a real parse error for a bare scalar; a well-formed manifest with no `ipBlock` yielding no blocked IPs), the safe case, real collateral in the same namespace, a different namespace correctly ignored, the honest not-confirmed-safe case with no history, deterministic ordering; orchestration-level: unsafe overrides an otherwise-qualifying decision and replaces the stale reason, no manifest and already-disqualified decisions are never looked up at all, an unparseable manifest is treated conservatively, real fetch errors propagate, the policy toggle still computes but doesn't enforce; store-level: sums across raw + daily rollups (survives compaction) and tenant/cluster isolation; API-level: a real end-to-end collateral case through the real store and handler |

Two new tests in `store_test.go` guard the WAL/connection-pool change
specifically: `TestOpenStoreConnectionsShareRealDataAgainstAFile` (8 real
concurrent connections against a real file all see the same real
ingested row) and the `:memory:`-stays-single-connection guard baked
into `OpenStore` itself.

Run: `cd controlplane && go test ./...` — **all 209 passing** (re-verified
2026-08-05, `go test -race` clean at 331s), ~70s wall time without
`-race` (several tests use real 1.1s sleeps to get distinct `reported_at`
second-granularity timestamps).

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
8. **Cookie/CLI accounts could reach the dashboard, but never sign
   in to it — caught live, by hand, not by a test.** §3.9's fix made
   `App.tsx`'s session check work for cookie sessions on page load, but
   `LoginPage.tsx`'s actual form was, and had remained, Supabase email/
   password only (`type="email"`, `supabase.auth.signInWithPassword`) —
   there was never a visible way to *create* a cookie session through
   the browser at all. A real self-hosted/CLI-provisioned username typed
   into that field was rejected by the browser's own HTML5 validation
   ("Please include an '@'") before any app code even ran. Every E2E
   auth test (§3.10) exercises the cookie path via `context.request.post`
   directly, bypassing the real form entirely, so this never surfaced in
   CI — only found by actually trying to log in through the live browser
   UI. Fixed by adding a real "Sign in with username instead" mode to
   `LoginPage.tsx` that posts straight to `POST /api/v1/auth/login`
   (§3.9's own already-correct backend path) and resolves the session the
   same way the Supabase flow does — plus two new E2E tests that fill out
   and submit the actual rendered form, not the API directly, so this
   class of bug can't silently return (24/24 E2E now, up from 22/22).
   Verified live: a real browser, no stored cookies, going landing page →
   "Sign in with username instead" → real `verify-fix` credentials →
   real dashboard, screenshotted at each step.

### 3.9 Real Supabase-backed signup/login + self-service team invites

Additive to the existing cookie/CLI auth (§3.1) — self-hosted,
CLI-provisioned tenants keep working unchanged, since `requireSession`
now checks for a Supabase `Authorization: Bearer` header first and falls
back to the legacy cookie path when there isn't one.

- **Supabase auth.** `supabase_auth.go`'s `SupabaseAuthenticator`
  verifies a token by calling the real `GET /auth/v1/user` against the
  live project (`jccydmmygpfdkufkswcw.supabase.co`) — not local JWKS
  verification, a deliberate simplicity/correctness tradeoff. On first
  login with a new Supabase identity, `ResolveOrProvisionSupabaseUser`
  either joins a pending invite (below) or calls
  `ProvisionTenantForSupabaseUser` to atomically create a brand-new
  tenant + admin user + ingest token — i.e. **Supabase login is now a
  real public signup path**, layered on top of (not replacing) the
  operator-only `create-tenant` CLI. Verified live against the actual
  Supabase project: real signup, real email-confirmation-required
  behavior, real rate-limit response surfaced honestly in the UI rather
  than worked around.
- **Team invites** (`invite.go`/`invite_api.go`, new `invites` table).
  An admin can add a viewer/co-admin without shell access: `POST
  /api/v1/invites` (admin-only) stores a pending `(tenant_id, email,
  role)` row; the invited person's *first* Supabase login checks for a
  pending invite before falling back to provisioning a brand-new tenant,
  and joins the inviter's exact tenant with the assigned role
  (`AcceptInvite`, atomic tx: insert user + delete invite). The entire
  `/api/v1/invites` resource, including `GET`, is admin-gated — a viewer
  can't even list pending invites. `TeamsPage.tsx`'s new `MembersCard`
  (admin-only) is the UI: an email+role form, a pending-invites table
  with revoke buttons, no link-sending or shell access needed. Verified
  live end-to-end with two real Supabase users through the actual
  browser UI: founder invites a teammate by email, the teammate's first
  real login lands in the founder's exact tenant with the invited role.
- **A real bug this caught**: `App.tsx`'s top-level auth check originally
  gated the call to `/api/v1/auth/me` behind "does a Supabase session
  exist," which made the legacy cookie-session path (every self-hosted/
  CLI-provisioned tenant) completely unreachable through the dashboard.
  Caught while designing the E2E auth tests (§3.10), fixed by always
  calling `/api/v1/auth/me` unconditionally on load.
- **A second, related real bug, caught later by actually using the live
  site, not by any test**: fixing the session *check* above didn't fix
  the session *creation* path — `LoginPage.tsx` had no UI at all for
  logging in with a cookie/CLI username, only the Supabase email form.
  See §3.8 item 8 for the full story and the fix (a real "Sign in with
  username instead" mode, backed by the existing `POST
  /api/v1/auth/login`, plus two new E2E tests that drive the actual
  rendered form).

### 3.10 Real Playwright E2E suite (`controlplane/web/e2e/`)

Committed and green — no longer the "throwaway scripts, never
committed" gap described in earlier drafts of this document (see the
now-corrected §6/§8 below). `globalSetup.ts` builds the actual
`controlplane` binary from source, provisions a real tenant + admin +
viewer via the real `create-tenant`/`create-user` CLI subcommands (the
same code path a real operator uses), starts the real server, and
ingests real findings via a real HTTP POST to `/api/v1/ingest` — the
same discipline every manual verification pass in this project has used
all along, now automated and repeatable instead of one-off. Runs against
the system's `/usr/bin/google-chrome` (Playwright's bundled-browser
install needs root, unavailable in this sandbox) via `launchOptions` in
`playwright.config.ts`. **28 tests across 7 files** (`auth`, `dashboard`,
`role-gating`, `teams`, `assistant`, `forecast`, `anomaly-watch`), all
passing against the live server: session cookie reaches the dashboard,
wrong password gets a real 401, logout actually revokes the server-side
session, a viewer's direct API POSTs get real 403s, an admin's real
namespace→team mapping and real teammate invite both round-trip through
the actual API. Caught a real bug along the way (the logout regression in
§3.9). Run: `cd controlplane/web && npm run test:e2e` (needs Node ≥20;
system Node in this dev environment is 18, worked around with a portable
Node 20 install at `~/.local/node20`).

### 3.11 Closed-loop recommendation outcome tracking (`outcome.go`/`outcome_api.go`)

The foundational data layer the AI roadmap needs before any model gets
trained or prompted against real outcomes — not an AI feature itself,
deliberately built first. New `recommendation_outcomes` table
(`(tenant_id, cluster_id, source, destination, path_class)` unique):
every real fix recommendation the dashboard surfaces is auto-logged on
each `dashboard-summary` load (best-effort, like deploy-event ingestion),
capturing the real predicted savings and real pre-fix cost. An operator
can mark one applied via a real "Mark as applied" button on the top-fixes
table (`POST /api/v1/outcomes/apply`) — once applied, the baseline
(`cost_before_inr`) freezes even if the same finding keeps showing up
with a different cost on later loads. Once a fresher snapshot exists for
that cluster, `measurePendingOutcomes` automatically records the real
observed post-fix cost for the same source→destination pair (any
path_class, since a working fix often changes the class rather than
keeping it) — `0` if the flow disappeared from the latest snapshot
entirely (a genuine success, not missing data), the observed cost
otherwise. `GET /api/v1/outcomes` (any role) returns the full history.
13 tests: upsert/freeze-on-applied semantics, idempotent apply,
`ErrOutcomeNotFound` vs. already-applied, real cost-after measurement
(both the "fixed" and "flow gone" cases), staying honestly unmeasured
with no fresher data, tenant isolation. Verified live via the E2E suite
(a real browser click round-trips through the real API).

### 3.12 Real grounded chat assistant (`groq.go`/`chat_tools.go`/`chat_api.go`)

A tenant-scoped chat assistant (`POST /api/v1/chat`, new Assistant
sidebar page) that answers cost questions by having an LLM call real
tools bound to this tenant's actual data — never inventing a number.
Backed by Groq's OpenAI-compatible chat-completions API (Llama 3.3 70B
by default, `GROQ_MODEL` env override), not a self-hosted model: this
dev environment has no GPU, and reliable tool-calling needs one to run
at usable chat speed — a small CPU-only model would be both too slow and
too unreliable at tool-calling. Opt-in via `GROQ_API_KEY`, same additive
pattern as Supabase auth's `SUPABASE_URL` (unset = a real, honest 503,
not a crash or a silent fake reply).

Five real tools, each wrapping an already-tested store method,
tenant-scoped by Go closure (not by trusting the model's output):
`get_top_fixes`, `get_anomalies`, `get_workload_growth`,
`get_spend_by_team`, `get_recommendation_outcomes` (§3.11's data). The
tool-calling loop (`runChatLoop`) caps at 5 rounds and gives an honest
"couldn't finish" reply rather than looping forever.

**A real bug caught during live verification against the actual Groq
API**: Groq's server-side JSON-schema validation rejects a tool call
outright when the model stringifies an integer argument — a
reproducible Llama-on-Groq quirk, not a transient error (confirmed by
retrying and getting the identical failure every time). Fixed by
declaring the two numeric tool parameters (`limit`, `top_n`) as
string-typed in the schema and parsing leniently on the Go side
(`parseLenientInt`, handles both a JSON string and a real JSON number),
plus a separate bounded one-shot retry in the loop for genuinely
transient failures (a different, real failure class, tested with a
fake server that fails once then succeeds).

Verified: 24 new Go tests (a `fakeGroq` `httptest.Server` mirroring the
real API contract, same pattern as `fakeSupabase`) covering the client,
the tool-calling loop, tenant isolation of tool calls, and the API
layer; a live manual pass against the real Groq API with real ingested
data and a real browser (screenshot captured) — confirmed grounded
answers quoting real ₹ figures, an honest "no anomalies detected" when
there genuinely were none (not a fabricated one), and a correct refusal
of an off-topic question. The committed E2E suite adds a hermetic test
proving the honest-503 path (no `GROQ_API_KEY` in CI, matching the
Supabase-auth pattern of keeping real secrets out of the committed
suite) — not a live Groq call in CI.

### 3.13 Real anomaly root-cause narrator (`anomaly_narrator.go`/`anomaly_narrator_api.go`)

The third AI item from the same session's build order (outcome
tracking → chat assistant → this). Turns one already-computed real
`Anomaly` (previous/current cost, growth ratio, and the optional real
correlated deploy event from §3.7's root-cause correlation) into a short
plain-English explanation — a single Groq completion, not a tool-calling
loop, since all the real data is already known and there's nothing to
look up. New `POST /api/v1/anomalies/narrate` (`{cluster_id}` →
`{narrative}`), re-derives the anomaly fresh from real data server-side
rather than trusting client-supplied numbers. New "Explain this" button
on `AnomalyCard.tsx`, on-demand per anomaly — not run automatically for
every anomaly on every dashboard load, which would mean an LLM call per
anomaly per page view. The system prompt explicitly forbids upgrading
"likely cause" correlation into causation and forbids inventing any
figure not present in the input.

Verified: 6 new Go tests (prompt actually contains the real cluster ID
and real deploy event; the honest "none found" case when there's no
correlated event; 503/404/tenant-isolation at the API layer, same
`fakeGroq` pattern). Live manual pass against the real Groq API: a real
2-snapshot cost jump (₹10→₹60, 6x) with a real correlated deploy event
(`ScalingReplicaSet`, 2→10 replicas) produced a correctly grounded,
correctly hedged explanation, confirmed in a real browser (screenshot
captured). E2E suite adds the same hermetic honest-503 test as the chat
assistant (direct API call — `globalSetup.ts` only ingests one snapshot,
so there's no real 2-snapshot anomaly to click through in the committed
suite).

### 3.14 Real deeper forecasting model (`forecast.go`/`forecast_api.go`) — closes punch-list "deeper forecasting model"

Scoped against what the real data actually supports, checked before
building anything: a pulled copy of the real production database showed
one cluster with 4,164 snapshots over ~1.8 days (~37s cadence), another
with 5. Real volume, but nowhere near enough distinct calendar days for
a seasonal component (Holt-Winters/Prophet) to be anything but invented
pattern-matching on noise, and not enough/clean enough data to justify
ARIMA or a neural sequence model. What the real data *does* support:
genuine rolling-origin (walk-forward) backtesting comparing plain Holt
(the existing client-side model, ported to Go) against damped-trend Holt
(a real, well-established fix for Holt's known runaway-extrapolation
weakness) — picking whichever actually measured lower error on that
cluster's own held-out real history, server-side, against the cluster's
full retained history (up to 5,000 points) rather than the 30-point cap
the lightweight Overview trend card uses.

`GET /api/v1/forecast?cluster_id=X` returns which model won, its
backtested MAE for both candidates, how many real folds ran (0 if there
wasn't enough history — an honest single-Holt fallback, not a forced
comparison), and the forecast itself. New `DeepForecastCard.tsx` on the
Forecasting page, per-cluster selector.

**Verified two ways.** Synthetic: a genuinely unbounded linear series
correctly picks plain Holt; a series that grows then plateaus correctly
picks damped Holt — asserted via real measured MAE comparison in the
test, not hand-waved. Live, against a pulled copy of the actual
production database: on the real 4,164-point cluster, 20 real backtest
folds initially picked damped Holt (MAE 0.0200 vs 0.0222); re-run
minutes later with 39 more real snapshots the live agent had ingested in
the meantime, it correctly flipped to plain Holt (0.015 vs 0.028) —
proof this is a live-recomputed real decision, not a fixed choice. The
sparse 5-point cluster correctly falls back to a single Holt fit with 0
backtest folds, no false claim of a comparison having run.

### 3.15 Real live performance investigation and fix — 37s to under 6s

Triggered by a real user report against the live k3d deployment
("all the sections loading is taking too long") right after §3.9-§3.14's
features were first deployed there. Investigated live, not guessed, at
every step:

- `kubectl top pod` showed the pod pegged at ~1 full CPU core sustained.
- A pulled copy of the actual production database (`kubectl cp`,
  integrity-checked) showed `flow_aggregate` had grown to **~2 million
  rows / ~500MB** — never pruned, by design (§3.3).
- `EXPLAIN QUERY PLAN` against that copy confirmed the relevant queries
  *were* using the right index — ruling out a missing-index theory.
- A trivial endpoint (`/api/v1/auth/me`, touching only the tiny
  `sessions`/`users` tables) still took **4+ real seconds** — proving
  the bottleneck was connection-level queuing, not query cost, before
  any code changed.

Five real, independently-verified fixes, each rebuilt, redeployed to the
live cluster, and re-measured before moving to the next:

1. **Helm chart gap closed**: `values.yaml`/`deployment.yaml` gained
   `GROQ_API_KEY`/`GROQ_MODEL` support — the three AI features (§3.11-
   §3.14) had never actually been deployable to the cluster until this;
   this is also what made this whole investigation possible; live
   users only discovered the slowness because the AI features arriving
   is what prompted the redeploy in the first place.
2. **`maxFitWindow` in `forecast.go`**: bounded the Holt fit to the most
   recent 400 points instead of refitting from the start of full history
   on every backtest fold — was measuring ~1.6 real CPU-seconds per
   request. New regression test asserts a real wall-clock budget (60ms)
   against 4,200 real-scale points.
3. **Batched writes in `outcome.go`**: `RecordRecommendationsShown` and
   `measurePendingOutcomes` ran one auto-committed transaction (one real
   fsync, on this storage layer) per row — up to 10 per single
   dashboard-summary request. Now one transaction for the whole batch.
4. **WAL mode in `store.go`**: enabled `journal_mode=WAL` +
   `synchronous=NORMAL` + `busy_timeout=5000` via per-connection DSN
   pragmas, and raised `MaxOpenConns` from 1 to 4 — the original
   single-connection choice existed specifically to avoid rollback-
   journal lock errors, exactly what WAL removes (safe concurrent
   readers, one writer). **Caught a real latent bug this exposed**: a
   bare `":memory:"` DSN gives every pooled connection its own
   independent, empty database (confirmed directly with a throwaway
   script) — the test suite's `testStore()` helper relies on `:memory:`
   and had been passing purely by luck of sequential connection reuse.
   Fixed by pinning `:memory:` to a single connection regardless, with a
   new test proving real concurrent connections against an actual file
   share data correctly.
5. **Eliminated redundant table scans** (new `summary.go` +
   `anomaly.go`): `Summary`/`SpendByClass`/`SpendByCloud` were each
   independently re-deriving "the latest snapshot per cluster" via their
   own SQL query, when `dashboard-summary` already fetches exactly that
   data via `LatestFindings` — rewritten as pure Go aggregations over the
   one fetch (the same pattern `computeSpendByTeam` already used).
   `detectAnomalies` no longer calls `store.Clusters` itself; callers
   that already have the list pass it in.

**Net result, measured live before and after, real numbers**:
`auth/me` 4.2s → 34ms; `findings` 7.4s → ~2s; `forecast` 8-12s → ~1.5s;
`dashboard-summary` 20-40s → ~5.8s (confirmed in a real browser: the
first real stat card now renders in ~7.8s total, down from a page that
previously never finished loading). Verified at every step: full Go
suite (130 tests, two new concurrency-specific ones) plus `go test
-race` clean both before and after the connection-pool change; full
Playwright E2E suite (19 tests) green throughout.

**Not attempted, and why**: a covering index or actual retention pruning
for `flow_aggregate` would cut the residual ~5.8s further, but both are
bigger decisions (a schema migration; a real data-retention policy) than
a bug fix — left for a separate call if it matters later.

### 3.16 Real dry-run closed-loop remediation preview (`remediation.go`/`remediation_api.go`)

The safe first increment of "closed-loop remediation" — one of three
directions considered for making the product genuinely differentiated,
not just well-engineered (the other two: a cost-aware placement
algorithm validated on real traces, and an eventual cross-customer
reference dataset, both still future vision, not started). Checked the
real fix-manifest generation (`agent/cmd/kharcha/fixengine.go`) before
designing anything: it's a real `NetworkPolicy` that denies egress to
one flagged IP with `podSelector: {}` (every pod in the namespace) —
confirming blind real auto-apply would be genuinely risky, and that a
transparent, dry-run-only first pass was the right scope, not a
shortcut.

`EvaluateRemediation` is a real, deterministic policy (pure Go function
over already-fetched findings, same pattern as `computeSpendByTeam`):
a fix qualifies as "would auto-apply" only if it has a real generated
manifest, real high confidence, and real positive predicted savings —
all three required, and every finding's decision — qualifying or not —
comes with an explicit real reason, not a silent filter. **Never
touches a real cluster**: chidrixx still has no write access to any
cluster (`AutomationsPage.tsx`'s long-standing disclaimer stays
literally true); this is the evidence-gathering phase before ever
requesting that access, not a quiet backdoor to it.

`GET /api/v1/remediation/preview` (any authenticated role, read-only)
serves this computed-on-demand — no new persistence, no new schema.
`AutomationsPage.tsx` — which already carried the "never applies
automatically" disclaimer, the natural home for this — gained a
transparent "Would apply" / "Would skip" breakdown.

Verified: 9 new Go tests (each disqualifying reason checked
individually — no manifest, low confidence, zero savings — a real
no-mutation guarantee test documenting that nothing is ever touched,
API-layer tenant isolation), full suite green. Added a second real E2E
fixture finding (a real `internet_egress`/high-confidence/manifest-
bearing fix) so the committed E2E suite exercises the real "would
apply" path live, not only "would skip" — full suite green (20/20).
Live-verified in a real browser against 3 real findings spanning all
three disqualifying reasons plus the qualifying case; screenshotted and
confirmed each displayed reason was correct.

### 3.17 Real offline placement simulator (`placement.go`/`placement_api.go`)

The safe, offline first increment of "idea #2" from the S+-innovation
discussion (a cost-aware placement algorithm): a real Kernighan-Lin
graph-partitioning heuristic answering "how much of this cluster's real
cross-zone cost is avoidable by co-locating workloads that talk to each
other a lot" — computed entirely from already-ingested real `CROSS_AZ`
findings, zero live-cluster access, zero new agent capability. Checked
what data actually exists before designing anything: the agent resolves
real zone identity per node at classification time
(`agent/cmd/kharcha/classify.go`) but only ships the resulting
`CROSS_AZ`/`SAME_AZ`/etc. label to the control plane, not raw zone
identity — so this can only answer "what's the best K-way split of this
communication graph," not "move workload X from us-east-1a to
us-east-1b." That would need a real agent change, not attempted here.

**Two real bugs caught and fixed before shipping, neither by assumption:**
1. The first version (single-node moves, no constraint) trivially
   collapsed every graph to 1 group — caught by a real test (a fully-
   connected triangle falsely reporting 0 avoidable cost). Fixed by
   requiring every one of the `numGroups` real zones to stay non-empty
   throughout — the honest framing: `numGroups` means "zones you need
   genuine redundant presence across" (e.g. real multi-AZ HA), not an
   arbitrary bucket count.
2. That fix introduced real order-dependency, caught by hand while
   screenshotting the live feature: a 4-workload/2-real-pair/3-zone
   example converged to only the *smaller* of two possible savings
   amounts, purely because of which pair the arbitrary initial split
   happened to favor. Pairwise swaps (the textbook Kernighan-Lin
   mechanism) were tried next and traced to preserve the initial group-
   size distribution forever, so they couldn't fix this either. The
   real fix: multiple deterministic restarts from varied initial splits,
   keeping whichever converges to the lowest real cost — re-verified
   live afterward that it now correctly finds the mathematically best
   answer (co-locating the *costlier* pair), not just re-trusted.

`GET /api/v1/placement/preview?groups=N` (default 3, any authenticated
role, read-only, computed on demand — no new persistence). New
`PlacementSimulatorCard.tsx` on the Cost Graph page (the natural home,
already about topology), with a zone-count selector and copy stating
plainly what "N zones" means and what real constraints (node capacity,
resource limits, anti-affinity) this doesn't model.

Verified: 15 new Go tests, including synthetic graphs with analytically-
known-correct answers (disconnected pairs, a triangle, the 4-workload/
2-pair case asserting the *specific* costlier-pair co-location, not just
*some* improvement) and a real wall-clock budget test at 100-workload
scale (following §3.15's hard lesson about shipping without one). Full
Go suite green (154 tests). Live-verified against a real running server
twice — the second pass is what caught the order-dependency bug, then
re-verified the rewritten algorithm live afterward. E2E suite extended
and green (21/21).

### 3.18 Real automated retention/compaction for `flow_aggregate` (`compaction.go`)

Direct response to a real, explicit ask: swap the SQLite backend for a
high-throughput time-series engine (e.g. ClickHouse), or implement
automated retention/compaction. Chose compaction, deliberately, not the
storage-engine swap — checked against this project's actual real scale
before deciding, not by default: one real tenant, SQLite+WAL already
fixed in §3.15 to genuinely handle millions of rows fine, and every
dashboard query that reads `flow_aggregate` other than
`WorkloadCostGrowth` only ever wants the latest snapshot per cluster or
an already-bounded recent window (checked directly against every
`FROM flow_aggregate` call site in `store.go`/`outcome.go`/
`forecast_api.go`, not assumed). A full storage-engine migration would
touch every one of those queries and the deployment topology for a
problem this solves directly — real, costly over-engineering for the
data volume that actually exists.

**How it works**: `CompactFindingsOlderThan(cutoff)` groups every raw row
reported before `cutoff` by real calendar day (UTC, `(reported_at /
86400) * 86400`), tenant, cluster, and workload pair, sums bytes/cost/
savings into `flow_aggregate_daily`, then deletes the raw rows just
folded in — all inside one transaction, rollup insert before raw delete,
so a crash partway through can never lose data without its aggregate
already durably written. `StartCompactionLoop` runs this once
immediately then on an hourly tick, logged, non-fatal on error (retries
next tick). `CHIDRIXX_RAW_RETENTION_DAYS` (default 30) lets an operator
tune the window without a rebuild.

**The one query this couldn't leave alone**: `WorkloadCostGrowth` ranks
workloads by cost change between "first ever appearance" and "most
recent" — if compaction silently deleted the raw row behind a workload's
first appearance, that delta would quietly drift forward every time old
data aged out, changing what "growth over full retained history"
honestly means without anyone asking for that. Fixed by `UNION ALL`ing
`flow_aggregate` with `flow_aggregate_daily` in that one query — proven,
not assumed, by a real integration test
(`TestWorkloadCostGrowthSurvivesCompactionOfItsFirstSnapshot`: ingest a
workload's first snapshot, compact it away, confirm the real 190-INR
delta is unchanged afterward, now sourced from the rollup).

**Verified against real production-scale data, not just synthetic
fixtures**: pulled a fresh copy of the live cluster's actual database
(2,369,010 real rows at pull time) and ran the exact real compaction SQL
against it directly (rolled back afterward — a read/verify pass, never
applied to the throwaway copy's own state or the live database). Result:
2,205,218 real raw rows folded into exactly 1,998 real daily rollups (a
**1,103x** real row-count reduction for the compacted slice), with a
real cost-conservation check (`remaining raw cost + rolled-up cost` had
to equal the pre-compaction total) passing exactly. `PRAGMA
integrity_check` on the copy: `ok`.

**Deliberately did not force a demonstration compaction against the live
database itself**: the live cluster's real retained history (~55 hours)
is nowhere near the 30-day default retention window, so deploying this
correctly compacts *nothing* yet on the real system — confirmed live via
the pod's own log line (`automated compaction enabled: raw
flow_aggregate rows older than 720h0m0s...`, no `folded N raw row(s)`
line, exactly as it should behave with fresh data). Lowering retention
just to force a visible live demo would have meant real, avoidable
downsampling of the only production dataset that exists, for a
correctness claim already proven rigorously against a real copy above —
not worth the risk for a demo.

8 new Go tests (`compaction_test.go`), full suite green (167 tests, `go
test -race` clean). Full Go build + `docker build` + `k3d image import`
+ `kubectl rollout restart`, re-verified live: `dashboard-summary` and
`workload-growth` both still return real correct data post-deploy
(~3.8s, consistent with §3.15, not regressed).

### 3.19 Real outcome-dataset-health visibility (`outcome_stats.go`)

Direct response to the second half of the same ask: get real operators
actively applying recommendations so `recommendation_outcomes` matures
into a dataset worth fine-tuning on. Being honest about what this
actually is: **not a code task**. No endpoint, model, or UI can
manufacture real operators applying real fixes over real time — §9
already said this plainly, and nothing here changes that underlying real
gap (still genuinely 0 real applied/measured rows on the live cluster,
confirmed directly — see below). What *is* buildable, and what this
ships, is honest visibility into that real gap, so progress toward it is
trackable instead of invisible the moment real usage starts.

`OutcomeDatasetStats(tenantID)` aggregates `ListRecommendationOutcomes`
(already-existing, already measures any newly-measurable pending
outcomes) in pure Go — real shown/applied/measured counts, plus
`MeanAbsPredictionErrorINR`: the real average gap between a fix's
predicted savings and what actually happened
(`cost_before_inr - cost_after_inr`) for outcomes that have real
measured data. A pointer, not a bare `0`, when nothing is measured yet —
reporting `0` there would dishonestly read as "predictions are perfect"
instead of "nothing to measure yet." `GET /api/v1/outcomes/stats`
exposes it; a new "Outcome dataset health" section on the Automations
page (natural home — already the closed-loop-remediation story) renders
the three real counts plus an explicit, honest empty state
("No recommendations marked applied yet — this is expected before real
operators are actively using the product") when nothing has been applied.

Verified live against the real running cluster, not just fixtures:
`GET /api/v1/outcomes/stats` returns `{"total_shown":26,
"total_applied":0,"total_measured":0}` — the real, current state of this
lab environment's dataset, screenshotted rendering correctly on the
actual live Automations page. This is the honest baseline the ask was
about: a real, non-fabricated 26 recommendations captured, 0 real
operator applications yet, because this environment doesn't have real
operators using it day-to-day yet — exactly the gap §9 already named.

6 new Go tests (`outcome_stats_test.go` + 2 additions to
`outcome_api_test.go`), full suite green. 1 new E2E test asserting the
real shown/applied/measured counts and the honest empty state
(22/22 E2E, up from 21/21).

### 3.20 Real AI evaluation telemetry (`aieval.go`/`aieval_api.go`)

Direct response to an external review's fair criticism: this project has
two real AI features (the chat assistant, the anomaly narrator) and,
until now, zero visibility into whether either one actually works —
success rate, latency, tool-call reliability, token cost. "We have AI"
with no evaluation data behind it is exactly the gap called out, and
it's a real, buildable gap: unlike outcome data or customer proof, this
doesn't need real customers, only real requests this control plane
already makes to Groq.

**What's captured, per real request, not sampled**: `groq.go`'s
`Complete()` now parses and returns the real `usage` field Groq's
OpenAI-compatible API already sends back (prompt/completion tokens) —
previously received and silently discarded. `chat_api.go`'s
`runChatLoop` now returns a real `chatLoopResult` accounting for the
whole turn: how many completion rounds it took, how many tool calls it
made and how many of those errored, summed real token usage across every
round, and whether it had to give up at `maxChatToolRounds` instead of
converging. `handleChat` and `handleNarrateAnomaly` both record one real
`AIEvalEvent` per request (best-effort, same non-blocking pattern as
`RecordRecommendationsShown` — a telemetry write failure must never take
down the actual feature it's observing).

`AIEvalStats(tenantID)` aggregates real events per feature: success
rate, average latency, tool-call success rate (a real pointer, not a
fabricated 0, when a feature has made zero real tool calls to average),
and total token cost. `GET /api/v1/ai-eval/stats` exposes it; a new
panel on the Assistant page (natural home) renders it, with an explicit
honest empty state ("No AI requests recorded yet") rather than a
misleading 0%.

**Verified against the real, live Groq API, not just a fake test
server**: sent a real chat message to the live deployment and confirmed
`GET /api/v1/ai-eval/stats` recorded it accurately —
`{"total_requests":1,"success_count":1,"success_rate":1,"avg_latency_ms":2950,"total_tool_calls":1,"tool_success_rate":1,"total_prompt_tokens":3752,"total_completion_tokens":114}` —
real token counts from the actual Groq response, not estimated.
Screenshotted rendering correctly on the live Assistant page.

8 new Go tests across `aieval_test.go`/`aieval_api_test.go` plus
additions to `chat_test.go`/`anomaly_narrator_test.go` (real token-usage
parsing, real per-round telemetry accumulation, real event recording on
both success and failure), full suite green (189 tests). 1 new E2E test
asserting the honest empty state (this suite never sets `GROQ_API_KEY`,
so zero real events is the actual truth, not an untested path).

### 3.22 Real deterministic traffic-replay safety check (`traffic_replay.go`)

The next safe increment on §3.16's dry-run remediation preview, and a
direct answer to the real risk that section already named: the fix
engine's generated `NetworkPolicy` (`agent/cmd/kharcha/fixengine.go`)
always uses `podSelector: {}` — it applies to **every** real pod in the
namespace, not just the one workload that got flagged. So "this manifest
looks reasonable" was never the real question; "does any *other* real
workload in that namespace also depend on the destination this policy
would block" is. That's answerable directly from data this control plane
already has, with no kernel tracing, no symbolic execution, and no
cluster write access.

**How it works.** `parseGeneratedNetworkPolicy` parses a manifest this
same codebase generated (deliberately a minimal struct matching
`networkPolicyDenyDestination`'s real output, not the full Kubernetes API
surface) and extracts the real namespace plus the blocked IP(s), stripping
the `/32`/`/128` mask since `flow_aggregate` stores bare IPs.
`HistoricalFlowsToDestination` (new, `store.go`) sums real cost per real
source workload for that destination across `flow_aggregate UNION ALL
flow_aggregate_daily` — the same pattern `WorkloadCostGrowth` uses, so
§3.18's compaction can't silently hide old collateral traffic from a
safety check. `EvaluateTrafficReplay` (pure function) then splits that
real history into the flagged workload's own traffic vs. everything else
in the same namespace, and reports a real safety score.

**Three honest edge cases, each deliberate, each tested**: a workload in
a *different* namespace is never counted (this policy's `podSelector: {}`
genuinely doesn't touch it); a manifest that can't be parsed is treated as
**not confirmed safe** rather than silently skipped ("has a manifest" and
"we verified the manifest" are different claims); and finding *no*
historical traffic at all reports unsafe with an explicit note, never a
default-safe — reporting confidence this check never earned would be
exactly the failure mode this whole document exists to avoid.

**A real performance regression, caught live and reverted, not shipped.**
The first version ran the replay for every manifest-bearing decision and
added a covering index (`tenant_id, cluster_id, dst_workload_or_endpoint`)
to make the lookup fast. Both were wrong, and only real measurement showed
it:
- The index genuinely worked as intended in isolation — `EXPLAIN QUERY
  PLAN` against a real 4.1M-row copy of the live database confirmed it was
  being used, cutting a real lookup from **4.24s to 1.0s**.
- But deployed to the live cluster it was a net disaster: `/api/v1/findings`
  — a read path this feature never touches — regressed from **3.9s to
  36–45s**, the pod pegged **above a full CPU core** (1029m, up from 4m),
  and the WAL grew to 162MB and stopped checkpointing. Indexing a TEXT
  column on a table under continuous real agent ingest cost far more in
  write amplification than it saved on a rare read, overwhelming SQLite's
  single writer and queueing every reader behind it.
- Reverted (`DROP INDEX IF EXISTS`, so any database that ran the
  intermediate build recovers on upgrade) and re-measured live: findings
  back to **3.74s**, CPU back to **4m**.
- The real fix was scoping, not indexing: `SimulateTrafficReplay` now only
  runs the lookup for decisions that already cleared every *other* policy
  bar. It's a gate on auto-applying, so a fix that isn't a candidate
  anyway never needed the check. Net result on the live cluster:
  `/api/v1/remediation/preview` is **3.70s — faster than the 11.4s it
  measured before this feature existed**, because zero live fixes
  currently qualify and so zero replay queries run.

A related real risk found in the same pass: the Helm chart's readiness/
liveness probes are `tcpSocket`, and the server only starts listening
*after* `OpenStore` finishes schema setup — a one-time index build
measured at ~26s on a real 4.1M-row database would have blown past the
3-failure default and could have caused a restart loop on a first boot
after upgrade. Both probes now carry explicit headroom
(`failureThreshold: 30`), which is correct regardless of this specific
index being reverted, since any future migration on a large real database
has the same shape.

**Verified live, honestly.** Against the real production cluster: 2 real
findings currently carry a generated manifest (both
`kube-system/traefik-...`), and the real replay confirmed both genuinely
safe — no other real workload in `kube-system` talks to those
destinations, verified by direct query, not assumed. The live cluster has
**no** real collateral case right now (checked directly: zero destinations
with more than one real namespaced workload), so the UI honestly shows no
collateral warning rather than a manufactured one. The unsafe path was
verified for real instead against a real running server with real
ingested data — screenshotted showing "Would apply (1)" with *No
collateral traffic found* and "Would skip (1)" with *Collateral traffic
found*, naming the real affected workload (`checkout/reporting-xyz`) and
its real ₹5 of traffic — plus a committed E2E test that drives the same
path end-to-end.

**A real UI bug caught by actually looking at the screenshot**, not by any
test: the new explanatory copy rendered `podSelector: ` with the braces
missing, because `{}` in JSX is an empty expression, not literal text.
Fixed (`{"podSelector: {}"}`) and re-screenshotted.

12 new Go tests (`traffic_replay_test.go` + additions to
`remediation_test.go`, `remediation_api_test.go`, `store_test.go`),
full suite green at **209** (up from 189), `go test -race` clean. 1 new
E2E test plus a real fixture fix — the committed E2E fixture's
`fix_manifest` had been a truncated stand-in with no
`spec:`/`egress:`/`ipBlock:` section at all, i.e. not the shape
`fixengine.go` actually emits; it's now the real complete manifest, so
the suite tests the manifest the product genuinely generates. 28/28 E2E
green (up from 27), with the other affected assertions updated to the
real new fixture counts rather than weakened. One new direct dependency:
`gopkg.in/yaml.v3` (the first non-SQLite/crypto dependency this module
has taken on, for real YAML parsing rather than hand-rolled string
matching).

### 3.21 Real proactive anomaly detection (`anomaly_watch.go`/`anomaly_watch_api.go`)

Direct response to another fair external criticism: anomaly detection
(`detectAnomalies`, §3.1) already existed but was 100% pull — a real
anomaly could sit undetected-by-anyone until an operator happened to
open the Anomalies page or ask the chat assistant. "AI should notice,
not just answer" is a real, buildable gap here, using data that already
exists, not a new capability that needs new infrastructure.

**How it works**: `WatchTenantForNewAnomalies(tenantID)` runs the exact
same existing `detectAnomalies` against every real cluster a tenant has,
and records a real new row in `anomaly_alerts` for any anomaly not
already captured for that exact snapshot. The real dedup key is
`(tenant_id, cluster_id, snapshot_reported_at)` — `Anomaly` gained a new
`SnapshotReportedAt` field (the real timestamp of the "current" snapshot
the anomaly was computed from) specifically so a still-true anomaly
(nothing new has happened, just still above the same old snapshot)
never gets re-alerted on every tick; only a genuinely fresh snapshot
crossing the threshold counts as new. `StartAnomalyWatchLoop` runs this
for every real tenant (`AllTenantIDs()`, new on `tenant.go`) on a
recurring tick — 5 minutes by default, tunable via
`CHIDRIXX_ANOMALY_WATCH_INTERVAL` (used to tighten this to 2s for the
E2E suite, so a real test can observe a real tick without waiting 5 real
minutes).

`GET /api/v1/anomalies/alerts` lists real unacknowledged alerts; `POST
.../acknowledge` dismisses one (idempotent — re-acknowledging is a
no-op, not an error, same pattern as `MarkRecommendationApplied`). A new
`AnomalyAlertBell.tsx` in the Topbar (visible on every page, not just
Anomalies) polls this every 15s and shows a real unread-count badge,
with a dropdown listing each real alert and a dismiss button.

**Verified live against real production data**: deployed with the
5-minute default; the pod's own log line confirms it's running
("proactive anomaly watch enabled: checking every 5m0s..."); `GET
/api/v1/anomalies/alerts` honestly returns `[]` on the live cluster right
now (no real cost anomaly currently exists there), screenshotted showing
"No new anomalies since the last check" rather than a fabricated alert.
End-to-end correctness (a real anomaly actually gets detected and
surfaced) was verified via a dedicated real E2E test rather than left
to unit tests alone, ingesting two real snapshots with a genuine 10x
jump on an isolated cluster ID and confirming the bell surfaces it
within one real background tick.

14 new Go tests across `anomaly_watch_test.go`/`anomaly_watch_api_test.go`
(idempotency across repeated ticks, a genuinely new alert after a fresh
snapshot, below-threshold no-op, acknowledge idempotency, tenant
isolation, `AllTenantIDs`), full suite green. 2 new E2E tests (real
detection end-to-end, and that an acknowledged alert stays gone across a
later real tick) — 27/27 E2E, up from 24/24. One real, deliberate
cross-file dependency, same discipline as this suite already uses for
shared global state: the Cost Graph edge-count test now expects 3 real
edges instead of 2, since the anomaly test's own fixture is a genuine
new real edge, not a stale count left unfixed.

---

## 4. Frontend — component inventory

39 `.tsx` files + 10 `.ts` files (excluding `.d.ts`), 5,909 lines in
`src/` — re-counted directly against the tree, not carried forward from
an earlier session's number. Separately, `e2e/` holds 11 more `.ts`
files (the committed Playwright suite, §3.10 — a distinct concern from
the app's own source, not counted here). React 18 + Vite 5 +
TypeScript + Tailwind + Recharts + Framer Motion + GSAP + `cmdk` (new,
§4.5). No component library — every visual element (donut chart, trend
chart, cost graph, force layout, command palette) is hand-built or
directly styled, matching the project's stated preference for owning
its own rendering rather than pulling in a pre-styled UI kit. Every
sidebar page beyond Overview is its own code-split chunk via
`React.lazy()`/`Suspense` (§8's now-closed bundle-size item).

### 4.1 Pages (rail-routed, 16 total, verified against `CommandRail.tsx`)

Overview (default), **Assistant**, Insights, Explorer, Workloads, Cost
Graph, Teams, Costs & Usage, Budgets, Savings Advisor, Forecasting,
Anomalies, History, Reports, Automations, Settings — same 16 real
destinations as before §4.5's shell redesign, now grouped into four
constellations (Observe/Understand/Act/Organize) instead of one flat
list; no page's own route id or internal content changed.

### 4.2 Components added across this document's sessions (cumulative, file-verified)

| File | Role |
|---|---|
| `AssistantPage.tsx` | The real Groq-backed chat assistant UI (§3.11) — message list, suggestion chips, calls `POST /api/v1/chat`; now also renders a real AI-evaluation panel (§3.20) fetching `GET /api/v1/ai-eval/stats` |
| `DeepForecastCard.tsx` | The real backtested-model forecast UI (§3.14) — per-cluster selector, calls `GET /api/v1/forecast` |
| `CostGraphPage.tsx` + `graphLayout.ts` | Node-link topology; dependency-free force-relaxation layout, builds nodes/edges client-side from `/api/v1/findings` |
| `HistoryPage.tsx` | Fetches `/api/v1/workload-growth`, renders ranked list + per-workload sparkline + correlated-event note |
| `TeamsPage.tsx` | Fetches/mutates `/api/v1/teams` (spend-by-team + namespace-ownership CRUD) **and** `/api/v1/invites` (§3.9's `MembersCard` — admin-only invite form + pending-invite table) |
| `PredictiveDriverCard.tsx` | Fetches `/api/v1/workload-growth`, reuses `holtForecast()` from `forecast.ts` to determine trend direction |
| `LoginPage.tsx` | Real Supabase `signUp`/`signInWithPassword` (§3.9), sign-in/sign-up toggle, honest "check your email" pending state; a third real "Sign in with username" mode (a real bug fix — see §3.8 item 8) posts straight to the cookie-session `POST /api/v1/auth/login` for self-hosted/CLI accounts, which previously had no browser-visible way to authenticate at all |
| `apiFetch.ts` | Single chokepoint attaching a Supabase bearer token to every authenticated frontend call |
| `supabaseClient.ts` | Real `createClient(url, publishableKey)`, fails loudly if env vars are missing rather than silently degrading |
| `session.ts` | `SessionContext`/`Session` type shared across the app |
| `PlacementSimulatorCard.tsx` | The real offline placement-optimization UI (§3.17) — zone-count selector, calls `GET /api/v1/placement/preview`, rendered on Cost Graph |
| `AnomalyAlertBell.tsx` | The real proactive-detection UI (§3.21) — a HUD bell (every page, not just Anomalies) polling `GET /api/v1/anomalies/alerts` every 15s, unread-count badge, dismiss via `POST .../acknowledge`; restyled in §4.5 (flat ink badge + an expanding-ring pulse instead of a colored ping-glow) |
| `CommandRail.tsx` **(new)** | Replaces the old flat `Sidebar.tsx` (§4.5) — the same 16 real pages, grouped into four constellations, default-expanded (240px, icon+label) with a real collapse toggle down to a 72px icon-only rail |
| `TopHUD.tsx` **(new)** | Replaces the old `Topbar.tsx` (§4.5) — org mark, current-section breadcrumb, the `⌘K` search pill, the live-sync pulse, date/filter/download icon buttons, `AnomalyAlertBell`, and an account menu (role + real log out) |
| `CommandPalette.tsx` **(new)** | A real `⌘K`/`Ctrl K` command palette (§4.5, `cmdk`-based) — fuzzy-jump to any of the 16 real sections |
| `ChidrixxMark.tsx` **(new)** | The real brand mark (§4.6) — a 3-facet shaded isometric cube, zero blur/glow, reused at every size from the 24px Top HUD mark to the 96px landing-page hero |

### 4.3 Frontend build output (measured, current — rebuilt as part of this update)

```
dist/assets/index-*.js               1,169.82 kB  (345.56 kB gzip)
dist/assets/index-*.css                 22.6 kB   (5.7 kB gzip)
dist/assets/AutomationsPage-*.js          9.02 kB  (2.51 kB gzip)
dist/assets/CostGraphPage-*.js            9.16 kB  (3.29 kB gzip)
dist/assets/FeaturePages-*.js             8.58 kB  (3.08 kB gzip)
dist/assets/AssistantPage-*.js            5.94 kB  (2.24 kB gzip)
dist/assets/TeamsPage-*.js                7.83 kB  (2.09 kB gzip)
+ 6 more page chunks, 1.5-4kB each
```

Code-split (§8, closed): only Overview + the new shell (`CommandRail`/
`TopHUD`/`CommandPalette`) ship in the main bundle's *page* code — the
main chunk grew slightly (`cmdk`, §4.5) but is still dominated by vendor
libraries (React, Framer Motion, GSAP, Recharts, Supabase client), not
app page code; vendor-chunk splitting would be the next lever if this
needs to go further (not attempted, wasn't asked).

### 4.4 Frontend dependencies (`package.json`, exact)

Runtime: `@fontsource-variable/geist`, `@supabase/supabase-js`, `cmdk`
**(new, §4.5)**, `framer-motion`, `gsap`, `motion`, `react` +
`react-dom` (18.x), `recharts`. Dev: `@playwright/test` (§3.10's E2E
suite) plus the standard Vite+React+TS+Tailwind toolchain. Vendored (not
in `package.json`, committed as source): `DecryptedText.jsx`,
`RotatingText.jsx` + `.css`, `VariableProximity.jsx` + `.css` —
react-bits components pulled in via the real `shadcn` CLI, not
hand-copied.

### 4.5 Real UI/UX shell redesign — Phase 1 of `web/DESIGN_VISION.md`

A full creative brief (`controlplane/web/DESIGN_VISION.md`) was written
first, then deliberately scoped down before building: the brief's
"Mission Control" north star was kept, but its original dark, glowing
night-control-room visual direction was replaced, on direct instruction,
with a light, near-monochrome, zero-glow direction — reframed in the
brief itself around *2001: A Space Odyssey*'s white spacecraft interiors
and "the HAL principle": chrome (backgrounds, buttons, borders) is
monochrome; color is reserved entirely for real computed data, never
decoration. This section covers what was actually shipped now (Phase 1
of that brief's 4-phase roadmap): a real shell redesign, not the full
living-topology canvas (Phase 3), which remains future work.

**Real color-token change, chrome only, data colors deliberately
untouched**: `index.css`'s `--page`/`--surface`/`--ink`/`--border`/
`--card-shadow`/`--accent` moved to a warm off-white/near-black system
(`--accent` is now literally `--ink` — a primary button is ink-on-paper,
not a separate brand color). A first attempt also desaturated the 8
categorical data colors (`--series-*`/`--status-*`) to match — this was
**caught and reverted** by actually running the dataviz skill's own
`validate_palette.js` against the muted values rather than eyeballing
them: the desaturated set failed the chroma-floor, CVD-separation, and
normal-vision-floor checks. Those 8 colors are the project's own
previously-validated categorical palette (`palette.ts`, unchanged since
originally adopted) and represent real data (path class, confidence,
status) — the "HAL principle" itself says color is reserved for exactly
that case, so muting them further would have traded real accessibility
for a look. Re-verified: the reverted values pass validation; only
chrome changed.

**Shell rebuilt, same 16 real pages/routes underneath**: `Sidebar.tsx`
and `Topbar.tsx` are deleted, replaced by `CommandRail.tsx` (grouped,
collapsible 72↔240px rail) and `TopHUD.tsx` (org mark, section
breadcrumb, `⌘K` search, live pulse, date/filter/download, the
restyled `AnomalyAlertBell`, an account menu). A new `CommandPalette.tsx`
(`cmdk`) adds real fuzzy-jump navigation across all 16 sections — a
genuine usability addition, not just visual. Every existing page's own
internal layout and logic is untouched; only the shell around them
changed, per the brief's own Phase-1 scoping.

**Glow removed everywhere it was found, not just where it was planned**:
beyond the brief's own scope, two pre-existing decorative blurred-glow
background orbs (`App.tsx`, `LandingPage.tsx`) were found and removed
during this pass — replaced with a plain, motion-only dot-grid reference
plane on the landing page (drift, not light). The anomaly bell's colored
ping-glow badge became a flat ink badge with an expanding-ring pulse
(motion carries "live," not light) — the same real backend data, just no
longer signaled with a glow.

**Verified real, not just visually**: full TypeScript build clean, full
Playwright suite green (27/27) after real, honest fixes — not
disabling tests, but updating them to match real UI changes: 5 tests hit
a real new ambiguity (the HUD's breadcrumb duplicates rail text) fixed
with `.first()`; 2 tests (log out, viewer-role visibility) now correctly
open the new account menu first, since those real controls actually
moved there. Screenshotted directly (Overview, Automations, command
palette, bell dropdown, collapsed rail, landing page) against a real
locally-run instance with real ingested data before calling this done,
consistent with this project's UI-verification discipline throughout.

Explicitly deferred to a later phase (per the brief's own roadmap, §10
of `DESIGN_VISION.md`): the living topology canvas itself (Phase 3),
page renaming to the Mission Control vocabulary ("The Grid," "Ground
Control" as a full overlay merging the Assistant page), and the
motion/sound polish pass (Phase 2). This pass is the shell and color
system only.

---

### 4.6 Real branded landing page + a real GSAP scroll-animation bug fix

Direct follow-up feedback on §4.5: the new landing page was "too simple"
(wanted a real marketing page — feature sections, a real demo slot —
like a proper product site) and the visual language needed one more
correction: no separate accent color anywhere, grey used as a real,
visible shade (not just a whisper of off-white), and headline text with
a genuine grey-gradient 3D/embossed look. A brand mark (a 3D chrome "C")
was shared as a style reference first; since pasted chat images aren't
saved to a file this build can reference, it was initially recreated in
CSS/SVG (a real isometric cube built from three flat shaded facets).
Once a real file path for the actual asset was provided
(`~/Downloads/...png`), it replaced the CSS version outright, per direct
instruction — the real asset, not a recreation of it.

**`ChidrixxMark.tsx`**: renders the real provided logo image
(`public/chidrixx-mark.png`) inside a sized bounding box via
`object-contain`, so every call site controls its own size without
distorting the source art's real aspect ratio. The source PNG (1536×1024,
2.2MB, real alpha transparency confirmed directly by inspecting pixel
alpha values, not assumed) was downscaled to 600px wide (~90KB) — same
visual result at every real display size used, a fraction of the
original transfer weight. Used large (h-24/h-32) on the landing page
hero, small (24–28px) in the Top HUD (§4.5) and the landing nav/footer —
the same real mark everywhere, not a different logo pre- and
post-sign-in.

**A new `.text-3d-grey` CSS utility**: a grey-gradient fill
(`background-clip: text`) plus a stack of crisp, zero-blur, same-direction
offset text-shadows underneath it, producing a real embossed/extruded
look built entirely from flat shade layers — applied to the landing
page's "chidrixx" wordmark and the "How it works" step numerals.

**`LandingPage.tsx` rebuilt as a real marketing page**: a sticky nav
(mark, anchor links, log in / get started), a hero with the 3D mark +
gradient heading + a real screenshot of the live dashboard (captured
directly from the running app, not a mockup) framed in a tilted
browser-chrome card, a 6-card features grid covering every real shipped
capability (path classification, fix engine, multi-cluster, the grounded
AI assistant, proactive anomaly watch, the placement simulator — nothing
listed that isn't actually running), a 3-step "how it works," a real
`<video>`-based demo section (points at `/demo.mp4`, falls back to an
honest "Demo video coming soon" state via `onError` when the file isn't
there yet — never fabricates a video that doesn't exist), and a footer
with a real GitHub link. A new `--surface-grey` token gives alternating
sections a real, visible grey band instead of a near-invisible off-white.

**A real bug found and fixed along the way, not just a cosmetic
pass**: screenshotting the dashboard for the hero image surfaced
several dashboard section titles (`SectionTitle`/`ScrollFloat.jsx`, a
vendored GSAP `ScrollTrigger`-scrubbed character reveal) rendering
permanently garbled — stuck mid-reveal, not a transient animation frame.
Root cause, confirmed directly (forced a real `scrollTop` change via
`page.evaluate` and watched it instantly resolve correctly): a scrubbed
`ScrollTrigger` computes its start/end pixel positions once, and real
async layout settling shortly after mount (a chart library measuring
itself, fonts swapping in) leaves that cached position stale, freezing
the reveal at whatever fractional progress the stale math produces,
until a real scroll event forces GSAP to recompute. §4.5's shell
restructuring (moving the scroll container, adding a fixed-height Top
HUD sibling above it) changed layout timing enough to make this
reliably reproducible, though the underlying bug was already latent in
the vendored component. Fixed in `ScrollFloat.jsx`: an `rAF` pass, three
staged `setTimeout` passes (200/600/1200ms), a `window.load` listener,
and a real `ResizeObserver` on `document.body` (open for a bounded 2s
after mount) all call `ScrollTrigger.refresh()` — reacting to whatever
actually resettles layout instead of guessing one fixed delay. Also
added proper teardown (the original had no cleanup at all — a real leak,
fixed alongside). Verified, not assumed: repeated real screenshots at
multiple wait times (300ms–4.5s) against both a local instance and the
live cluster's real production data, before and after, confirmed the fix
holds rather than just happening to look right once.

Full TypeScript build clean, full Go suite green (unaffected — this pass
touched only the frontend), full Playwright suite green (27/27), and
redeployed to the live cluster with a direct visual re-verification —
the same discipline as every other feature in this document.

---

## 5. Deployment & Distribution

| Item | Status |
|---|---|
| Helm charts (`deploy/helm/kharcha`, `deploy/helm/controlplane`) | **Real.** Lint-clean, template-clean, and actually `helm install`/`upgrade`d against a live cluster (not just linted). |
| OCI chart repo on GHCR | **Real.** `oci://ghcr.io/ananttyagi07/charts/{kharcha,chidrixx-controlplane}` — packaged, pushed, pulled back down, and `helm template`d from the round-tripped artifact to confirm it's not corrupted. |
| GHCR image + chart visibility | **Done.** All four packages (`chidrixx-agent`, `chidrixx-controlplane`, `charts/kharcha`, `charts/chidrixx-controlplane`) are now **public**, flipped via the GitHub web UI (GitHub's REST API 404s on the documented visibility-change endpoint even with a working `GET` and the right token scope — web-UI-only, confirmed by direct testing). Re-verified with zero credentials involved: `docker logout ghcr.io` then a clean `docker pull` of both images succeeded; both OCI Helm charts' `tags/list` returned 200 using a freshly-minted anonymous GHCR bearer token. |
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

### 5.2 Live cluster state (k3d, as of this document — re-verified 2026-08-05 post-traffic-replay)

**Re-measured 2026-08-05 while shipping §3.22** (the numbers further down
this section are from the 2026-08-03 pass and are kept for the record;
these supersede them):

- `flow_aggregate` has grown to **4,125,883 real rows** (up from
  2,391,057 two days earlier — continuous real ingestion, integrity-
  checked `ok` on a freshly pulled copy). The live database file is now
  **~1.19GB** (up from ~590MB), plus a real WAL.
- **55 rows** in `recommendation_outcomes` (up from 26), still **0
  applied and 0 measured** — the real, unchanged gap §9 names: this needs
  real operators, not more code.
- Real request latency, measured directly against the live deployment:
  `/api/v1/findings` **3.74s**, `/api/v1/remediation/preview` **3.70s**
  (down from 11.4s before §3.22, because that feature's scoping change
  means zero replay queries run when no fix qualifies). Pod at **4m CPU**
  idle.
- **§3.22 verified honestly on real data**: exactly 2 real findings
  currently carry a generated manifest (both `kube-system/traefik-...`),
  and the real traffic replay confirms both genuinely safe. There is
  currently **no** real collateral case anywhere in the live data
  (checked directly: zero destinations with more than one real
  namespaced workload), so the UI honestly shows no collateral warning —
  the unsafe path was proven against a real running server with real
  ingested data and in the committed E2E suite instead, not faked here.
- Two extra admin users (`replay-verify`, `replay-verify2`) were
  provisioned on tenant 1 via the real `create-user` CLI purely to verify
  §3.22 through the real browser/API. Noted rather than left silent;
  there is no `delete-user` subcommand today, so they're still present.

### 5.2.1 Earlier snapshot (2026-08-03, post-AI-eval/proactive-anomaly-watch)

- Namespace `chidrixx`: `chidrixx-controlplane` Deployment (Helm
  revision **8**), image `chidrixx-controlplane:dev` (rebuilt and
  `k3d image import`ed several more times today for the login-form fix,
  §3.20, and §3.21 — no registry push needed for local iteration),
  PVC-backed SQLite, `GROQ_API_KEY` wired via the
  `chidrixx-controlplane-groq-key` secret (§3.11). Pod logs confirm all
  three background loops are live: WAL mode, the compactor, and now the
  proactive anomaly watch (`checking every 5m0s for new real anomalies
  per tenant`).
- Namespace `kharcha`: `kharcha-kharcha` DaemonSet (Helm revision 5,
  unchanged today), image `chidrixx-agent:dev`, real ingest token issued
  via `create-token` and stored in the `kharcha-controlplane-token`
  secret.
- **Real production data, directly queried from a pulled+integrity-
  checked copy of the live database, not estimated**: `flow_aggregate`
  has **2,391,057 real rows** (up from 2,229,443 earlier this session —
  continuous real ingestion, not paused). `chidrixx-lab` and
  `chidrixx-lab-2` together span ~55h40m of real continuous ingestion.
  `flow_aggregate_daily` (§3.18) is correctly **0 rows** on the live
  system — the default 30-day retention window is far longer than this
  cluster's real ~2.3-day history, so the live background compactor
  correctly compacts nothing yet (confirmed via the pod's own log line,
  not assumed); the compaction logic itself was verified separately
  against a real pulled copy with a tightened window (§3.18: 2,205,218
  real rows → 1,998 real rollups, cost-conserving). 1 real tenant, 2
  real admin users. **26 rows** in `recommendation_outcomes` (up from
  24), of which **0 are applied and 0 are measured** — the real,
  unfabricated current state of §3.19's outcome-dataset-health metric,
  because this lab environment has no real day-to-day operators using
  it yet. `journal_mode` confirmed `wal` on the live file. Real
  `spend_by_team` still shows `Unassigned` (no namespace mappings
  configured on the live tenant yet) — a real gap in configuration, not
  a bug.
- The live database file itself is ~590MB (main file) + a few MB of
  WAL — consistent with real, continuous ingestion over multiple days,
  not a synthetic fixture. This will stop growing unboundedly once real
  data starts crossing the 30-day retention window (§3.18).
- **§3.20 verified against the real Groq API, not a fixture**: sent one
  real chat message through the live deployment; `GET
  /api/v1/ai-eval/stats` recorded it accurately (100% success, 2,950ms
  latency, 1 real tool call succeeded, 3,752 prompt + 114 completion
  real tokens). **§3.21 verified honestly**: `GET
  /api/v1/anomalies/alerts` correctly returns `[]` on the live
  cluster — no real cost anomaly currently exists there, and the UI
  shows that truthfully ("No new anomalies since the last check")
  rather than a fabricated alert.

---

## 6. CI (`.github/workflows/ci.yml`) — verified against the actual file, not assumed

Seven jobs total now — the original three, `agent/`-only, plus four new
ones giving `controlplane/` equal coverage:

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
4. `build-and-test-controlplane` (`working-directory:
   controlplane`): the same `gofmt`/`go vet`/`go build`/`go test`
   sequence as job 1, covering all 130 control-plane test functions
   (§3.6).
5. `docker-build-controlplane`: builds `chidrixx-controlplane:ci`
   from `controlplane/Dockerfile`.
6. `helm-lint-controlplane`: `helm lint` + `helm template`
   against `deploy/helm/controlplane`.
7. `e2e-controlplane`: installs Node 20 + runs `npm ci` in
   `controlplane/web`, then `npm run test:e2e` — the real Playwright
   suite from §3.10, on every push now instead of only when someone
   remembers to run it locally. Relies on the Google Chrome
   GitHub-hosted `ubuntu-latest` runners ship preinstalled (same
   `launchOptions.executablePath` approach used for local dev — no
   `playwright install --with-deps` needed). Uploads the Playwright
   report as a build artifact on failure.

All four new jobs verified locally before being committed: `gofmt -l`,
`go vet`, `go build`, `go test` all clean; `docker build controlplane`
succeeds end-to-end; the workflow YAML parses. `helm lint`/`template`
weren't runnable locally (no `helm` binary in this dev environment) but
use the identical pattern as the pre-existing, working `kharcha` job.

This closes what was punch-list item §8.5 and the "zero CI coverage for
controlplane" gap this section used to describe — a change to
`controlplane/` today now gets the same automatic build/test/Docker/Helm/
E2E coverage `agent/` always had.

---

## 7. What's explicitly NOT built (and why)

These are honest placeholders — visually present where relevant, clearly
labeled, never filled with invented numbers:

- **A neural/ARIMA/Prophet-grade time-series forecast** — §3.14's
  backtested Holt-vs-damped-Holt model is a real, measured improvement
  over plain Holt (§3.7), but it's still classical exponential
  smoothing, not a neural sequence model, and it still can't be
  calendar-aligned (chidrixx's data is cumulative snapshots, not fixed
  time windows) — deliberately, since the real production data doesn't
  span enough calendar days to honestly fit a seasonal component yet
  (checked directly, not assumed — see §3.14/§3.15).
- **No automated release/versioning** — both charts are still `0.1.0`;
  there's no CI job that bumps versions or cuts releases automatically.
- **A covering index for `flow_aggregate`** — retention/compaction now
  exists (§3.18, closing the other half of this former gap), and as of
  §3.22 this is no longer just "not attempted": a real covering index on
  `(tenant_id, cluster_id, dst_workload_or_endpoint)` was actually built,
  deployed to the live cluster, measured, and **reverted**. It did make
  the targeted lookup ~4x faster, but the write amplification on a table
  under continuous real agent ingest regressed unrelated read paths from
  3.9s to 36-45s and pegged the pod above a full CPU core. The honest
  conclusion: indexing this table is not a free win at this write volume,
  and any future attempt needs the same live before/after measurement
  rather than the assumption that an index is automatically an
  improvement.
- **A ClickHouse (or similar) storage-engine swap** — deliberately not
  done; see §3.18 for the real reasoning (SQLite+WAL already handles
  this project's actual real scale fine; a full swap would be costly
  over-engineering for a problem retention/compaction solves directly).
  Revisit only if a real second engine's cost is actually justified by
  real multi-tenant scale this environment doesn't have yet.

---

## 8. What's actually left — punch list, with technical detail

| # | Item | Who | Effort | Technical detail |
|---|---|---|---|---|
| 1 | An actual second cloud deployed with the new GCP price book | You | Blocked | Plumbing is 100% real and tested (`pricebook/gcp.yaml`, `values-gcp.yaml`, `SpendByCloud()`, the donut) — verified by walking through the actual deploy steps live (Cloud Shell, `gcloud compute instances create`). Blocked on a real, non-technical wall: this GCP project has no billing account, and India requires a ~₹15,000 refundable deposit to activate one. Deliberately not pushed through — that's real money, not a 5-minute task, and it's not worth spending to prove plumbing that's already tested. Options if this gets revisited: pay the deposit, or retarget `values-gcp.yaml`'s pattern at a genuinely free-tier host (e.g. Oracle Cloud) with a new price book file. |
| 2 | Business/GTM (pricing, personas, launch) | You | Deprioritized | No pricing page, no persona docs, no launch plan — explicitly deprioritized per prior direction ("technical completeness came first"). |

**Done since the last version of this table**:
- Flipping the 4 GHCR packages to public (was item #1) — verified for real: fully logged-out `docker pull` of both container images, and anonymous GHCR-token `tags/list` calls against both OCI Helm charts, all succeeding with no credentials involved.
- Frontend bundle code-splitting (was item #4) — `React.lazy()` + `Suspense` now wraps all 11 non-Overview sidebar pages (`controlplane/web/src/App.tsx`); each is its own chunk (11 new files in `dist/assets/`, ~1.5–8KB each, `FeaturePages`'s four exports dedupe into one shared chunk since Rollup collapses same-module dynamic imports). The main bundle is still large (~1.1MB) because that size is dominated by vendor libraries (React, Framer Motion, GSAP, Recharts, Supabase client), not app page code — vendor-chunk splitting would be the next lever if this needs to go further, not attempted here since it wasn't what was asked. Re-running the E2E suite after this change surfaced and fixed a real pre-existing test race in `dashboard.spec.ts` (asserted on page text before the async dashboard-summary fetch resolved) — noted here since it was found doing this work, not a new gap.
- CI coverage for `controlplane/` (was item #5) — see §6, seven jobs now, `controlplane/` has the same build/test/Docker/Helm/E2E coverage `agent/` always had.
- **A deeper forecasting model** (was item #2/#3) — see §3.14. Scoped honestly against the real production data volume (checked first, not assumed): real rolling-origin backtesting comparing plain Holt against damped-trend Holt, server-side, against a cluster's full retained history — not Holt-Winters/ARIMA/a neural model, which the real data doesn't support yet without inventing pattern-matching on noise. Live-verified against a pulled copy of the actual production database, including watching the model selection genuinely flip as more real data arrived.

---

## 9. Future vision — not started, direction only

Everything above is either shipped or a scoped, estimateable task. This
section is different: it's where the project is headed next, not
something in progress. Marking it clearly as vision, not a roadmap
commitment with dates, so it doesn't get conflated with the punch list
above.

- **Revised plan (superseding the original fine-tuning-first plan
  below): the grounded chat assistant already shipped** (§3.12) — a
  Groq-hosted off-the-shelf model (Llama 3.3 70B) doing real tool-calling
  against real tenant-scoped data, not a custom fine-tuned model. This
  was a deliberate strategic pivot made explicitly in conversation: a
  small model fine-tuned on eBPF/networking *documentation* has nowhere
  near the capacity to reason well and would still need real GPU
  infrastructure this environment doesn't have; a frontier model doing
  retrieval/tool-calling gets most of the value today, cheaply, with no
  training step at all.
- **What's actually worth fine-tuning later, and why it isn't yet**: not
  a model trained on eBPF/networking knowledge, but a small model or
  ranking/calibration model trained on *this product's own real outcome
  data* — did a given recommendation get applied, did it actually save
  the predicted amount — which is a genuinely proprietary dataset no
  competitor can replicate (needs chidrixx's own byte-level telemetry).
  §3.11's `recommendation_outcomes` table is that data layer, shipped
  this session specifically to start capturing it. §3.19 added real,
  honest visibility into how mature it actually is (`GET
  /api/v1/outcomes/stats`, an "Outcome dataset health" card on
  Automations) — but visibility isn't the same as progress: the real gap
  is unchanged by that. Directly confirmed against the live cluster
  while building §3.19: **26 real rows shown, 0 applied, 0 measured**.
  This needs real operators actually applying real recommendations over
  real time; no feature can shortcut that, only make it trackable once
  it starts happening. Revisit once that data actually exists.
- **A deeper domain-specialized model remains a real, undated
  possibility** if Groq's off-the-shelf model proves insufficient for
  more advanced reasoning (e.g. multi-hop causal simulation over the
  cost-topology graph in §3.7 — "what would happen to cost if I moved
  this workload" — a materially harder task than the current tool-calling
  Q&A). Not started, not scoped, genuinely future vision.

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
- **A real second-cloud agent deployment is blocked on a real money
  decision** (India's GCP billing deposit requirement), not a technical
  gap — see §8 item 1. Deliberately not pushed through for a demo.
- **The chat assistant has no proprietary *outcome* data to fine-tune on
  yet** — §3.11's `recommendation_outcomes` table is live and real (26
  real rows from real dashboard loads on the live cluster, verified by
  direct query against a pulled copy of the production database), but
  0 of those 26 have `applied_at` set — no operator has clicked "Mark as
  applied" on the live cluster yet, so the "did this fix actually work"
  signal the whole feature exists to eventually train on is still
  empty. The *shown* tracking works; the *outcome* dataset doesn't
  exist yet. §3.19 added a real `GET /api/v1/outcomes/stats` endpoint
  and dashboard card making this exact gap honestly visible (rather
  than only discoverable by pulling a database copy) — the gap itself
  is unchanged by that. See §9.

---

## Bottom line

The agent's core claims (byte-accurate attribution, real topology
classification, real fix generation) are measured, not asserted. The
control plane and dashboard are a genuinely working multi-tenant SaaS-
capable product: real Supabase-backed public signup alongside the
original self-hosted CLI path, real self-service team invites, real
root-cause correlation, real $ optimization recommendations, a real
historical trend-change view, a real cost topology graph, a real
predictive driver, real closed-loop recommendation-outcome tracking,
a real Groq-backed grounded chat assistant, a real anomaly root-cause
narrator, a real deeper forecasting model, a real dry-run closed-loop
remediation preview with a real traffic-replay safety check behind it,
and a real offline placement simulator — not a static mockup or a
single-shared-secret toy.

`controlplane/` now has the same CI coverage `agent/` always had (§6,
seven jobs total: build/test/Docker/Helm for both modules plus a real
E2E job) — the "no CI coverage" risk this section used to name is
closed. GHCR image/chart visibility is done and independently verified
with zero stored credentials. Frontend bundle code-splitting is done.
The deeper forecasting model (§3.14) is genuinely scoped to what the
real production data volume supports — checked first by pulling the
actual live database, not assumed — real backtested model selection
between plain and damped Holt, not an invented seasonal model the data
can't honestly back yet. A real live-production performance
investigation (§3.15) cut the dashboard's slowest real request from
20-40+ seconds to ~5.8 seconds, root-caused through direct live
measurement (`kubectl top`, a pulled+integrity-checked database copy,
`EXPLAIN QUERY PLAN`) rather than guessing, catching a genuine latent
concurrency bug along the way. The remediation preview (§3.16) is
deliberately the safe, evidence-gathering first step of a bigger idea
(closed-loop automatic remediation) — real policy, real transparency,
zero cluster write access, not a shortcut to the harder, riskier version
of the feature. The placement simulator (§3.17) is the equally safe
first step of the other big idea (a cost-aware placement algorithm) —
a real graph-partitioning algorithm with two real bugs caught and fixed
by actually testing and screenshotting it, not a black box trusted on
faith. Real automated retention/compaction (§3.18) closes
`flow_aggregate`'s unbounded-growth risk — chosen over a ClickHouse swap
after actually checking this project's real scale first, not by
default, and verified against a real 2.37M-row copy of the live
database (1,103x real row reduction on the compacted slice, cost-
conserving) before ever touching the live system. Real outcome-dataset-
health visibility (§3.19) makes the recommendation-outcomes dataset's
actual maturity trackable — honestly still 26 shown/0 applied/0 measured
on the live cluster, because that gap needs real operators over real
time, not more code. A real login-form bug (§3.8 item 8) — self-hosted/
CLI-provisioned accounts had no way to sign in through the browser at
all, only the Supabase email form, caught by actually trying to log in
on the live site rather than by any test — is now fixed, with two new
E2E tests driving the real rendered form so it can't silently regress.
Real AI evaluation telemetry (§3.20) closes a fair external gap — two
real AI features existed with zero visibility into whether either
actually worked; success rate, tool-call reliability, latency, and real
token cost are now measured per request and verified against the actual
live Groq API, not a fixture. Real proactive anomaly detection (§3.21)
closes another — anomaly detection was 100% pull before; a background
loop now checks every real tenant on a recurring tick and surfaces new
anomalies through a Topbar bell on every page, not just when an operator
happens to ask, verified end-to-end with a real ingested anomaly in the
E2E suite and honestly showing nothing on the live cluster where none
currently exists. The real traffic-replay safety check (§3.22) is the
next safe increment on the remediation preview, answering the exact risk
§3.16 itself named: the generated policy uses `podSelector: {}`, so
before any fix can be called safe to auto-apply it is now replayed
against this tenant's own real recorded traffic to see whether another
real workload in that namespace also depends on the destination being
blocked. That pass is also this document's clearest case of measurement
beating intuition — the obvious optimization (a covering index) was
built, deployed, measured on the real live system, found to make
*unrelated* read paths ~10x slower through write amplification, and
reverted; the fix that actually shipped was scoping the check to fixes
that could genuinely qualify, which left the endpoint faster than it was
before the feature existed.

What's genuinely left, in priority order (§8): a real second-cloud agent
deployment is blocked on a real, non-technical wall (India's GCP billing
deposit requirement — a money decision, deliberately not pushed through
for a demo); GTM/business work is explicitly deprioritized per prior
direction. The four AI/modeling features (outcome tracking, the chat
assistant, the anomaly narrator, the deeper forecasting model) are
deliberately sequenced — the data layer first, then models grounded
against real data volume/history — with the honest data gap named
directly in §9's future-vision section: there's no proprietary outcome
dataset yet, only the schema (and now, §3.19's visibility into it)
capturing progress toward one as real usage actually happens.
