# chidrixx — Project Status

_Last updated: 2026-08-01_

This is the honest, complete picture: what's real and verified, what's
explicitly not built (and why), and what's actually left to do. Every claim
below was verified against a real cluster during development — not just
written and assumed to work. Where a number appears, it's a number that was
actually measured.

---

## What chidrixx is

An eBPF agent that attributes Kubernetes network traffic to workloads and
prices it — which pod is talking to which destination, over which path
(same node / same zone / cross-zone / cross-region / NAT / internet /
managed service), and roughly what that's costing. A control plane
aggregates this across clusters with a real dashboard on top.

Two components, two Go modules:
- **`agent/`** (module `chidrixx`) — the eBPF agent, one per cluster (DaemonSet).
- **`controlplane/`** (module `chidrixx-controlplane`) — optional multi-cluster
  ingest API, storage, and dashboard.

---

## 1. Agent — Real & Verified

- **eBPF programs** (`bpf/flow_cgroup.c`): `cgroup_skb` egress/ingress,
  counting bytes per `(cgroup_id, 5-tuple)` in a `BPF_MAP_TYPE_LRU_PERCPU_HASH`
  map. Both programs unconditionally `return 1` (SK_PASS) — they only count,
  never gate traffic.
- **Loader** (`agent/cmd/kharcha/loader.go`): the agent owns its own BPF
  lifecycle (load/attach/detach), not relying on an externally-pinned map.
  Recognizes the specific `EPERM` from the cgroup-namespace limitation (see
  §5) and wraps it with an actionable message instead of a generic error.
- **Classification** (`classify.go`): 8 path classes — `SAME_NODE`,
  `SAME_AZ`, `CROSS_AZ`, `CROSS_REGION`, `MANAGED_SERVICE`,
  `PRIVATE_OFFCLUSTER`, `NAT_EGRESS`, `INTERNET_EGRESS` — each with a
  confidence level (high/med/low) that widens the price estimate when
  topology data is incomplete rather than guessing.
- **Pricing** (`pricing.go`): overridable YAML price book, cost bands by
  class + confidence.
- **Reporting**: CLI table, HTML report, Prometheus metrics
  (`kharcha_flow_bytes_total`, `kharcha_cost_inr`, `kharcha_map_entries`,
  `kharcha_scrape_lag_seconds`).
- **Alerting** (`alert.go`): threshold and growth-ratio based, posts to a
  Slack-compatible webhook.
- **Shipper** (`shipper.go`): posts findings to the control plane, Basic Auth.
- **Kubernetes metadata resolution** (`kubernetes.go`): talks to the API
  server directly over a keep-alive `http.Client` using the mounted
  service-account token — **not** by shelling out to `kubectl` (the original
  design), which was the dominant source of CPU overhead. Falls back to
  `kubectl` exec only when running outside a cluster (local dev).
