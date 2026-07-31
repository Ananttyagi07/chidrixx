# Chidrixx / kharcha

An eBPF agent that attributes Kubernetes network traffic to workloads and
prices it, so you can see which pod is talking to which destination, over
which path (same node / same zone / cross-zone / cross-region / NAT /
internet / managed service), and roughly what that's costing.

Two `cgroup_skb` programs ([bpf/flow_cgroup.c](bpf/flow_cgroup.c)) count
bytes per `(cgroup_id, 5-tuple)` in a per-CPU LRU hash map — no packets ever
leave the kernel, only aggregated counters. A Go agent
([agent/cmd/kharcha](agent/cmd/kharcha)) loads and attaches those programs
itself, scrapes the map every 15s, resolves cgroup IDs and remote IPs to
Kubernetes pods/services/EndpointSlices, classifies each flow's path, prices
it against an overridable YAML price book, and reports the result as a CLI
table, an HTML page, and Prometheus metrics — with optional Slack alerting
on cost thresholds.

## Quickstart (local dev, e.g. a k3d/kind cluster)

```bash
# 1. Compile the eBPF object (needs clang + bpftool + kernel headers)
clang -O2 -g -target bpfel -D__TARGET_ARCH_x86 -I bpf -c bpf/flow_cgroup.c -o bpf/flow_cgroup.o

# 2. Run the agent (needs root/CAP_BPF to load and attach the programs)
cd agent
sudo go run ./cmd/kharcha \
  -bpf-object=../bpf/flow_cgroup.o \
  -pricebook=../pricebook/aws.yaml
```

Point `kubectl` at your cluster first (the agent shells out to `kubectl`
for pod/service/endpoint/node metadata) — if `kubectl get pods -A` already
works in your shell, the agent will use the same config.

Within a few scrape cycles you'll see:
- A ranked CLI table on stdout, refreshed every 15s.
- `report.html` in the working directory (same data, for a browser).
- `curl localhost:9300/metrics` — Prometheus metrics.

Ctrl+C detaches the eBPF programs cleanly and prints a final report before
exiting.

## Running in a cluster (Helm)

The chart's default image is published on GHCR
(`ghcr.io/ananttyagi07/chidrixx-agent:latest`). Note: as of this writing
that package is still private — pulling it from outside this account needs
either the package set to public or an `imagePullSecrets` entry with a
token that can read it. Once public, a plain install works against any
cluster with no local build step:

```bash
helm install kharcha deploy/helm/kharcha -n kharcha --create-namespace
```

To iterate on a local change instead of pulling the published image:

```bash
docker build -t chidrixx-agent:dev .
k3d image import chidrixx-agent:dev -c <your-cluster>   # or push your own build to a registry
helm install kharcha deploy/helm/kharcha -n kharcha --create-namespace \
  --set image.repository=chidrixx-agent --set image.tag=dev
```

See [deploy/helm/kharcha/values.yaml](deploy/helm/kharcha/values.yaml) for
the price book, managed-service CIDRs, NAT-egress heuristic toggle, and
alert-webhook (Secret-backed) knobs.

### Multi-cluster control plane (optional)

The agent works standalone (CLI/HTML/Prometheus, single cluster) with no
control plane at all. For a multi-cluster view, deploy
[controlplane/](controlplane) (also published at
`ghcr.io/ananttyagi07/chidrixx-controlplane:latest`) and point one or more
agents at it:

```bash
helm install chidrixx deploy/helm/controlplane -n chidrixx --create-namespace

# then, per agent:
helm upgrade kharcha deploy/helm/kharcha -n kharcha \
  --set controlPlane.url=http://chidrixx-controlplane.chidrixx.svc.cluster.local:8090/api/v1/ingest \
  --set controlPlane.clusterID=<a-name-for-this-cluster>
```

The control plane serves a real dashboard at `/` — a React/Tailwind/Recharts
SPA ([controlplane/web](controlplane/web)) built against
[controlplane/summary_api.go](controlplane/summary_api.go)'s
`/api/v1/dashboard-summary`: total spend, data transferred, active
workloads, spend-by-path-class, a combined spend trend, per-cluster cards
with sparklines, and the top flagged flows with their generated
NetworkPolicy manifests. Every number on it is a real aggregate over
ingested data — no forecasting, no multi-cloud breakdown, no invented
"efficiency score," since chidrixx doesn't compute any of those.

The built assets (`controlplane/web/dist`) are committed to git and
embedded into the Go binary via `go:embed`, the same way
`bpf/flow_cgroup.o` is committed for the agent — `go build`/`go test`
work out of the box without Node installed. After changing anything under
`controlplane/web/src`, rebuild and re-commit the output:

