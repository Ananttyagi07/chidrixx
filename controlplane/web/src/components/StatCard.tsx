import { motion } from "framer-motion";
import { Area, AreaChart, ResponsiveContainer } from "recharts";
import { IconConstruction } from "../icons";
import { cardMotion } from "../motion";

// A stat tile per the dataviz skill's form heuristic. No delta arrow
// ("+12% vs last period") is shown unless we have two real comparable
// snapshots — chidrixx's data model is cumulative-since-agent-start, not
// fixed time windows, so a fabricated percentage would be meaningless.
// `children` carries the (optionally animated) value so callers control
// exactly how the number renders.
export function StatCard({
  label,
  children,
  sub,
  trend,
}: {
  label: string;
  children: React.ReactNode;
  sub?: string;
  trend?: number[];
}) {
  return (
    <motion.div
      {...cardMotion}
      className="group relative flex flex-col justify-between overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)] transition-shadow hover:shadow-[0_4px_24px_rgba(27,175,122,0.12)]"
    >
      <div className="pointer-events-none absolute -right-6 -top-6 h-20 w-20 rounded-full bg-[var(--accent)] opacity-0 blur-2xl transition-opacity duration-500 group-hover:opacity-[0.08]" />
      <div>
        <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">{label}</div>
        <div className="mt-1.5 text-2xl font-semibold tabular-nums">{children}</div>
        {sub && <div className="mt-0.5 text-xs text-[var(--ink-secondary)]">{sub}</div>}
      </div>
      {trend && trend.length > 1 && (
        <div className="mt-2 h-8">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={trend.map((v) => ({ v }))} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
              <defs>
                <linearGradient id={`stat-${label}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--accent)" stopOpacity={0.18} />
                  <stop offset="100%" stopColor="var(--accent)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area
                type="monotone"
                dataKey="v"
                stroke="var(--accent)"
                strokeWidth={2}
                fill={`url(#stat-${label})`}
                dot={false}
                isAnimationActive
                animationDuration={700}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </motion.div>
  );
}

export function StatCardComingSoon({ label }: { label: string }) {
  return (
    <motion.div
      {...cardMotion}
      className="flex flex-col justify-between rounded-2xl border border-dashed border-[var(--border)] bg-[var(--surface)] p-4 opacity-70 shadow-[var(--card-shadow)]"
    >
      <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">{label}</div>
      <div className="mt-2 flex items-center gap-1.5 text-sm text-[var(--ink-muted)]">
        <IconConstruction className="h-4 w-4" />
        Coming soon
      </div>
    </motion.div>
  );
}
