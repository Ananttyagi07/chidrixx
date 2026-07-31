import { useEffect, useState } from "react";
import { fetchDashboardSummary } from "./api";
import type { DashboardSummary } from "./types";
import { StatCard } from "./components/StatCard";
import { SpendTrendChart } from "./components/SpendTrendChart";
import { SpendByClassBar } from "./components/SpendByClassBar";
import { ClusterCard } from "./components/ClusterCard";
import { TopFixesTable } from "./components/TopFixesTable";
import { formatBytes, formatINR } from "./format";

const NAV = [
  { label: "Overview", id: "overview" },
  { label: "Clusters", id: "clusters" },
  { label: "Fix opportunities", id: "fix-opportunities" },
] as const;

export default function App() {
  const [data, setData] = useState<DashboardSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState<(typeof NAV)[number]["id"]>("overview");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const d = await fetchDashboardSummary();
        if (!cancelled) {
          setData(d);
          setError(null);
        }
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
    }

    load();
    const id = setInterval(load, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  return (
    <div className="flex min-h-screen">
      <aside className="flex w-56 flex-shrink-0 flex-col gap-6 border-r border-[var(--border)] bg-[var(--surface)] p-5">
        <div>
          <div className="font-mono text-lg font-semibold tracking-tight">chidrixx</div>
          <div className="mt-0.5 text-xs text-[var(--ink-muted)]">Network cost attribution</div>
        </div>
        <nav className="flex flex-col gap-1">
          {NAV.map((item) => (
            <button
              key={item.id}
              onClick={() => {
                setActive(item.id);
                document.getElementById(item.id)?.scrollIntoView({ behavior: "smooth", block: "start" });
              }}
              className={`rounded-md px-3 py-1.5 text-left text-sm ${
                active === item.id
                  ? "bg-[var(--series-blue)]/10 font-medium text-[var(--series-blue)]"
                  : "text-[var(--ink-secondary)] hover:bg-[var(--page)]"
              }`}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      <main className="flex-1 p-6">
        <header className="mb-5 flex items-baseline justify-between">
          <h1 id="overview" className="text-xl font-semibold">
            {NAV.find((n) => n.id === active)?.label}
          </h1>
          {data && (
            <span className="text-xs text-[var(--ink-muted)]">
              {data.summary.ClusterCount} cluster{data.summary.ClusterCount === 1 ? "" : "s"} · refreshes every 15s
            </span>
          )}
        </header>

        {error && (
          <div className="mb-4 rounded-md border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
            Couldn't load dashboard data: {error}
          </div>
        )}

        {!data && !error && <div className="text-sm text-[var(--ink-muted)]">Loading…</div>}

        {data && (
          <div className="flex flex-col gap-6">
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <StatCard
                label="TOTAL SPEND"
                value={`${formatINR(data.summary.TotalCostLowINR)}–${formatINR(data.summary.TotalCostHighINR)}`}
              />
              <StatCard
                label="DATA TRANSFERRED"
                value={formatBytes(data.summary.TotalBytesTx + data.summary.TotalBytesRx)}
              />
              <StatCard label="ACTIVE WORKLOADS" value={String(data.summary.WorkloadCount)} />
              <StatCard label="CLUSTERS" value={String(data.summary.ClusterCount)} />
            </div>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
              <div className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4 lg:col-span-2">
                <div className="mb-1 font-mono text-xs uppercase tracking-wide text-[var(--ink-muted)]">
                  Spend trend
                </div>
                <SpendTrendChart points={data.trend} />
              </div>
              <div className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
                <div className="mb-3 font-mono text-xs uppercase tracking-wide text-[var(--ink-muted)]">
                  Spend by path class
                </div>
                <SpendByClassBar classes={data.spend_by_class} />
              </div>
            </div>

            <div id="clusters">
              <div className="mb-3 font-mono text-xs uppercase tracking-wide text-[var(--ink-muted)]">
                Clusters
              </div>
              {data.clusters.length === 0 ? (
                <div className="rounded-lg border border-[var(--border)] bg-[var(--surface)] py-8 text-center text-sm text-[var(--ink-muted)]">
                  No clusters have shipped data yet.
                </div>
              ) : (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {data.clusters.map((c) => (
                    <ClusterCard key={c.ClusterID} cluster={c} />
                  ))}
                </div>
              )}
            </div>

            <div id="fix-opportunities" className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4">
              <div className="mb-3 font-mono text-xs uppercase tracking-wide text-[var(--ink-muted)]">
                Top fix opportunities
              </div>
              <TopFixesTable findings={data.top_fixes} />
            </div>
          </div>
        )}
      </main>
    </div>
  );
}