```bash
cd controlplane/web && npm install && npm run build
```

For frontend-only iteration, `npm run dev` proxies API calls to a
locally running control plane (`go run . -addr=:8090` from
`controlplane/`).

By default the control plane is unauthenticated — fine for a quick local
trial, not for anything reachable beyond a trusted network. Set a shared
token on both sides via `CHIDRIXX_AUTH_TOKEN` (control plane) and the
matching `controlPlane.tokenSecretName`/`auth.tokenSecretName` Helm values
(agent/control plane respectively) — see
[deploy/helm/controlplane/values.yaml](deploy/helm/controlplane/values.yaml).
Basic Auth, not Bearer, deliberately: it's the one mechanism that works
naturally for both an agent posting JSON and a human viewing the
dashboard in a plain browser.

The static dashboard shell itself (`GET /`, its JS/CSS) is served
**without** auth — it carries no secrets, everything real is fetched from
`/api/v1/*` at runtime, and this lets the app show its own landing screen
before the browser's native Basic Auth dialog ever fires. Only `/api/*` is
behind `requireToken`; the first authenticated fetch is what actually
triggers the credential prompt.

### Known limitation: cgroup namespaces

The agent needs to attach `cgroup_skb` programs at the node's cgroup v2
root, which needs real host-level cgroup namespace privilege — a
`privileged: true` Pod is not enough on its own. Confirmed on this repo's
own k3d dev cluster: Docker defaults to `--cgroupns=private` on cgroup v2
hosts (20.10+), which puts every container — including k3d's own node
containers — in its own private cgroup namespace, one level removed from
the host's real init namespace. `cgroup_skb` attach fails with `operation
not permitted` across that boundary no matter what the Pod's
`securityContext` says, because capabilities are scoped to the namespace
they were granted in.

Fix: make sure the nodes actually run with the host's cgroup namespace —
either set `"default-cgroupns-mode": "host"` in `/etc/docker/daemon.json`
and restart Docker (affects all containers on that host), or use a cluster
type/config that doesn't privatize the cgroup namespace for its nodes. Real
managed Kubernetes nodes (EKS/GKE/AKS) don't hit this — it's specific to
Docker-in-Docker style local clusters (kind/k3d) on a cgroup v2 host.

## Testing

```bash
cd agent
go test ./...
```

Most tests are pure logic and run unprivileged (classifier, pricing bands,
HTML rendering, the Slack alerter's HTTP/debounce logic against a real
`httptest.Server`, and a real-cluster integration test gated on this repo's
own `chidrixx-test` namespace fixture). Two tests load actual BPF programs
and need root/`CAP_BPF` — they skip cleanly without it:

```bash
sudo go test ./... -run 'TestEgressByteAccounting|TestLoadAttachesAndDetaches'
```

## Flags

Run `go run ./cmd/kharcha -h` for the full list. The ones you'll actually
want to change: `-pricebook`, `-managed-cidrs`, `-node-has-public-ip`
(affects the NAT-egress heuristic), `-alert-webhook` /
`-alert-threshold-inr` / `-alert-growth-ratio`.

## What's been verified for real (not just unit-tested)

- **Byte accuracy**: a real `iperf3 -n 1G` transfer between two pods measured
  1,073,741,824 payload bytes; the agent independently measured 1,075,417,978
  on-wire bytes for the same flow (client TX exactly matches server RX —
  cross-validated from both ends). The 0.156% difference is real TCP/IP
  header overhead that `cgroup_skb` correctly counts as on-wire bytes, not
  measurement error — well inside the ≤1% bar.
- **`SAME_AZ`/`CROSS_AZ`/`CROSS_REGION` classification**: proven on a real
  4-node cluster with real `topology.kubernetes.io/zone` labels
  (`ap-south-1a` ×2 nodes, `ap-south-1b`, `us-east-1a`) — all four classes
  (`SAME_NODE`/`SAME_AZ`/`CROSS_AZ`/`CROSS_REGION`) confirmed against actual
  cross-node pod traffic. Caught a real, minor issue along the way: a new
  pod's traffic briefly classifies as `PRIVATE_OFFCLUSTER` until the next
  Kubernetes metadata refresh (currently every 30s) catches up — worth
  tightening toward FR-E1's ≤10s target.
- **Alert delivery**: a real alert was posted to a live external HTTP
  endpoint and the exact received payload was fetched back and inspected
  (`{"text": "..."}`, matching Slack's incoming-webhook format exactly).
  `alert_live_test.go` makes this repeatable against a real Slack webhook:
  `KHARCHA_ALERT_WEBHOOK_URL=https://hooks.slack.com/... go test ./cmd/kharcha/... -run TestAlerterRealWebhookDelivery -v`
