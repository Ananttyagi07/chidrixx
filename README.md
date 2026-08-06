# chidrixx / kharcha

An eBPF agent that attributes Kubernetes network traffic to workloads and
prices it — which pod is talking to which destination, over which path
(same node / same zone / cross-zone / cross-region / NAT / internet /
managed service), and roughly what that's costing. A multi-tenant control
plane aggregates this across clusters behind a real dashboard.

Two `cgroup_skb` programs ([bpf/flow_cgroup.c](bpf/flow_cgroup.c)) count
bytes per `(cgroup_id, 5-tuple)` in a per-CPU LRU hash map — **no packet
payload ever leaves the kernel, only aggregated counters.** Both programs
unconditionally `return 1` (SK_PASS): they count, they never gate traffic.
A Go agent ([agent/cmd/kharcha](agent/cmd/kharcha)) loads and attaches
them itself, scrapes the map, resolves cgroup IDs and remote IPs to
Kubernetes pods/services/EndpointSlices **directly through the API server**
(not by shelling out to `kubectl`), classifies each flow's path, prices it
against an overridable YAML price book, and ships the result to the
control plane.

> The exhaustive engineering record — every claim, which file, which test,
> which command proved it — lives in **[PROJECT_STATUS.md](PROJECT_STATUS.md)**.
> This README is the front door; that file is the audit trail.

## Install into a cluster

Both charts and both images are **public on GHCR** — no credentials, no
cloning this repo:

```bash
# 1. Control plane (dashboard + storage)
helm install chidrixx oci://ghcr.io/ananttyagi07/charts/chidrixx-controlplane \
  --version 0.2.0 -n chidrixx --create-namespace

# 2. Grab the admin password and the one-time agent ingest token
kubectl get secret -n chidrixx chidrixx-controlplane-admin-password \
  -o jsonpath='{.data.password}' | base64 -d; echo
kubectl logs -n chidrixx -l app.kubernetes.io/name=chidrixx-controlplane --tail=200 | grep -i token

# 3. Store the token for the agent
kubectl create namespace kharcha
kubectl create secret generic kharcha-controlplane-token \
  -n kharcha --from-literal=token='<TOKEN>'

# 4. Agent (DaemonSet, one pod per node)
helm install kharcha oci://ghcr.io/ananttyagi07/charts/kharcha \
  --version 0.2.0 -n kharcha \
  --set controlPlane.url=http://chidrixx-controlplane.chidrixx.svc.cluster.local:8090/api/v1/ingest \
  --set controlPlane.clusterID=my-cluster \
  --set controlPlane.tokenSecretName=kharcha-controlplane-token
```

Then `kubectl port-forward -n chidrixx svc/chidrixx-controlplane 8090:8090`
and open `http://localhost:8090` → **Log in** → **"Sign in with username
instead"** → `admin` + the password from step 2.

If the ingest token scrolls out of the logs, mint a new one rather than
reinstalling:

```bash
kubectl exec -n chidrixx deploy/chidrixx-controlplane -- \
  /app/controlplane create-token --db /data/controlplane.db --tenant-id 1
```

**Requirements:** kernel **5.8+** with cgroup v2 (EKS/GKE/AKS defaults are
fine). Chart versions and image tags move together — `image.tag` defaults
to the chart's own `appVersion`, so a chart never deploys an image it
wasn't released with. Releases are cut by
[.github/workflows/release.yml](.github/workflows/release.yml), which
refuses to publish on a version mismatch and re-pulls the result
anonymously to prove it's installable.

### Local iteration

```bash
docker build -t chidrixx-agent:dev .
k3d image import chidrixx-agent:dev -c <cluster>
helm install kharcha deploy/helm/kharcha -n kharcha --create-namespace \
  --set image.repository=chidrixx-agent --set image.tag=dev
```

## Security posture

Three things a security review will ask about, answered concretely:

**1. What privileges does the agent need?** By default it runs
`privileged: false`, `allowPrivilegeEscalation: false`, `drop: ["ALL"]`,
and adds exactly **two** capabilities:

| Capability | Why |
|---|---|
| `CAP_BPF` | the `bpf()` syscall — create maps, load programs (5.8+) |
| `CAP_NET_ADMIN` | required to load **and** attach a networking program type (`cgroup_skb`) |

That set was minimised by testing, not guessed: `CAP_PERFMON` was included
first, then removed after the agent was verified still loading, attaching
and counting real flows without it. `security.mode: privileged` remains as
a fallback for pre-5.8 kernels.

**Stated plainly rather than overclaimed:** this does *not* satisfy
PodSecurity `baseline` or `restricted`. Neither permits adding
`CAP_BPF`/`CAP_NET_ADMIN`, and both forbid the `hostPath` cgroup mount that
attaching to the cgroup hierarchy requires — **no eBPF agent can meet
those profiles**, so that namespace needs an exemption regardless of
vendor. What this buys is a reviewable posture instead of a blanket
`privileged: true`.

**2. What does the agent read?** Byte and packet counters per flow tuple.
It never reads packet payloads — auditable in ~60 lines of C.

**3. What leaves my environment if I enable AI?** The optional assistant
(`GROQ_API_KEY`) sends **aliased** data by default: workload, namespace,
cluster and endpoint identifiers are pseudonymised, and generated
NetworkPolicy manifests are dropped entirely before anything is sent.
Every number — bytes, costs, ratios, path class, confidence — is passed
through unchanged, so accuracy is unaffected; aliases are mapped back on
the way out so you still read your own real names. Controlled by
`CHIDRIXX_AI_MODE` (`sanitized` default, `raw` to opt out); leaving
`GROQ_API_KEY` unset disables AI entirely. Caveat: a question you *type*
containing a real workload name is sent as typed.

