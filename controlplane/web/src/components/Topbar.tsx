import { useState } from "react";
import { IconCalendar, IconDownload, IconFilter, IconSun } from "../icons";
import type { DashboardSummary } from "../types";

function greeting(): string {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 17) return "Good afternoon";
  return "Good evening";
}

export function Topbar({ data }: { data: DashboardSummary | null }) {
  const [soonMsg, setSoonMsg] = useState<string | null>(null);

  function showSoon(label: string) {
    setSoonMsg(label);
    window.setTimeout(() => setSoonMsg(null), 2000);
  }

  function downloadSnapshot() {
    if (!data) return;
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `chidrixx-dashboard-${new Date().toISOString().slice(0, 19)}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const today = new Date().toLocaleDateString(undefined, { day: "2-digit", month: "short", year: "numeric" });

  return (
    <header className="mb-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            {greeting()}
            <IconSun className="h-5 w-5 text-[var(--accent)]" />
          </h1>
          <p className="mt-0.5 text-sm text-[var(--ink-muted)]">
            Real-time network cost attribution across{" "}
            {data ? `${data.summary.ClusterCount} cluster${data.summary.ClusterCount === 1 ? "" : "s"}` : "your clusters"}.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => showSoon("Custom date ranges — coming soon (data is cumulative-since-agent-start today)")}
            className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]"
          >
            <IconCalendar className="h-3.5 w-3.5" />
            {today}
          </button>
          <span className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />
            Live · refreshes every 15s
          </span>
          <button
            onClick={() => showSoon("Dimension filters — coming soon")}
            className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]"
          >
            <IconFilter className="h-3.5 w-3.5" />
            Filters
          </button>
          <button
            onClick={downloadSnapshot}
            disabled={!data}
            title="Download this snapshot as JSON"
            className="flex items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--surface)] p-1.5 text-[var(--ink-secondary)] shadow-[var(--card-shadow)] disabled:opacity-40"
          >
            <IconDownload className="h-4 w-4" />
          </button>
        </div>
      </div>

      {soonMsg && (
        <div className="mt-2 inline-flex items-center gap-2 rounded-lg bg-[var(--surface-sunken)] px-3 py-1.5 text-xs text-[var(--ink-secondary)]">
          {soonMsg}
        </div>
      )}
    </header>
  );
}
