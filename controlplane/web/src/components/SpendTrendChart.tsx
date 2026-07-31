import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { CostTrendPoint } from "../types";
import { formatINR } from "../format";

// Single series -> sequential blue, 2px line, ~10% opacity area fill,
// hairline recessive gridlines, no legend box (a single series needs
// none — the card title already names it).
export function SpendTrendChart({ points }: { points: CostTrendPoint[] }) {
  if (points.length === 0) {
    return <EmptyChart label="No trend data yet — needs at least one ingest snapshot." />;
  }

  const data = points.map((p) => ({
    t: new Date(p.ReportedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    cost: p.CostHigh,
  }));

  return (
    <ResponsiveContainer width="100%" height={220}>
      <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id="trendFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--series-blue)" stopOpacity={0.1} />
            <stop offset="100%" stopColor="var(--series-blue)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <XAxis
          dataKey="t"
          tick={{ fill: "var(--ink-muted)", fontSize: 11 }}
          axisLine={{ stroke: "var(--baseline)" }}
          tickLine={false}
        />
        <YAxis
          tick={{ fill: "var(--ink-muted)", fontSize: 11 }}
          axisLine={false}
          tickLine={false}
          width={56}
          tickFormatter={(v) => formatINR(v)}
        />
        <Tooltip
          contentStyle={{
            background: "var(--surface)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            fontSize: 12,
          }}
          labelStyle={{ color: "var(--ink-secondary)" }}
          formatter={(v: number) => [formatINR(v), "Cost (high estimate)"]}
        />
        <Area
          type="monotone"
          dataKey="cost"
          stroke="var(--series-blue)"
          strokeWidth={2}
          fill="url(#trendFill)"
          dot={false}
          activeDot={{ r: 4, strokeWidth: 2, stroke: "var(--surface)" }}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

export function EmptyChart({ label }: { label: string }) {
  return (
    <div className="flex h-[220px] items-center justify-center text-sm text-[var(--ink-muted)]">
      {label}
    </div>
  );
}
