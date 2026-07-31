import { useEffect, useMemo, useState } from "react";
import { fetchDashboardSummary } from "./api";
import type { DashboardSummary } from "./types";
import { StatCard, StatCardComingSoon } from "./components/StatCard";
import { SpendTrendChart } from "./components/SpendTrendChart";
import { DonutChart, type DonutSlice } from "./components/DonutChart";
import { ClusterCard } from "./components/ClusterCard";
import { TopFixesTable } from "./components/TopFixesTable";
import { ComingSoonCard, ComingSoonPage } from "./components/ComingSoon";
import { Sidebar, NAV_ITEMS } from "./components/Sidebar";
import { Topbar } from "./components/Topbar";
import { CATEGORICAL, PATH_CLASS_COLOR, PATH_CLASS_LABEL, STATUS } from "./palette";
import { formatBytes, formatINR } from "./format";

export default function App() {
  const [data, setData] = useState<DashboardSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState("overview");

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

  const classSlices: DonutSlice[] = useMemo(() => {
    if (!data) return [];
    return data.spend_by_class.map((c) => ({
      key: c.PathClass,
      label: PATH_CLASS_LABEL[c.PathClass] ?? c.PathClass,
      value: c.CostHighINR,
      count: c.FindingCount,
      color: CATEGORICAL[PATH_CLASS_COLOR[c.PathClass] ?? "blue"].light,
    }));
  }, [data]);

  const confidenceSlices: DonutSlice[] = useMemo(() => {
    if (!data) return [];
    const byConf: Record<string, { value: number; count: number }> = {};
    for (const f of data.top_fixes) {
      const c = f.confidence || "unknown";
      byConf[c] ??= { value: 0, count: 0 };
      byConf[c].value += f.cost_high_inr;
      byConf[c].count += 1;
    }
    const order: Array<[string, string, keyof typeof STATUS]> = [
      ["high", "High confidence", "good"],
      ["med", "Medium confidence", "warning"],
      ["low", "Low confidence", "serious"],
    ];
    return order
      .filter(([k]) => byConf[k])
      .map(([k, label, status]) => ({
        key: k,
        label,
        value: byConf[k].value,
        count: byConf[k].count,
        color: STATUS[status].light,
      }));
  }, [data]);

  const potentialSavings = useMemo(() => {
    if (!data) return 0;
    return data.top_fixes.reduce((s, f) => s + f.cost_high_inr, 0);
  }, [data]);

  const navLabel = NAV_ITEMS.find((n) => n.id === active)?.label ?? "Overview";

  return (
    <div className="flex min-h-screen bg-[var(--page)]">
      <Sidebar active={active} onSelect={setActive} />

      <main className="flex-1 p-6">
        <Topbar data={data} />

        {active !== "overview" ? (
          <ComingSoonPage title={navLabel} />
        ) : (
          <>
            {error && (
              <div className="mb-4 rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
                Couldn't load dashboard data: {error}
              </div>
            )}

            {!data && !error && <div className="text-sm text-[var(--ink-muted)]">Loading…</div>}

            {data && (
              <div className="flex flex-col gap-5">
                <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
                  <StatCard
                    label="Total spend"
                    value={`${formatINR(data.summary.TotalCostLowINR)}–${formatINR(data.summary.TotalCostHighINR)}`}
                    trend={data.trend.map((p) => p.CostHigh)}
                  />
                  <StatCard
                    label="Data transferred"
                    value={formatBytes(data.summary.TotalBytesTx + data.summary.TotalBytesRx)}
                  />
                  <StatCard
                    label="Potential savings"
                    value={formatINR(potentialSavings)}
                    sub={data.top_fixes.length > 0 ? `${data.top_fixes.length} flagged flows` : undefined}
                  />
                  <StatCard label="Active workloads" value={String(data.summary.WorkloadCount)} />
                  <StatCardComingSoon label="Carbon footprint" />
                </div>

                <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                  <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
                    <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
                      Spend distribution
                    </div>
                    <DonutChart slices={classSlices} centerLabel="total spend" />
                  </div>

                  <ComingSoonCard
                    title="Multi-cloud topology"
                    note="chidrixx attributes one cluster's network paths today — a cross-cloud view needs multi-provider ingestion this doesn't have yet."
                  />

                  <ComingSoonCard
                    title="Spend by provider"
                    note="Single-cloud price book (AWS) today — no Azure/GCP/OCI attribution."
                  />
                </div>

                <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                  <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 lg:col-span-2 shadow-[var(--card-shadow)]">
                    <div className="mb-1 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
                      Spend trend
                    </div>
                    <SpendTrendChart points={data.trend} />
                  </div>

                  <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
                    <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
                      Spend by confidence
                    </div>
                    <DonutChart slices={confidenceSlices} centerLabel="flagged spend" />
                  </div>
                </div>

                <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                  <ComingSoonCard title="Anomaly detection" note="No cross-snapshot growth comparison in the dashboard yet." />
                  <ComingSoonCard title="Forecast (next 7 days)" note="No forecasting model exists." />
                  <ComingSoonCard title="Budget status" note="No budget-setting feature exists." />
                </div>

                <div id="clusters">
                  <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">Clusters</div>
                  {data.clusters.length === 0 ? (
                    <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] py-8 text-center text-sm text-[var(--ink-muted)] shadow-[var(--card-shadow)]">
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

                <div id="fix-opportunities" className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
                  <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
                    Top fix opportunities
                  </div>
                  <TopFixesTable findings={data.top_fixes} />
                </div>
              </div>
            )}
          </>
        )}
      </main>
    </div>
  );
}
