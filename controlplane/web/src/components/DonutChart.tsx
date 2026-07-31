import { motion } from "framer-motion";
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import { formatINR } from "../format";
import { AnimatedNumber } from "./AnimatedNumber";

export interface DonutSlice {
  key: string;
  label: string;
  value: number;
  count: number;
  color: string;
}

// Part-to-whole donut with a real center total and a direct-labeled
// legend (percentages computed from real values, never invented).
export function DonutChart({ slices, centerLabel }: { slices: DonutSlice[]; centerLabel: string }) {
  const total = slices.reduce((s, x) => s + x.value, 0);
  const nonZero = slices.filter((s) => s.value > 0);

  if (nonZero.length === 0) {
    return <div className="py-8 text-center text-sm text-[var(--ink-muted)]">No findings yet.</div>;
  }

  return (
    <div className="flex items-center gap-4">
      <div className="relative h-36 w-36 flex-shrink-0">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={nonZero}
              dataKey="value"
              nameKey="label"
              innerRadius="68%"
              outerRadius="100%"
              paddingAngle={2}
              stroke="var(--surface)"
              strokeWidth={2}
              isAnimationActive
              animationDuration={800}
              animationEasing="ease-out"
            >
              {nonZero.map((s) => (
                <Cell key={s.key} fill={s.color} />
              ))}
            </Pie>
            <Tooltip
              contentStyle={{
                background: "var(--surface)",
                border: "1px solid var(--border)",
                borderRadius: 8,
                fontSize: 12,
              }}
              formatter={(v: number, _n, entry) => [formatINR(v), entry.payload.label]}
            />
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <AnimatedNumber value={total} format={formatINR} className="text-base font-semibold tabular-nums" />
          <div className="text-[0.65rem] text-[var(--ink-muted)]">{centerLabel}</div>
        </div>
      </div>
      <div className="flex flex-1 flex-col gap-1.5">
        {slices.map((s, i) => (
          <motion.div
            key={s.key}
            initial={{ opacity: 0, x: 8 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: i * 0.05, duration: 0.3 }}
            className="flex items-center justify-between gap-2 text-xs"
          >
            <span className="flex items-center gap-1.5 text-[var(--ink-secondary)]">
              <span className="inline-block h-2 w-2 rounded-full" style={{ background: s.color }} />
              {s.label}
            </span>
            <span className="font-mono tabular-nums text-[var(--ink)]">
              {total > 0 ? ((s.value / total) * 100).toFixed(1) : "0.0"}%
            </span>
          </motion.div>
        ))}
      </div>
    </div>
  );
}
