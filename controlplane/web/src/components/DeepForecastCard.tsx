import { useEffect, useState } from "react";
import { Area, ComposedChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { motion } from "framer-motion";
import { cardMotion } from "../motion";
import { formatINR } from "../format";
import { apiFetch } from "../apiFetch";
import type { ClusterSummaryView, ForecastResponse } from "../types";

// The real deeper-forecasting-model upgrade: server-side (forecast.go),
// using the cluster's full retained history (not the 30-point cap the
// lightweight Overview trend card uses) and real rolling-origin
// backtesting that compares plain Holt against damped-trend Holt and
// picks whichever actually measured lower error on that cluster's own
// held-out real history -- not an assumed-better model. See
// PROJECT_STATUS.md §3.14 for why this, not Holt-Winters/ARIMA/a neural
// model: the real production data doesn't span enough calendar days to
// honestly fit a seasonal component yet.
export function DeepForecastCard({ clusters }: { clusters: ClusterSummaryView[] }) {
  const [clusterID, setClusterID] = useState(clusters[0]?.ClusterID ?? "");
  const [data, setData] = useState<ForecastResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!clusterID) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    apiFetch(`/api/v1/forecast?cluster_id=${encodeURIComponent(clusterID)}`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((d: ForecastResponse) => !cancelled && setData(d))
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [clusterID]);

  if (clusters.length === 0) {
    return null;
  }

  return (
    <motion.div
      {...cardMotion}
      className="flex flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-1 flex items-center justify-between gap-2">
        <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">Deep forecast</div>
        <select
          value={clusterID}
          onChange={(e) => setClusterID(e.target.value)}
          className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2 py-1 text-xs outline-none focus:border-[var(--accent)]"
        >
          {clusters.map((c) => (
            <option key={c.ClusterID} value={c.ClusterID}>
              {c.ClusterID}
            </option>
          ))}
        </select>
      </div>
      <p className="mb-2 text-[0.68rem] text-[var(--ink-muted)]">
        Real rolling-origin backtesting over this cluster's full retained history, comparing plain
        Holt against damped-trend Holt and using whichever actually measured lower error — not an
        assumed-better model, and not a calendar-day forecast.
      </p>

      {error && (
        <div className="rounded-lg border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-3 py-2 text-xs text-[var(--status-critical)]">
          Couldn't load a forecast: {error}
        </div>
      )}

      {loading && !data && <div className="py-6 text-center text-xs text-[var(--ink-muted)]">Loading…</div>}

      {data && !data.available && (
        <div className="py-6 text-center text-xs text-[var(--ink-muted)]">
          Not enough real history for this cluster yet (needs at least 3 snapshots).
        </div>
      )}

      {data?.available && data.result && (
        <>
          <div className="h-32">
            <ResponsiveContainer width="100%" height="100%">
              <ComposedChart
                data={data.result.forecast.map((f) => ({
                  label: `+${f.h}`,
                  forecast: f.forecast,
                  bandBase: f.lower,
                  bandWidth: f.upper - f.lower,
                }))}
                margin={{ top: 4, right: 4, left: 0, bottom: 0 }}
              >
                <XAxis dataKey="label" hide />
                <YAxis hide domain={[0, "auto"]} />
                <Tooltip
                  formatter={(v: number) => formatINR(v)}
                  contentStyle={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 8, fontSize: 11 }}
                />
                <Area dataKey="bandBase" stroke="none" fill="transparent" stackId="band" />
                <Area dataKey="bandWidth" stroke="none" fill="var(--series-orange)" fillOpacity={0.15} stackId="band" />
                <Area type="monotone" dataKey="forecast" stroke="var(--series-orange)" strokeWidth={2} fill="none" dot={false} />
              </ComposedChart>
            </ResponsiveContainer>
          </div>
          <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-[0.68rem] text-[var(--ink-secondary)]">
            <div>
              Model: <span className="font-mono">{data.result.model === "damped_holt" ? "damped Holt" : "Holt"}</span>
            </div>
            <div>
              Points used: <span className="font-mono tabular-nums">{data.result.points_used}</span>
            </div>
            {data.result.backtest_folds > 0 ? (
              <>
                <div>
                  Backtest folds: <span className="font-mono tabular-nums">{data.result.backtest_folds}</span>
                </div>
                <div>
                  Backtested error (Holt vs damped):{" "}
                  <span className="font-mono tabular-nums">
                    {data.result.backtest_mae_holt.toFixed(3)} vs {data.result.backtest_mae_damped.toFixed(3)}
                  </span>
                </div>
              </>
            ) : (
              <div className="col-span-2 text-[var(--ink-muted)]">
                Not enough history to backtest a model comparison — using a single Holt fit.
              </div>
            )}
          </div>
        </>
      )}
    </motion.div>
  );
}
