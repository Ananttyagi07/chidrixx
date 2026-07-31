import { Area, AreaChart, ResponsiveContainer } from "recharts";
import { IconConstruction } from "../icons";

// A stat tile per the dataviz skill's form heuristic. No delta arrow
// ("+12% vs last period") is shown unless we have two real comparable
// snapshots — chidrixx's data model is cumulative-since-agent-start, not
// fixed time windows, so a fabricated percentage would be meaningless.
export function StatCard({
  label,
  value,
  sub,
  trend,
}: {
  label: string;
  value: string;
  sub?: string;
  trend?: number[];
}) {
  return (
    <div className="flex flex-col justify-between rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
      <div>
        <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">{label}</div>
        <div className="mt-1.5 text-2xl font-semibold tabular-nums">{value}</div>
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
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}

export function StatCardComingSoon({ label }: { label: string }) {
  return (
    <div className="flex flex-col justify-between rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 opacity-70 shadow-[var(--card-shadow)]">
      <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">{label}</div>
      <div className="mt-2 flex items-center gap-1.5 text-sm text-[var(--ink-muted)]">
        <IconConstruction className="h-4 w-4" />
        Coming soon
      </div>
    </div>
  );
}
