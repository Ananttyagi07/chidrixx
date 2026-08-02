import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import { cardMotion } from "../motion";
import { formatINR } from "../format";
import { apiFetch } from "../apiFetch";

interface PlacementResult {
  groups: number;
  workloads: number;
  edges: number;
  observed_cross_zone_inr: number;
  optimized_cross_zone_inr: number;
  potential_savings_inr: number;
  iterations: number;
}

const GROUP_OPTIONS = [2, 3, 4];

// The safe, offline first increment of a cost-aware placement algorithm
// (controlplane/placement.go): a real graph-partitioning heuristic
// (Kernighan-Lin-style local search) that answers "how much of this
// real cross-zone cost is avoidable by co-locating workloads that talk
// to each other a lot" -- computed entirely from already-ingested real
// findings. Not a live scheduler, not a placement recommendation: the
// agent doesn't ship per-workload zone identity today, only the
// resulting path-class label, so this can only answer the graph-
// theoretic question, not "move workload X to zone Y."
export function PlacementSimulatorCard() {
  const [groups, setGroups] = useState(3);
  const [result, setResult] = useState<PlacementResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    apiFetch(`/api/v1/placement/preview?groups=${groups}`)
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((d: PlacementResult) => !cancelled && setResult(d))
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [groups]);

  return (
    <motion.div
      {...cardMotion}
      className="flex flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-1 flex items-center justify-between gap-2">
        <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Placement simulator
        </div>
        <select
          value={groups}
          onChange={(e) => setGroups(Number(e.target.value))}
          className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2 py-1 text-xs outline-none focus:border-[var(--accent)]"
        >
          {GROUP_OPTIONS.map((g) => (
            <option key={g} value={g}>
              {g} zones
            </option>
          ))}
        </select>
      </div>
      <p className="mb-3 text-[0.68rem] text-[var(--ink-muted)]">
        A real graph-partitioning algorithm run offline against your current real cross-zone
        findings — never a live scheduler, never applied. "{groups} zones" means you need real
        redundant presence across that many zones (e.g. multi-AZ HA), not an arbitrary bucket —
        with few real workloads relative to that count, 0 savings is often the honest answer, not
        a bug. Ignores real constraints (node capacity, resource limits, anti-affinity) this
        doesn't model yet.
      </p>

      {error && (
        <div className="rounded-lg border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-3 py-2 text-xs text-[var(--status-critical)]">
          Couldn't load the placement simulation: {error}
        </div>
      )}

      {loading && !result && <div className="py-6 text-center text-xs text-[var(--ink-muted)]">Loading…</div>}

      {result && result.edges === 0 && (
        <div className="py-6 text-center text-xs text-[var(--ink-muted)]">
          No real cross-zone traffic to evaluate right now.
        </div>
      )}

      {result && result.edges > 0 && (
        <>
          <div className="mb-3 grid grid-cols-3 gap-2 text-center">
            <div className="rounded-lg border border-[var(--border)] bg-[var(--page)] p-2">
              <div className="text-[0.65rem] text-[var(--ink-muted)]">Observed</div>
              <div className="font-mono text-sm tabular-nums">{formatINR(result.observed_cross_zone_inr)}</div>
            </div>
            <div className="rounded-lg border border-[var(--border)] bg-[var(--page)] p-2">
              <div className="text-[0.65rem] text-[var(--ink-muted)]">Best possible</div>
              <div className="font-mono text-sm tabular-nums">{formatINR(result.optimized_cross_zone_inr)}</div>
            </div>
            <div className="rounded-lg border border-[var(--status-good)]/30 bg-[var(--status-good)]/5 p-2">
              <div className="text-[0.65rem] text-[var(--status-good)]">Potential savings</div>
              <div className="font-mono text-sm tabular-nums text-[var(--status-good)]">
                {formatINR(result.potential_savings_inr)}
              </div>
            </div>
          </div>
          <div className="text-[0.68rem] text-[var(--ink-muted)]">
            {result.workloads} real workloads, {result.edges} real cross-zone pairs, {result.iterations} real
            optimization steps.
          </div>
        </>
      )}
    </motion.div>
  );
}
