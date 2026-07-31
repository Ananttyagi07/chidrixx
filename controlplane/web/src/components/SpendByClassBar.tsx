import type { ClassSpend } from "../types";
import { CATEGORICAL, PATH_CLASS_COLOR, PATH_CLASS_LABEL } from "../palette";
import { formatINR } from "../format";

// Part-to-whole with up to 8 named categories -> horizontal bar list, per
// the form heuristic (a donut buries long class names and doesn't scale
// past a few slices). Each row is direct-labeled with its own name+value
// since this is a compact list, not a dense scatter — the "never a number
// on every point" rule targets flooding a chart with per-point labels,
// not a handful of named bar rows.
export function SpendByClassBar({ classes }: { classes: ClassSpend[] }) {
  if (classes.length === 0) {
    return <div className="py-8 text-center text-sm text-[var(--ink-muted)]">No findings yet.</div>;
  }

  const max = Math.max(...classes.map((c) => c.CostHighINR), 0.0001);

  return (
    <div className="flex flex-col gap-2.5">
      {classes.map((c) => {
        const slot = PATH_CLASS_COLOR[c.PathClass] ?? "blue";
        const pct = Math.max((c.CostHighINR / max) * 100, 2);
        return (
          <div key={c.PathClass}>
            <div className="mb-1 flex items-baseline justify-between text-xs">
              <span className="flex items-center gap-1.5 text-[var(--ink-secondary)]">
                <span
                  className="inline-block h-2 w-2 rounded-full"
                  style={{ background: CATEGORICAL[slot].light }}
                />
                {PATH_CLASS_LABEL[c.PathClass] ?? c.PathClass}
                <span className="text-[var(--ink-muted)]">· {c.FindingCount}</span>
              </span>
              <span className="font-mono tabular-nums text-[var(--ink)]">{formatINR(c.CostHighINR)}</span>
            </div>
            <div className="h-2 rounded-full bg-[var(--gridline)]">
              <div
                className="h-2 rounded-full"
                style={{ width: `${pct}%`, background: `var(--series-${slot})` }}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}
