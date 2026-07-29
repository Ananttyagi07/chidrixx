#!/usr/bin/env bash
# Overhead benchmark (build manual NFR-1: <1% CPU/node). Measures the
# kharcha agent's own CPU/memory usage via `kubectl top` (needs
# metrics-server) and expresses CPU as a percentage of each node's
# allocatable capacity.
#
# Honest scope note: this measures the agent's steady-state footprint on
# whatever traffic the cluster is already carrying. It is NOT the same as
# the manual's precise bar ("at 10k concurrent flows"), which needs a
# synthetic flow-generation load harness this repo doesn't have yet — that
# would need something to actually create 10k distinct concurrent flows
# (e.g. many parallel iperf3/curl connections across many source/dest
# pairs) and is real additional work, not a rename of this script.
#
# Usage: test/bench/overhead_bench.sh [kharcha-namespace]
# Requires: metrics-server running in the cluster (k3d ships it by
# default; `kubectl top nodes` failing means it's not available/ready
# yet).

set -euo pipefail

KHARCHA_NS="${1:-kharcha}"

if ! kubectl top nodes >/dev/null 2>&1; then
  echo "kubectl top failed — metrics-server isn't available/ready in this cluster." >&2
  exit 1
fi

echo "== kharcha agent pod CPU/memory =="
kubectl top pods -n "$KHARCHA_NS" -l app.kubernetes.io/name=kharcha --no-headers

echo
echo "== per-node overhead as % of allocatable CPU =="
printf '%-30s %10s %10s %8s\n' "NODE" "AGENT_CPU" "ALLOC_CPU" "PCT"

kubectl top pods -n "$KHARCHA_NS" -l app.kubernetes.io/name=kharcha --no-headers | while read -r pod cpu _mem; do
  node=$(kubectl get pod -n "$KHARCHA_NS" "$pod" -o jsonpath='{.spec.nodeName}')
  alloc=$(kubectl get node "$node" -o jsonpath='{.status.allocatable.cpu}')

  awk -v cpu="$cpu" -v alloc="$alloc" -v node="$node" '
    function to_millicores(v) {
      if (v ~ /m$/) { sub(/m$/, "", v); return v + 0 }
      return (v + 0) * 1000
    }
    BEGIN {
      cpu_m = to_millicores(cpu)
      alloc_m = to_millicores(alloc)
      pct = (alloc_m > 0) ? (cpu_m / alloc_m * 100) : -1
      printf "%-30s %9sm %9sm %7.2f%%\n", node, cpu_m, alloc_m, pct
    }
  '
done

echo
echo "note: this is idle/current-traffic overhead, not the 10k-concurrent-flow"
echo "benchmark NFR-1 specifically asks for — see the script header."
