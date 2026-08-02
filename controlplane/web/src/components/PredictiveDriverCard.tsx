import { useEffect, useState } from "react";
import { apiFetch } from "../apiFetch";
import { motion } from "framer-motion";
import { cardMotion } from "../motion";
import { formatINR } from "../format";
import { holtForecast } from "../forecast";
import type { CostTrendPoint, WorkloadGrowth } from "../types";

// Extends the Holt forecast (TrendProjectionCard) with a real "why" when
// it's trending up: the workload with the largest real cost increase over
// the retained history (controlplane/workloadgrowth.go, the same ranking
// the History page shows), reused here rather than inventing a separate
// "reason" -- if a total-cost forecast is trending up, the workload
// that's actually grown the most in real terms is the most defensible
// thing to point at, not a guess.
export function PredictiveDriverCard({ trend }: { trend: CostTrendPoint[] }) {
  const [growth, setGrowth] = useState<WorkloadGrowth[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    apiFetch("/api/v1/workload-growth")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((d) => !cancelled && setGrowth(d))
      .catch(() => !cancelled && setGrowth([]));
    return () => {
      cancelled = true;
    };
  }, []);

  const result = holtForecast(trend, Math.min(trend.length, 10));
  if (!result || growth === null) return null;

  const last = result.forecast[result.forecast.length - 1];
  const lastReal = trend[trend.length - 1]?.CostHigh ?? 0;
  const direction = last.forecast > lastReal ? "up" : last.forecast < lastReal ? "down" : "flat";

  if (direction !== "up") return null;

  const topGrower = growth.find((g) => g.delta_inr > 0);

  return (
    <motion.div
      {...cardMotion}
      className="rounded-2xl border border-dashed border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-1 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
        Likely driver
      </div>
      {topGrower ? (
        <>
          <div className="text-sm">
            <span className="font-mono font-semibold">{topGrower.workload}</span> on{" "}
            <span className="text-[var(--ink-secondary)]">{topGrower.cluster_id}</span> grew{" "}
            <span className="font-semibold text-[var(--status-critical)]">
              +{formatINR(topGrower.delta_inr)}
            </span>{" "}
            over the retained history — the largest real increase of any workload, and the most
            defensible reason this forecast trends up.
          </div>
          {topGrower.related_events && topGrower.related_events.length > 0 && (
            <div className="mt-1.5 text-[0.7rem] text-[var(--ink-muted)]">
              A real Deployment event ({topGrower.related_events[0].message}) happened in its
              namespace during that window — worth checking, not proven causation.
            </div>
          )}
        </>
      ) : (
        <div className="text-sm text-[var(--ink-secondary)]">
          Trending up, but no single workload's real growth in the retained history accounts for
          it — likely broad-based, not one obvious driver.
        </div>
      )}
    </motion.div>
  );
}
