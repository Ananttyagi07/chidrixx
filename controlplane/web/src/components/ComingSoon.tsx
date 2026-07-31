import { IconConstruction } from "../icons";

// Used for any widget/page whose underlying feature genuinely doesn't
// exist yet (multi-cloud attribution, forecasting, budgets, anomaly
// detection, automations...) — shown honestly rather than filled with
// invented numbers. Matches the real widget's card footprint so the
// layout holds its shape.
export function ComingSoonCard({ title, note }: { title: string; note?: string }) {
  return (
    <div className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
      <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">{title}</div>
      <div className="flex flex-1 flex-col items-center justify-center gap-2 py-6 text-center">
        <IconConstruction className="h-6 w-6 text-[var(--ink-muted)]" />
        <div className="text-sm font-medium text-[var(--ink-secondary)]">Coming soon</div>
        {note && <div className="max-w-[16rem] text-xs text-[var(--ink-muted)]">{note}</div>}
      </div>
    </div>
  );
}

export function ComingSoonPage({ title }: { title: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-[var(--border)] bg-[var(--surface)] py-24 text-center shadow-[var(--card-shadow)]">
      <IconConstruction className="h-8 w-8 text-[var(--ink-muted)]" />
      <div className="text-lg font-semibold">{title}</div>
      <div className="max-w-sm text-sm text-[var(--ink-muted)]">
        This page isn't built yet — Overview is the only real page chidrixx has today.
      </div>
    </div>
  );
}
