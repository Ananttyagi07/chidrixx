import { useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import type { DashboardSummary, FindingRow } from "../types";
import { PATH_CLASS_LABEL } from "../palette";
import { formatINR } from "../format";

function pluralize(n: number, unit: string): string {
  return `${n} ${unit}${n === 1 ? "" : "s"}`;
}

// Real derived observations over data the dashboard already computes or
// fetches -- a synthesis view, not a new data source. Every number here
// traces back to a real aggregate.
export function InsightsPage({ data }: { data: DashboardSummary }) {
  const [findings, setFindings] = useState<FindingRow[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v1/findings")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((d) => !cancelled && setFindings(d))
      .catch(() => !cancelled && setFindings([]));
    return () => {
      cancelled = true;
    };
  }, []);

  const topDestinations = useMemo(() => {
    if (!findings) return [];
    const byDest = new Map<string, number>();
    for (const f of findings) {
      byDest.set(f.destination, (byDest.get(f.destination) ?? 0) + f.cost_high_inr);
    }
    return Array.from(byDest.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);
  }, [findings]);

  const confidenceCounts = useMemo(() => {
    if (!findings) return { high: 0, med: 0, low: 0 };
    const out = { high: 0, med: 0, low: 0 };
    for (const f of findings) {
      if (f.confidence === "high") out.high++;
      else if (f.confidence === "med") out.med++;
      else if (f.confidence === "low") out.low++;
    }
    return out;
  }, [findings]);

  const busiestClass = data.spend_by_class[0];
  const costliestCluster = [...data.clusters].sort((a, b) => b.CostHighINR - a.CostHighINR)[0];

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">Insights</h2>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
          <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
            Top 5 destinations by cost
          </div>
          {!findings ? (
            <div className="text-sm text-[var(--ink-muted)]">Loading…</div>
          ) : topDestinations.length === 0 ? (
            <div className="text-sm text-[var(--ink-muted)]">No findings yet.</div>
          ) : (
            <ul className="flex flex-col gap-2">
              {topDestinations.map(([dest, cost]) => (
                <li key={dest} className="flex items-center justify-between gap-3 text-sm">
                  <span className="truncate font-mono text-xs" title={dest}>
                    {dest}
                  </span>
                  <span className="flex-shrink-0 font-mono tabular-nums">{formatINR(cost)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
          <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
            Classification confidence
          </div>
          {!findings ? (
            <div className="text-sm text-[var(--ink-muted)]">Loading…</div>
          ) : (
            <div className="flex flex-col gap-2 text-sm">
              <div className="flex justify-between">
                <span className="text-[var(--status-good)]">High confidence</span>
                <span className="tabular-nums">{pluralize(confidenceCounts.high, "flow")}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[var(--status-warning)]">Medium confidence</span>
                <span className="tabular-nums">{pluralize(confidenceCounts.med, "flow")}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-[var(--status-serious)]">Low confidence</span>
                <span className="tabular-nums">{pluralize(confidenceCounts.low, "flow")}</span>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
        <div className="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          At a glance
        </div>
        <ul className="flex flex-col gap-1.5 text-sm text-[var(--ink-secondary)]">
          {busiestClass && (
            <li>
              <span className="font-medium text-[var(--ink)]">
                {PATH_CLASS_LABEL[busiestClass.PathClass] ?? busiestClass.PathClass}
              </span>{" "}
              is the largest cost category ({formatINR(busiestClass.CostHighINR)} across{" "}
              {busiestClass.FindingCount} flows).
            </li>
          )}
          {costliestCluster && (
            <li>
              <span className="font-medium text-[var(--ink)]">{costliestCluster.ClusterID}</span> is the
              costliest cluster currently reporting ({formatINR(costliestCluster.CostHighINR)}).
            </li>
          )}
          {data.anomalies.length > 0 ? (
            <li>
              {data.anomalies.length} cluster{data.anomalies.length === 1 ? "" : "s"} flagged with cost growth
              ≥2x since last snapshot — see Anomalies.
            </li>
          ) : (
            <li>No clusters have shown a ≥2x cost jump since their last snapshot.</li>
          )}
          {!busiestClass && !costliestCluster && <li>Not enough data yet to summarize.</li>}
        </ul>
      </div>
    </motion.div>
  );
}
