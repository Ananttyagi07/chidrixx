// A stat tile per the dataviz skill's form heuristic ("a single current
// value" -> stat tile, not a one-bar bar chart). No delta/trend arrow is
// shown unless we have a real prior value to compare against — this is
// the current cumulative-since-start snapshot, not a windowed metric, so
// a fabricated "+12% vs last period" would be meaningless here.
export function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--surface)] px-4 py-3.5 border-l-[3px] border-l-[var(--series-blue)]">
      <div className="font-mono text-[0.72rem] tracking-wide text-[var(--ink-muted)]">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      {sub && <div className="mt-0.5 text-xs text-[var(--ink-secondary)]">{sub}</div>}
    </div>
  );
}
