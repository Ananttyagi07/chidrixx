import { motion } from "framer-motion";
import { Area, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { cardMotion } from "../motion";
import { formatINR } from "../format";
import type { CostTrendPoint } from "../types";

// A naive linear-regression projection over the real recent trend --
// deliberately not called "Forecast (next 7 days)" or framed as ML/AI:
// chidrixx's data is cumulative snapshots at whatever cadence agents
// happen to ship at, not a calendar-day time series, so a calendar-bound
// forecast would be a claim this can't back. This just extends the
// observed slope forward by the same number of points already observed,
// labeled as exactly what it is.
export function TrendProjectionCard({ points }: { points: CostTrendPoint[] }) {
  if (points.length < 2) {
    return (
      <motion.div
        {...cardMotion}
        className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
      >
        <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Trend projection
        </div>
        <div className="flex flex-1 items-center justify-center text-center text-sm text-[var(--ink-muted)]">
          Needs at least 2 real snapshots to fit a trend line.
        </div>
      </motion.div>
    );
  }

  const n = points.length;
  const xs = points.map((_, i) => i);
  const ys = points.map((p) => p.CostHigh);
  const meanX = xs.reduce((a, b) => a + b, 0) / n;
  const meanY = ys.reduce((a, b) => a + b, 0) / n;
  let num = 0;
  let den = 0;
  for (let i = 0; i < n; i++) {
    num += (xs[i] - meanX) * (ys[i] - meanY);
    den += (xs[i] - meanX) ** 2;
  }
  const slope = den === 0 ? 0 : num / den;
  const intercept = meanY - slope * meanX;

  const projectionCount = Math.min(n, 10);
  const lastReal = Math.max(ys[n - 1], 0);

  const chartData = points.map((p, i) => ({
    label: new Date(p.ReportedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    actual: p.CostHigh,
    projected: i === n - 1 ? p.CostHigh : null,
  }));

  for (let i = 1; i <= projectionCount; i++) {
    const x = n - 1 + i;
    const projected = Math.max(slope * x + intercept, 0);
    chartData.push({ label: `+${i}`, actual: null as unknown as number, projected });
  }

  const projectedEnd = chartData[chartData.length - 1].projected as number;
  const direction = projectedEnd > lastReal ? "up" : projectedEnd < lastReal ? "down" : "flat";

  return (
    <motion.div
      {...cardMotion}
      className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-1 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
        Trend projection
      </div>
      <div className="mb-2 text-[0.68rem] text-[var(--ink-muted)]">
        Linear fit over the last {n} snapshots — not a calendar-day forecast.
      </div>
      <div className="h-32">
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={chartData} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
            <defs>
              <linearGradient id="actualFill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--accent)" stopOpacity={0.15} />
                <stop offset="100%" stopColor="var(--accent)" stopOpacity={0} />
              </linearGradient>
            </defs>
            <XAxis dataKey="label" hide />
            <YAxis hide domain={[0, "auto"]} />
            <Tooltip
              contentStyle={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 8, fontSize: 11 }}
              formatter={(v: number, name: string) => [formatINR(v), name === "actual" ? "Actual" : "Projected"]}
            />
            <Area type="monotone" dataKey="actual" stroke="var(--accent)" strokeWidth={2} fill="url(#actualFill)" dot={false} connectNulls={false} />
            <Line
              type="monotone"
              dataKey="projected"
              stroke="var(--series-orange)"
              strokeWidth={2}
              strokeDasharray="4 3"
              dot={false}
              connectNulls
            />
          </ComposedChart>
        </ResponsiveContainer>
      </div>
      <div className="mt-2 flex items-center justify-between text-xs">
        <span className="text-[var(--ink-secondary)]">
          Projected: <span className="font-mono tabular-nums">{formatINR(Math.max(projectedEnd, 0))}</span>
        </span>
        <span
          className={
            direction === "up"
              ? "text-[var(--status-critical)]"
              : direction === "down"
                ? "text-[var(--status-good)]"
                : "text-[var(--ink-muted)]"
          }
        >
          {direction === "up" ? "↑ trending up" : direction === "down" ? "↓ trending down" : "→ flat"}
        </span>
      </div>
    </motion.div>
  );
}
