import { motion } from "framer-motion";
import { Area, ComposedChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { TooltipProps } from "recharts";
import type { NameType, ValueType } from "recharts/types/component/DefaultTooltipContent";
import { cardMotion } from "../motion";
import { formatINR } from "../format";
import type { CostTrendPoint } from "../types";
import { holtForecast } from "../forecast";

// Recharts calls the tooltip formatter for every series present at the
// hovered point, including the two internal band-stacking series
// (bandBase/bandWidth) that only exist to draw the shaded area -- this
// filters those out so the tooltip shows just the two real series.
function trendTooltipContent({ active, payload, label }: TooltipProps<ValueType, NameType>) {
  if (!active || !payload) return null;
  const visible = payload.filter((p) => p.dataKey === "actual" || p.dataKey === "projected");
  if (visible.length === 0) return null;
  return (
    <div
      style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 8, fontSize: 11 }}
      className="px-2 py-1.5"
    >
      <div className="mb-1 text-[var(--ink-muted)]">{label}</div>
      {visible.map((p) => (
        <div key={p.dataKey}>
          {p.dataKey === "actual" ? "Actual" : "Projected"}: {formatINR(Number(p.value ?? 0))}
        </div>
      ))}
    </div>
  );
}

// Holt's linear (double exponential smoothing) trend model over the real
// recent snapshot history -- see forecast.ts for why this method and not
// plain OLS or a seasonal model. Deliberately still not called "Forecast
// (next 7 days)": chidrixx's data is cumulative snapshots at whatever
// cadence agents happen to ship at, not a calendar-day time series, so a
// calendar-bound forecast would be a claim this can't back. The shaded
// band is a real 80% interval computed from the model's own in-sample
// residual error, not a cosmetic margin.
export function TrendProjectionCard({ points }: { points: CostTrendPoint[] }) {
  const n = points.length;
  const result = holtForecast(points, Math.min(n, 10));

  if (!result) {
    return (
      <motion.div
        {...cardMotion}
        className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
      >
        <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Trend projection
        </div>
        <div className="flex flex-1 items-center justify-center text-center text-sm text-[var(--ink-muted)]">
          Needs at least 3 real snapshots to fit a trend model.
        </div>
      </motion.div>
    );
  }

  const ys = points.map((p) => p.CostHigh);
  const lastReal = Math.max(ys[n - 1], 0);

  const chartData: Array<{ label: string; actual: number | null; projected: number | null; bandBase: number | null; bandWidth: number | null }> = points.map((p, i) => ({
    label: new Date(p.ReportedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    actual: p.CostHigh,
    projected: i === n - 1 ? p.CostHigh : null,
    bandBase: null,
    bandWidth: null,
  }));

  for (const f of result.forecast) {
    chartData.push({
      label: `+${f.h}`,
      actual: null,
      projected: f.forecast,
      bandBase: f.lower,
      bandWidth: f.upper - f.lower,
    });
  }

  const last = result.forecast[result.forecast.length - 1];
  const direction = last.forecast > lastReal ? "up" : last.forecast < lastReal ? "down" : "flat";

  return (
    <motion.div
      {...cardMotion}
      className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-1 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
        Trend projection
      </div>
      <div className="mb-2 text-[0.68rem] text-[var(--ink-muted)]">
        Holt's trend model (α={result.alpha.toFixed(1)}, β={result.beta.toFixed(1)}, fit to the last {n} snapshots) —
        not a calendar-day forecast. Shaded band is an 80% interval from the model's own residual error.
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
            <Tooltip content={trendTooltipContent} />
            <Area type="monotone" dataKey="bandBase" stroke="none" fill="transparent" stackId="band" connectNulls />
            <Area
              type="monotone"
              dataKey="bandWidth"
              stroke="none"
              fill="var(--series-orange)"
              fillOpacity={0.12}
              stackId="band"
              connectNulls
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
          Projected: <span className="font-mono tabular-nums">{formatINR(Math.max(last.forecast, 0))}</span>{" "}
          <span className="text-[var(--ink-muted)]">
            ({formatINR(last.lower)}–{formatINR(last.upper)})
          </span>
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