## What it does

**Agent** — 8 path classes with confidence-widened cost bands; a fix
engine that generates real, copy-pasteable `NetworkPolicy` manifests;
Prometheus metrics including `kharcha_map_utilization_ratio` (the eBPF map
evicts silently at capacity, so occupancy is the only honest early
warning — alert rules ship in
[deploy/prometheus/kharcha-alerts.yaml](deploy/prometheus/kharcha-alerts.yaml));
Slack-compatible cost alerting.

**Control plane** — real multi-tenant auth (bcrypt, server-tracked
sessions, per-tenant ingest tokens, `admin`/`viewer` roles enforced
server-side), 21 API endpoints, and a React dashboard covering: cost graph
topology, anomaly detection with deploy-event correlation, backtested
forecasting, spend-by-team, historical workload growth, an offline
placement simulator, a dry-run remediation preview with a **traffic-replay
safety check** (replays a generated policy against your own recorded
traffic to find workloads it would break), closed-loop outcome tracking,
automated retention/compaction, and an optional grounded AI assistant with
its own evaluation telemetry.

## Known limitations

- **cgroup namespaces (kind/k3d).** Attaching at the node's cgroup v2 root
  needs the host's cgroup namespace. Docker defaults to
  `--cgroupns=private` on cgroup v2 hosts, so `cgroup_skb` attach fails
  with `EPERM` across that boundary no matter what the `securityContext`
  says. The agent detects this specific `EPERM` and says so explicitly.
  Fix: `"default-cgroupns-mode": "host"` in `/etc/docker/daemon.json`.
  **Real managed Kubernetes (EKS/GKE/AKS) doesn't hit this.**
- **No latency measurement.** These hooks count bytes; they never
  timestamp round-trips. The cost graph says so rather than inventing one.
- **Price books are list pricing**, not your negotiated rate — override
  them in Helm values.
- **`INTERNET_EGRESS` gets no savings estimate.** Its real fix is usage
  reduction, not a cheaper path class, so the UI shows "—" rather than a
  fabricated number.
- **Correlation is labelled as correlation.** Deploy-event "likely cause"
  is never presented as proven causation.

[PROJECT_STATUS.md §11](PROJECT_STATUS.md) keeps the complete honesty
audit — every "not real" claim this codebase makes about itself, in one
place.

## Testing

```bash
cd agent && go test ./...            # 29 tests
cd controlplane && go test ./...     # 219 tests
cd controlplane/web && npm run test:e2e   # 28 Playwright tests (needs Node 20+)
```

Two agent tests load real BPF programs and need root/`CAP_BPF`; they skip
cleanly without it. CI ([.github/workflows/ci.yml](.github/workflows/ci.yml))
runs seven jobs across both modules — build/vet/test, a recompile of the
BPF object from source with the privileged load/attach tests re-run
against it, Docker builds, `helm lint`/`template`, and the full E2E suite.

## Verified for real, not just unit-tested

- **Byte accuracy**: a real `iperf3 -n 1G` transfer measured 1,073,741,824
  payload bytes; the agent independently measured 1,075,417,978 on-wire
  bytes (client TX cross-validated against server RX). The **0.156%**
  difference is real TCP/IP header overhead, not measurement error.
- **Overhead**: first measured at 133m CPU (1.66%) — over budget.
  Root-caused to `kubectl`-exec'ing four times per refresh cycle; fixed by
  talking to the API server directly. Re-measured: **4m CPU, 0.05%**.
- **10k concurrent flows**: a purpose-built load harness
  ([test/load/](test/load)) exposed a real bug — the BPF map was capped at
  4,096 and silently LRU-evicted, tracking ~3.6k of ~9.7k real
  connections. Fixed (16,384) and re-run: **10,543 concurrent flows at 27m
  CPU (0.34%)**. That silent-eviction failure is why the map-utilization
  metric and alert exist.
- **Chaos safety**: the agent pod was force-killed mid-traffic — **0
  non-200 responses out of 45 requests**, confirming the "hooks only count,
  never gate" claim against a real cluster.
- **Two genuinely independent clusters**: a second, fully separate k3d
  cluster with its own agent, reporting into the same control plane — not
  a curl simulating a second cluster ID.
- **Retention/compaction at real scale**: verified against a real
  **2.37M-row** copy of the production database — 2,205,218 raw rows folded
  into 1,998 daily rollups (**1,103×**), cost-conserving, integrity-checked.
- **AI evaluation against the live Groq API**: real success rate, latency,
  tool-call reliability and real token counts parsed from Groq's own
  response — not estimated.

## Flags

`go run ./cmd/kharcha -h` for the full list. The ones worth changing:
`-pricebook`, `-managed-cidrs`, `-node-has-public-ip` (NAT-egress
heuristic), `-alert-webhook` / `-alert-threshold-inr` /
`-alert-growth-ratio`, `-k8s-refresh-interval`.

Control-plane environment: `GROQ_API_KEY` (enables AI), `CHIDRIXX_AI_MODE`,
`CHIDRIXX_RAW_RETENTION_DAYS` (default 30),
`CHIDRIXX_ANOMALY_WATCH_INTERVAL` (default 5m), `CHIDRIXX_ADMIN_PASSWORD`,
and `SUPABASE_URL`/`SUPABASE_PUBLISHABLE_KEY` for optional Supabase-backed
signup.