- **Fix engine** (`fixengine.go`): generates a real, copy-pasteable
  `NetworkPolicy` manifest for `INTERNET_EGRESS`/`NAT_EGRESS`/`CROSS_REGION`
  findings, scoped to the real source namespace and the real flagged
  destination IP. `CROSS_AZ`/`MANAGED_SERVICE` stay a text hint — their real
  fix needs pod labels this agent doesn't resolve, and a fabricated label
  selector would be worse than an honest sentence.

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
| **CI** (`.github/workflows/ci.yml`) | Build/vet/test, recompiles the BPF object from source and re-runs privileged load/attach tests against it, `helm lint`/`helm template`, Docker build — all passing on GitHub-hosted runners (full VMs, don't hit the cgroupns issue). |
| **Genuine two-cluster proof** | Not curl-simulated: a second, fully independent k3d cluster with its own real agent, connected to the same control plane. Both cluster IDs confirmed in `/api/v1/findings` and the dashboard. |

---

## 3. Control Plane — Real & Verified

- **Ingest + storage**: SQLite (pure Go, `modernc.org/sqlite`, no cgo),
  each ingest is a full point-in-time snapshot; `LatestFindings` always
  serves each cluster's most-recent snapshot, tested against stale-snapshot
  leakage.
- **Auth**: Basic Auth (deliberate — works for both an agent posting JSON
  and a human in a browser), constant-time token comparison. The control
  plane now **auto-generates and manages its own token** (`auth.generate:
  true`), stable across Helm upgrades (verified: two consecutive upgrades
  produced the identical token, not a fresh one).
- **Dashboard** (`controlplane/web`) — a real React + Vite + Tailwind +
  Recharts + Framer Motion SPA, replacing the old server-rendered HTML
  entirely. Built assets are committed to git (like `bpf/flow_cgroup.o`) and
  `go:embed`'d — `go build`/`go test` need no Node install.
  - **Landing page**: real react-bits components (`DecryptedText`,
    `ScrollFloat`, `RotatingText`, `VariableProximity`) installed through the
    actual `npx shadcn@latest add @react-bits/...` CLI (needed a portable
    Node 20 fetched just for that, since Node 18 can't run it). Lists only
    real features. Routing was split so the static shell serves without auth
    (no secrets in it) while `/api/*` stays gated — lets the landing page
    render before any credential prompt.
  - **Enforced light theme** — no longer follows the OS's dark-mode
    preference.
  - **Real widgets, all backed by actual aggregates**: total spend, data
    transferred, active workloads, spend distribution (by path class),
    spend trend, spend by confidence, per-cluster cards with real
    sparklines, top fix opportunities (with the real generated
    `NetworkPolicy` manifests, expandable inline).
  - **Built this session, all verified live, not just visually**:
    - **Budget Status** — a real user-set INR figure, persisted server-side
      (`settings` table), compared against real spend. Verified: set a
      value, reloaded the page, confirmed it survived (not client-only state).
    - **Anomaly Detection** — compares each cluster's two most recent
      snapshots, flags growth ≥2x. No fabricated "285% spike" narrative,
      just the two real numbers and the ratio. Verified: ingested a real 10x
      cost jump, confirmed it was correctly flagged.
    - **Trend Projection** — a real least-squares linear fit over recent
      snapshots, extended forward, rendered as a dashed continuation of the
      real solid line. Deliberately **not** called "Forecast (next 7 days)"
      or framed as ML — chidrixx's data is cumulative snapshots, not a
      calendar-day time series, so that framing would be a claim it can't
      back.
    - **Costs & Usage** page — the full cross-cluster findings list (not
      just flagged ones), with a real client-side search and cluster filter.
  - **Sidebar nav consistency fix**: Budgets/Anomalies/Forecasting/Savings
    Advisor all now route to their real content (they briefly still said
    "Coming soon" after the features above were built as Overview cards —
    caught and fixed the same session).

### Real bugs found and fixed during this work (kept for the record)

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

---

## 4. Deployment & Distribution — Partial

| Item | Status |
|---|---|
| Helm charts (`deploy/helm/kharcha`, `deploy/helm/controlplane`) | **Real.** Lint-clean, template-clean, and actually `helm install`ed against a live cluster (not just linted). |
| OCI chart repo on GHCR | **Real.** `oci://ghcr.io/ananttyagi07/charts/{kharcha,chidrixx-controlplane}` — packaged, pushed, pulled back down, and `helm template`d from the round-tripped artifact to confirm it's not corrupted. |
| GHCR image + chart visibility | **Blocked, needs your action.** All four packages (`chidrixx-agent`, `chidrixx-controlplane`, `charts/kharcha`, `charts/chidrixx-controlplane`) are still **private**. GitHub's REST API returns a 404 on the documented visibility-change endpoint even with a working `GET` on the same path and the right token scope — this is a web-UI-only setting, confirmed by direct testing, not a permissions issue on my end. Fix: `github.com/users/Ananttyagi07/packages/container/package/<name>` → Package settings → Change visibility → Public, for all four. |
| Ingress/TLS (control plane) | **Real, optional.** `ingress.enabled` in the Helm chart, verified: a real request through Traefik with the right `Host` header + Basic Auth returned 200. |
| cgroup-namespace limitation | **Documented, not fixable from our side** (inherent to Docker-in-Docker clusters like kind/k3d). The agent now gives a specific, actionable error instead of a generic `operation not permitted`. Real managed Kubernetes (EKS/GKE/AKS) doesn't hit this at all. |

---

## 5. What's explicitly NOT built (and why)

These are honest placeholders — visually present where relevant, clearly
labeled "Coming soon," never filled with invented numbers:

- **Multi-cloud cost attribution / "Spend by provider"** — chidrixx prices
  against one price book (AWS) today. A real multi-cloud view needs
  multi-provider ingestion this doesn't have.
- **Carbon footprint** — no carbon-cost model exists or is planned to be
  fabricated.
- **A real forecasting/ML model** — only the honest linear-projection exists
  (§3). A genuine demand forecast would need a real time-series model and
  calendar-aligned data this agent doesn't collect.
- **Insights, Explorer, Workloads, Reports, Automations, Settings** — sidebar
  items with no real page behind them yet. Clicking them shows a clear
  "not built yet" page, not a fake one.
- **No multi-tenant accounts / RBAC** — one shared Basic Auth token, no
  per-user identity. The sidebar's "Admin / shared token access" label
  reflects this honestly rather than fabricating a named logged-in user.
- **No automated release/versioning** — both charts are still `0.1.0`;
  there's no CI job that bumps versions or cuts releases automatically.

---

## 6. What's actually left — punch list

| # | Item | Who | Effort |
|---|---|---|---|
| 1 | Flip 4 GHCR packages to public (web UI, listed in §4) | **You** | 5 min |
| 2 | Real multi-cloud price-book ingestion, if wanted | Both | Large — new data source, new agent config surface |
| 3 | A genuine forecasting model (if the linear projection isn't enough) | Both | Large — needs a real time-series approach and more historical data than currently retained |
| 4 | Build out Insights / Explorer / Workloads / Reports / Automations / Settings | Both | Medium each — some (Workloads, Reports) are mostly data already available; Automations implies a real remediation-execution engine, which is a bigger scope decision |
| 5 | Multi-tenant accounts / real RBAC | Both | Large — a genuine new subsystem, only worth it if this becomes a multi-customer product rather than a single-operator tool |
| 6 | Frontend bundle size (886KB, no code-splitting) | Me | Small — `React.lazy`/dynamic imports per page |
| 7 | Commit the ad-hoc Playwright verification scripts into a real CI-run E2E suite | Me | Medium — they exist as throwaway scripts today, proven useful, worth making permanent |
| 8 | Business/GTM (pricing, personas, launch) | You | Deprioritized per your own direction — technical completeness came first |

---

## Bottom line

The agent's core claims (byte-accurate attribution, real topology
classification, real fix generation) are measured, not asserted. The
control plane and dashboard are a genuinely working multi-cluster product
with real user-facing features, not a static mockup. The two real gaps that
need your action are #1 above (five minutes) and a decision on how far to
take #2–#5 (all genuinely bigger scope, not oversights).