- **CI**: `.github/workflows/ci.yml` runs build/vet/test, recompiles the BPF
  object from source and re-runs the privileged load/attach tests against
  it, `helm lint`/`helm template`, and a Docker build — all confirmed
  passing on GitHub-hosted runners. Those runners are full VMs, not nested
  containers, so they don't hit the cgroupns issue above.
- **Chaos/safety (NFR-5)**: `test/chaos/chaos_test.sh` force-killed the
  kharcha agent pod mid-traffic against a real client/server fixture —
  0 non-200 responses out of 45 requests during the kill and DaemonSet
  reschedule window. Confirms the architecture claim (`kharcha_egress`/
  `kharcha_ingress` unconditionally `return 1`/SK_PASS, so the hooks only
  count and never gate) against a real cluster, not just the source.
- **Overhead (NFR-1)**: `test/bench/overhead_bench.sh` first measured the
  agent at 133m CPU / 9Mi memory on an 8-core node — 1.66% of allocatable
  CPU, over the <1%-per-node target. Root cause: `KubernetesResolver.Refresh`
  ([agent/cmd/kharcha/kubernetes.go](agent/cmd/kharcha/kubernetes.go)) shelled
  out to `kubectl` four times (pods/services/endpointslices/nodes) every
  refresh cycle — each invocation forks a whole new process and TLS
  handshake, dwarfing the actual eBPF map scrape cost. Fixed by talking to
  the API server directly over a single keep-alive `http.Client` using the
  mounted service-account token/CA (falling back to the original
  kubectl-exec path when running outside a cluster, for local dev). Re-measured
  after the fix: **4m CPU, 0.05% of allocatable** — comfortably under budget.
  That was still idle/current-traffic overhead, not the manual's precise
  "at 10k concurrent flows" bar — closing that gap surfaced a second real
  bug: the eBPF map itself
  ([bpf/flow_cgroup.c](bpf/flow_cgroup.c)) was capped at `max_entries=4096`
  and silently LRU-evicted under load. [test/load/](test/load) is a
  purpose-built flow generator (20 sink replicas + a 20-pod Job driving 500
  held-open TCP connections each, ~10k concurrent flows total, since the
  existing nginx test fixture can't sustain that many on its own) that
  caught it: tracked flows plateaued at ~3.6k instead of the ~9.7k
  connections actually open. Fixed by sizing the map up to 16384 and
  re-running the same test: **10,543 concurrent flows tracked, agent CPU at
  27m — 0.34% of allocatable** — genuinely inside the ≤1% NFR-1 bar at the
  scale the manual actually asks for, not just under light traffic.
- **Control-plane Helm chart, actually `helm install`ed**: every prior
  control-plane test ran the binary directly (`docker run`), never through
  [deploy/helm/controlplane](deploy/helm/controlplane) itself. Installed it
  for real (`helm install chidrixx deploy/helm/controlplane -n chidrixx`,
  token-secret-backed auth, PVC bound via the cluster's `local-path`
  storage class), then `helm upgrade`d the real `kharcha` agent release to
  point at it. Confirmed end-to-end: 401 without the Basic Auth token, 200
  with it, and the control plane's `/api/v1/findings` API returned real
  findings shipped by the real agent (`ClusterID: chidrixx-lab`, live
  timestamps) — not the curl-simulated ingestion used earlier.
- **Genuine two-real-cluster multi-cluster proof**: spun up a second, fully
  independent k3d cluster (its own Docker network, its own agent
  install — not a curl simulating a second cluster ID) and pointed its
  agent at the same control plane by connecting the two clusters' Docker
  networks and exposing the control plane via a NodePort. Confirmed
  `/api/v1/findings` returned real findings tagged with both `chidrixx-lab`
  and `chidrixx-lab-2`, and the dashboard rendered both cluster cards. Also
  incidentally reconfirmed the cgroupns=host fix generalizes: the second
  cluster's agent attached its `cgroup_skb` programs with no special
  handling. Torn down afterward — this was a proof, not a permanent
  fixture.
- **Control-plane Ingress/TLS**: added an optional `ingress.*` block to
  [deploy/helm/controlplane](deploy/helm/controlplane) (disabled by
  default, since the Service being ClusterIP-only was flagged as a real
  gap — Basic Auth over plain HTTP is fine inside a cluster, not for
  exposing it beyond one). `helm upgrade`d the live control plane with it
  enabled and confirmed real routing: a request with the right `Host`
  header and Basic Auth credentials through Traefik got `200`; this also
  surfaced and fixed a self-inflicted issue where the GHCR image-reference
  change above (still private) had silently made the *next* upgrade of
  this same release try to pull the private image — fixed by pinning
  `image.repository`/`image.tag` explicitly until the packages go public.
- **Helm-managed auth token**: bootstrapping used to mean coming up with
  your own token and manually `kubectl create secret`-ing it on both
  sides. The control plane now auto-generates and manages its own token
  secret by default (`auth.generate: true`), kept stable across upgrades
  via a `lookup` check (verified for real: two consecutive `helm upgrade`s
  produced the identical token, not a fresh one each time) and surfaced in
  `NOTES.txt`. Syncing that token to the agent's *independent* Helm release
  is still one manual copy — there's no shared secret store between two
  separate releases, possibly in different clusters — but re-ran the whole
  ship pipeline with the freshly-generated token end-to-end (real findings
  visible through the control plane's API) to confirm it actually works,
  not just renders.
- **Fix engine beyond a static sentence**: the "fix" a Finding carried used
  to be one of five fixed strings keyed only by path class
  ([agent/cmd/kharcha/report.go](agent/cmd/kharcha/report.go)'s
  `fixHints`) — never anything specific to the actual flow. Added
  [agent/cmd/kharcha/fixengine.go](agent/cmd/kharcha/fixengine.go), which
  mechanically generates a real, copy-pasteable `NetworkPolicy` manifest
  for `INTERNET_EGRESS`/`NAT_EGRESS`/`CROSS_REGION` findings, scoped to the
  source pod's actual namespace and the actual flagged destination
  IP/CIDR — an allow-all-except-this-destination policy, since
  NetworkPolicies are allow-lists and surgically blocking one wasteful
  destination needs the inverse of a bare deny rule.
  `CROSS_AZ`/`MANAGED_SERVICE`'s real fix (co-locating workloads by label)
  depends on pod labels this agent doesn't resolve — fabricating a label
  selector that might not match the real Deployment would be worse than
  the plain-text hint those classes keep instead.

  Getting this to actually reach a viewer surfaced two more wiring gaps,
  both fixed and verified: the control plane's own `Finding` model and
  SQLite schema didn't carry a manifest field at all (fixed with a
  `fix_manifest` column and a real migration path — tested against an
  actual pre-existing database file with the old schema, not just an
  in-memory one), and `shipper.go` had its own hand-maintained wire struct
  that silently dropped the new field (fixed, with the contract test
  updated to catch this class of drift in future). End-to-end proof: a
  real pod curling `1.1.1.1` produced a real `NetworkPolicy` named
  `deny-1-1-1-1` scoped to `namespace: default`, visible through both the
  control plane's API and its dashboard.
- **Light theme + a landing page, via `shadcn`/react-bits**: the dashboard's
  default is now an enforced light theme (no more following the OS's
  `prefers-color-scheme`), with real react-bits components
  (`DecryptedText`, `ScrollFloat`, `RotatingText`, `VariableProximity`)
  installed through the actual `npx shadcn@latest add @react-bits/...`
  CLI — Node 18 can't run it (needs 20+), so a portable Node 20 was used
  just to run the installer once; the checked-in output needs nothing
  beyond what was already required. A new pre-auth landing page
  (`controlplane/web/src/components/LandingPage.tsx`) lists only real
  features (path classification, the fix engine, multi-cluster
  aggregation) — nothing the mockup showed that chidrixx can't do. This
  required actually splitting the server's routing
  ([controlplane/main.go](controlplane/main.go)): the static SPA shell now
  serves without auth (it carries no secrets) so the landing page can
  render before any credential prompt; only `/api/*` stays behind
  `requireToken`.

  Verified for real, not just visually: a scripted Playwright click-through
  (landing → "View live dashboard" → scroll) caught two real bugs before
  they shipped. First, a fresh install with zero ingests ever received
  crashed the dashboard outright — Go's nil-slice-marshals-to-`null`
  behavior meant `trend`/`spend_by_class`/etc. serialized as JSON `null`
  instead of `[]`, and the frontend called `.map()` on them unguarded;
  fixed at the source (`store.go`, `make([]T, 0)` instead of `var out []T`)
  with a regression test (`TestHandleDashboardSummaryEmptyStateHasNoNullArrays`)
  rather than patched over client-side. Second, `ScrollFloat`'s section-title
  reveal animation went permanently invisible for anything below the first
  viewport — GSAP's `ScrollTrigger` defaults to tracking `window` scroll,
  but the dashboard actually scrolls inside `main` (`overflow-y-auto`), so
  window-scroll events never fired; fixed by threading the real scroll
  container ref through every `SectionTitle` usage.
