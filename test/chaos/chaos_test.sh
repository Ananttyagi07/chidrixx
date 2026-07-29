#!/usr/bin/env bash
# Chaos/safety test (build manual NFR-5): kill the kharcha agent under
# active traffic, assert zero impact on unrelated pod-to-pod networking.
# "The test that turns 'no' into 'I'll run it in prod'."
#
# What this actually verifies, empirically: kharcha_egress/kharcha_ingress
# (bpf/flow_cgroup.c) unconditionally `return 1` (SK_PASS) — there is no
# code path that ever drops a packet. Removing the agent's eBPF programs,
# whether via a clean detach or the process dying without pinning them,
# should therefore have zero effect on whether packets are delivered: the
# hooks only count, they never gate. This script proves that expectation
# against a real cluster instead of resting on the architecture argument
# alone.
#
# Usage: test/chaos/chaos_test.sh [test-namespace] [kharcha-namespace] [duration-seconds]
# Requires: the chidrixx-test fixtures (client/server) and the kharcha
# DaemonSet both deployed and healthy.

set -euo pipefail

NAMESPACE="${1:-chidrixx-test}"
KHARCHA_NS="${2:-kharcha}"
DURATION="${3:-30}"

CLIENT_POD=$(kubectl get pods -n "$NAMESPACE" -l app=client -o jsonpath='{.items[0].metadata.name}')
if [ -z "$CLIENT_POD" ]; then
  echo "no client pod found in namespace $NAMESPACE — deploy the chidrixx-test fixtures first" >&2
  exit 1
fi

echo "== baseline: confirming client -> server connectivity works =="
BASELINE=$(kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- \
  curl -s -o /dev/null -w "%{http_code}" --max-time 5 \
  "http://server.$NAMESPACE.svc.cluster.local/")
if [ "$BASELINE" != "200" ]; then
  echo "baseline connectivity check failed (http_code=$BASELINE) — fix that before running the chaos test" >&2
  exit 1
fi
echo "baseline OK (http_code=200)"

LOG=$(mktemp)
trap 'rm -f "$LOG"' EXIT

echo
echo "== monitoring connectivity every 0.2s for ${DURATION}s, killing the agent mid-run =="

(
  END=$((SECONDS + DURATION))
  while [ "$SECONDS" -lt "$END" ]; do
    CODE=$(kubectl exec -n "$NAMESPACE" "$CLIENT_POD" -- \
      curl -s -o /dev/null -w "%{http_code}" --max-time 2 \
      "http://server.$NAMESPACE.svc.cluster.local/" 2>/dev/null || echo "000")
    echo "$(date +%s.%N) $CODE" >> "$LOG"
    sleep 0.2
  done
) &
MONITOR_PID=$!

sleep 3
echo "-- killing kharcha agent pod(s) now --"
kubectl delete pod -n "$KHARCHA_NS" -l app.kubernetes.io/name=kharcha --force --grace-period=0 --wait=false

wait "$MONITOR_PID"

TOTAL=$(wc -l < "$LOG")
FAILURES=$(grep -cv ' 200$' "$LOG" || true)

echo
echo "== results =="
echo "requests during the monitoring window: $TOTAL"
echo "non-200 responses: $FAILURES"

if [ "$FAILURES" -gt 0 ]; then
  echo
  echo "FAIL: $FAILURES/$TOTAL requests were not 200 while the agent was killed:"
  grep -v ' 200$' "$LOG"
  exit 1
fi

echo
echo "PASS: zero impact on client<->server traffic while forcibly killing the kharcha agent."
echo "(The DaemonSet will now reschedule a replacement pod automatically.)"
