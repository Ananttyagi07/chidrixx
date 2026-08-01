import type { ClusterSummaryView } from "../types";
import { formatINR } from "../format";

// Real cluster -> cloud/region mapping (each cluster's agent reports the
// price book it was configured with). Chidrixx doesn't track network
// paths *between* clusters -- each agent only sees traffic inside its own
// cluster -- so this is a real topology of "which cluster runs where,"
// not a fabricated cross-cluster network graph.
export function ClusterTopologyCard({ clusters }: { clusters: ClusterSummaryView[] }) {
  if (clusters.length === 0) {
    return <div className="py-8 text-center text-sm text-[var(--ink-muted)]">No clusters have shipped data yet.</div>;
  }

  return (
    <div className="flex flex-col gap-2">
      {clusters.map((c) => (
        <div
          key={c.ClusterID}
          className="flex items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-2 text-sm"
        >
          <div className="flex items-center gap-2 truncate">
            <span className="font-mono text-xs font-semibold">{c.ClusterID}</span>
            <span className="rounded-full bg-[var(--accent-wash)] px-2 py-0.5 text-[0.68rem] text-[var(--accent)]">
              {c.Cloud || "unknown"}
            </span>
            <span className="text-xs text-[var(--ink-muted)]">{c.Region || "unknown region"}</span>
          </div>
          <span className="flex-shrink-0 font-mono text-xs tabular-nums text-[var(--ink-secondary)]">
            {formatINR(c.CostHighINR)}
          </span>
        </div>
      ))}
    </div>
  );
}
