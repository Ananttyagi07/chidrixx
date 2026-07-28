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

```bash
docker build -t chidrixx-agent:dev .
k3d image import chidrixx-agent:dev -c <your-cluster>   # or push to a real registry
helm install kharcha deploy/helm/kharcha -n kharcha --create-namespace
```

See [deploy/helm/kharcha/values.yaml](deploy/helm/kharcha/values.yaml) for
the price book, managed-service CIDRs, NAT-egress heuristic toggle, and
alert-webhook (Secret-backed) knobs.

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

## What's not verified yet

- Byte accuracy against real cross-host traffic (`iperf3 -n 1G`, ≤1% error)
  — the unit test (`TestEgressByteAccounting`) proves single-packet
  accounting is exact via `BPF_PROG_TEST_RUN`; it isn't a substitute for a
  live multi-host volume test.
- True `SAME_AZ`/`CROSS_AZ`/`CROSS_REGION` classification — this repo's own
  dev cluster is single-node with no zone labels, so only `SAME_NODE` is
  exercised for real. The classifier degrades to low-confidence `SAME_AZ`
  rather than guessing when zone labels are absent (see
  [classify.go](agent/cmd/kharcha/classify.go)); a real multi-AZ cluster is
  needed to exercise the other branches.
- Real Slack delivery — the alerter's HTTP/JSON/debounce logic is tested
  against a real local HTTP server; whether a message renders correctly in
  an actual Slack channel needs a real webhook URL.
