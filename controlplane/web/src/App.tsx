import { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion, useScroll, useTransform } from "framer-motion";
import { fetchDashboardSummary } from "./api";
import type { DashboardSummary } from "./types";
import { StatCard, StatCardComingSoon } from "./components/StatCard";
import { SpendTrendChart } from "./components/SpendTrendChart";
import { DonutChart, type DonutSlice } from "./components/DonutChart";
import { ClusterCard } from "./components/ClusterCard";
import { TopFixesTable } from "./components/TopFixesTable";
import { ComingSoonCard, ComingSoonPage } from "./components/ComingSoon";
import { BudgetCard } from "./components/BudgetCard";
import { AnomalyCard } from "./components/AnomalyCard";
import { TrendProjectionCard } from "./components/TrendProjectionCard";
import { CostsUsagePage } from "./components/CostsUsagePage";
import { Sidebar, NAV_ITEMS } from "./components/Sidebar";
import { Topbar } from "./components/Topbar";
import { LandingPage } from "./components/LandingPage";
import { SectionTitle } from "./components/SectionTitle";
import { AnimatedNumber, AnimatedRange } from "./components/AnimatedNumber";
import { CATEGORICAL, PATH_CLASS_COLOR, PATH_CLASS_LABEL, STATUS } from "./palette";
import { container, item } from "./motion";
import { formatBytes, formatINR } from "./format";

function Panel({
  title,
  children,
  scrollContainerRef,
}: {
  title: string;
  children: React.ReactNode;
  scrollContainerRef: React.RefObject<HTMLElement>;
}) {
  return (
    <motion.div
      variants={item}
      className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-3">
        <SectionTitle scrollContainerRef={scrollContainerRef}>{title}</SectionTitle>
      </div>
      {children}
    </motion.div>
  );
}

function SkeletonCard({ className = "" }: { className?: string }) {
  return (
    <div className={`animate-pulse rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)] ${className}`}>
      <div className="h-3 w-24 rounded bg-[var(--surface-sunken)]" />
      <div className="mt-3 h-6 w-32 rounded bg-[var(--surface-sunken)]" />
    </div>
  );
}

function Skeleton() {
  return (
    <div className="flex flex-col gap-5">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <SkeletonCard className="h-40" />
        <SkeletonCard className="h-40" />
        <SkeletonCard className="h-40" />
      </div>
    </div>
  );
}

function Dashboard() {
  const [data, setData] = useState<DashboardSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState("overview");
  const scrollRef = useRef<HTMLDivElement>(null);
  const { scrollYProgress } = useScroll({ container: scrollRef });
  const orb1Y = useTransform(scrollYProgress, [0, 1], [0, 220]);
  const orb2Y = useTransform(scrollYProgress, [0, 1], [0, -160]);

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
    <div className="relative flex h-screen overflow-hidden bg-[var(--page)]">
      <motion.div
        style={{ y: orb1Y }}
        className="pointer-events-none absolute -left-32 -top-32 h-96 w-96 rounded-full bg-[var(--accent)] opacity-[0.06] blur-3xl"
      />
      <motion.div
        style={{ y: orb2Y }}
        className="pointer-events-none absolute right-0 top-1/3 h-72 w-72 rounded-full bg-[var(--series-blue)] opacity-[0.05] blur-3xl"
      />

      <Sidebar active={active} onSelect={setActive} />

      <main ref={scrollRef} className="relative z-10 flex-1 overflow-y-auto p-6">
        <Topbar data={data} />

        <AnimatePresence mode="wait">
          {active === "costs" ? (
            <motion.div key="costs" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <CostsUsagePage />
            </motion.div>
          ) : active !== "overview" ? (
            <motion.div key="soon" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <ComingSoonPage title={navLabel} />
            </motion.div>
          ) : (
            <motion.div key="overview" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              {error && (
                <div className="mb-4 rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
                  Couldn't load dashboard data: {error}
                </div>
              )}

              {!data && !error && <Skeleton />}

              {data && (
                <motion.div variants={container} initial="hidden" animate="show" className="flex flex-col gap-5">
                  <motion.div variants={item} className="grid grid-cols-2 gap-3 lg:grid-cols-5">
                    <StatCard label="Total spend" trend={data.trend.map((p) => p.CostHigh)}>
                      <AnimatedRange
                        low={data.summary.TotalCostLowINR}
                        high={data.summary.TotalCostHighINR}
                        format={(lo, hi) => `${formatINR(lo)}–${formatINR(hi)}`}
                      />
                    </StatCard>
                    <StatCard label="Data transferred">
                      <AnimatedNumber
                        value={data.summary.TotalBytesTx + data.summary.TotalBytesRx}
                        format={formatBytes}
                      />
                    </StatCard>
                    <StatCard
                      label="Potential savings"
                      sub={data.top_fixes.length > 0 ? `${data.top_fixes.length} flagged flows` : undefined}
                    >
                      <AnimatedNumber value={potentialSavings} format={formatINR} />
                    </StatCard>
                    <StatCard label="Active workloads">
                      <AnimatedNumber value={data.summary.WorkloadCount} format={(n) => String(Math.round(n))} />
                    </StatCard>
                    <StatCardComingSoon label="Carbon footprint" />
                  </motion.div>

                  <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                    <Panel title="Spend distribution" scrollContainerRef={scrollRef}>
                      <DonutChart slices={classSlices} centerLabel="total spend" />
                    </Panel>

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
                    <motion.div
                      variants={item}
                      className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)] lg:col-span-2"
                    >
                      <div className="mb-1">
                        <SectionTitle scrollContainerRef={scrollRef}>Spend trend</SectionTitle>
                      </div>
                      <SpendTrendChart points={data.trend} />
                    </motion.div>

                    <Panel title="Spend by confidence" scrollContainerRef={scrollRef}>
                      <DonutChart slices={confidenceSlices} centerLabel="flagged spend" />
                    </Panel>
                  </div>

                  <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                    <AnomalyCard anomalies={data.anomalies} />
                    <TrendProjectionCard points={data.trend} />
                    <BudgetCard spentINR={data.summary.TotalCostHighINR} />
                  </div>

                  <div id="clusters">
                    <div className="mb-3">
                      <SectionTitle scrollContainerRef={scrollRef}>Clusters</SectionTitle>
                    </div>
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

                  <Panel title="Top fix opportunities" scrollContainerRef={scrollRef}>
                    <div id="fix-opportunities" />
                    <TopFixesTable findings={data.top_fixes} />
                  </Panel>
                </motion.div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </main>
    </div>
  );
}

export default function App() {
  const [entered, setEntered] = useState(false);

  return (
    <AnimatePresence mode="wait">
      {!entered ? (
        <motion.div key="landing" exit={{ opacity: 0 }}>
          <LandingPage onEnter={() => setEntered(true)} />
        </motion.div>
      ) : (
        <motion.div key="dashboard" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
          <Dashboard />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
