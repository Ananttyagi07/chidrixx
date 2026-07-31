import { Area, AreaChart, ResponsiveContainer } from "recharts";
import type { ClusterSummaryView } from "../types";
import { formatINR, relativeTime } from "../format";

export function ClusterCard({ cluster }: { cluster: ClusterSummaryView }) {
  const data = cluster.trend.map((p) => ({ cost: p.CostHigh }));

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-mono text-sm font-semibold">{cluster.ClusterID}</span>
        <span className="whitespace-nowrap text-xs text-[var(--ink-muted)]">
          {relativeTime(cluster.LastSeen)}
        </span>
      </div>
      <div className="text-lg font-semibold tabular-nums text-[var(--ink)]">
        {formatINR(cluster.CostHighINR)}
      </div>
      <div className="text-xs text-[var(--ink-secondary)]">{cluster.FindingCount} findings</div>
      {data.length > 1 && (
        <div className="h-8">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 2, right: 0, left: 0, bottom: 0 }}>
              <defs>
                <linearGradient id={`spark-${cluster.ClusterID}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--series-blue)" stopOpacity={0.15} />
                  <stop offset="100%" stopColor="var(--series-blue)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area
                type="monotone"
                dataKey="cost"
                stroke="var(--series-blue)"
                strokeWidth={2}
                fill={`url(#spark-${cluster.ClusterID})`}
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
