import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import { Area, AreaChart, ResponsiveContainer } from "recharts";
import type { WorkloadGrowth } from "../types";
import { formatINR } from "../format";

// Real "which workload's cost grew the most, and why" -- ranked by the
// actual delta between a workload's first and most recent snapshot over
// whatever history this control plane has retained (controlplane/workloadgrowth.go).
// Deliberately not framed as "last 6 months": the real retained window is
// whatever it is, not a fabricated fixed period.
export function HistoryPage() {
  const [growth, setGrowth] = useState<WorkloadGrowth[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v1/workload-growth")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((d) => !cancelled && setGrowth(d))
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)));
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">History</h2>
      <p className="max-w-xl text-sm text-[var(--ink-muted)]">
        Ranked by the real change between each workload's first and most recent snapshot over
        whatever history this control plane has retained — not a fabricated fixed window.
      </p>

      {error && (
        <div className="rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
          Couldn't load history: {error}
        </div>
      )}

      {!growth && !error && <div className="text-sm text-[var(--ink-muted)]">Loading…</div>}

      {growth && growth.length === 0 && (
        <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] py-8 text-center text-sm text-[var(--ink-muted)] shadow-[var(--card-shadow)]">
          No workload has at least two real snapshots yet — nothing to compare.
        </div>
      )}

      {growth && growth.length > 0 && (
        <div className="flex flex-col gap-3">
          {growth.map((g) => (
            <div
              key={`${g.cluster_id}/${g.workload}`}
              className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
            >
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate font-mono text-xs font-semibold">{g.workload}</div>
                  <div className="text-[0.68rem] text-[var(--ink-muted)]">{g.cluster_id}</div>
                </div>
                <div className="flex items-center gap-3">
                  <div className="h-10 w-24">
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={g.trend.map((p) => ({ v: p.cost_high }))}>
                        <defs>
                          <linearGradient id={`hist-${g.cluster_id}-${g.workload}`} x1="0" y1="0" x2="0" y2="1">
                            <stop
                              offset="0%"
                              stopColor={g.delta_inr >= 0 ? "var(--status-critical)" : "var(--status-good)"}
                              stopOpacity={0.25}
                            />
                            <stop
                              offset="100%"
                              stopColor={g.delta_inr >= 0 ? "var(--status-critical)" : "var(--status-good)"}
                              stopOpacity={0}
                            />
                          </linearGradient>
                        </defs>
                        <Area
                          type="monotone"
                          dataKey="v"
                          stroke={g.delta_inr >= 0 ? "var(--status-critical)" : "var(--status-good)"}
                          strokeWidth={1.5}
                          fill={`url(#hist-${g.cluster_id}-${g.workload})`}
                          dot={false}
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                  <span
                    className={`whitespace-nowrap font-mono text-sm font-semibold tabular-nums ${
                      g.delta_inr >= 0 ? "text-[var(--status-critical)]" : "text-[var(--status-good)]"
                    }`}
                  >
                    {g.delta_inr >= 0 ? "+" : ""}
                    {formatINR(g.delta_inr)}
                  </span>
                </div>
              </div>
              {g.related_events && g.related_events.length > 0 && (
                <div className="mt-2 rounded-lg border border-dashed border-[var(--border)] bg-[var(--surface-sunken)] px-2.5 py-1.5 text-[0.68rem] text-[var(--ink-secondary)]">
                  A real Deployment event ({g.related_events[0].message || g.related_events[0].reason}) happened in
                  this namespace during this window — worth checking, not proven causation.
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </motion.div>
  );
}
