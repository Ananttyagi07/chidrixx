import { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion, useScroll, useTransform } from "framer-motion";
import { fetchDashboardSummary } from "./api";
import { supabase } from "./supabaseClient";
import { apiFetch } from "./apiFetch";
import type { DashboardSummary } from "./types";
import { StatCard } from "./components/StatCard";
import { SpendTrendChart } from "./components/SpendTrendChart";
import { DonutChart, type DonutSlice } from "./components/DonutChart";
import { ClusterCard } from "./components/ClusterCard";
import { TopFixesTable } from "./components/TopFixesTable";
import { BudgetCard } from "./components/BudgetCard";
import { AnomalyCard } from "./components/AnomalyCard";
import { TrendProjectionCard } from "./components/TrendProjectionCard";
import { CostsUsagePage } from "./components/CostsUsagePage";
import { AnomaliesPage, BudgetsPage, ForecastingPage, SavingsAdvisorPage } from "./components/FeaturePages";
import { WorkloadsPage } from "./components/WorkloadsPage";
import { ExplorerPage } from "./components/ExplorerPage";
import { ReportsPage } from "./components/ReportsPage";
import { InsightsPage } from "./components/InsightsPage";
import { AutomationsPage } from "./components/AutomationsPage";
import { SettingsPage } from "./components/SettingsPage";
import { TeamsPage } from "./components/TeamsPage";
import { HistoryPage } from "./components/HistoryPage";
import { CostGraphPage } from "./components/CostGraphPage";
import { Sidebar } from "./components/Sidebar";
import { Topbar } from "./components/Topbar";
import { LandingPage } from "./components/LandingPage";
import { SectionTitle } from "./components/SectionTitle";
import { AnimatedNumber, AnimatedRange } from "./components/AnimatedNumber";
import { CATEGORICAL, PATH_CLASS_COLOR, PATH_CLASS_LABEL, STATUS } from "./palette";
import { container, item } from "./motion";
import { formatBytes, formatINR } from "./format";
import { ClusterTopologyCard } from "./components/ClusterTopologyCard";
import { SpendByProviderCard } from "./components/SpendByProviderCard";
import { CarbonFootprintCard } from "./components/CarbonFootprintCard";
import { LoginPage } from "./components/LoginPage";
import { SessionContext, type Session } from "./session";

// Pages that need the shared dashboard-summary fetch, keyed by nav id.
// costs/explorer/workloads fetch their own data (the full findings list,
// not the summary) so they're handled as separate branches below.
const DATA_PAGES: Record<string, (data: DashboardSummary) => React.ReactNode> = {
  budgets: (data) => <BudgetsPage data={data} />,
  anomalies: (data) => <AnomaliesPage data={data} />,
  forecasting: (data) => <ForecastingPage data={data} />,
  savings: (data) => <SavingsAdvisorPage data={data} />,
  insights: (data) => <InsightsPage data={data} />,
  reports: (data) => <ReportsPage data={data} />,
  automations: (data) => <AutomationsPage data={data} />,
  settings: (data) => <SettingsPage data={data} />,
  teams: (data) => <TeamsPage data={data} />,
};

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

function Dashboard({ session, onLogout }: { session: Session; onLogout: () => void }) {
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

  // Real re-priced savings (agent/cmd/kharcha/pricing.go's optimizationTarget):
  // the same bytes priced at the realistic cheaper class each fix hint
  // actually describes, not "eliminate this traffic entirely" -- a finding
  // whose real fix is usage reduction (INTERNET_EGRESS) contributes 0 here,
  // not its full cost.
  const potentialSavings = useMemo(() => {
    if (!data) return 0;
    return data.top_fixes.reduce((s, f) => s + f.savings_high_inr, 0);
  }, [data]);

  return (
    <SessionContext.Provider value={session}>
    <div className="relative flex h-screen overflow-hidden bg-[var(--page)]">
      <motion.div
        style={{ y: orb1Y }}
        className="pointer-events-none absolute -left-32 -top-32 h-96 w-96 rounded-full bg-[var(--accent)] opacity-[0.06] blur-3xl"
      />
      <motion.div
        style={{ y: orb2Y }}
        className="pointer-events-none absolute right-0 top-1/3 h-72 w-72 rounded-full bg-[var(--series-blue)] opacity-[0.05] blur-3xl"
      />

      <Sidebar active={active} onSelect={setActive} session={session} onLogout={onLogout} />

      <main ref={scrollRef} className="relative z-10 flex-1 overflow-y-auto p-6">
        <Topbar data={data} />

        <AnimatePresence mode="wait">
          {active === "costs" ? (
            <motion.div key="costs" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <CostsUsagePage />
            </motion.div>
          ) : active === "explorer" ? (
            <motion.div key="explorer" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <ExplorerPage />
            </motion.div>
          ) : active === "workloads" ? (
            <motion.div key="workloads" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <WorkloadsPage />
            </motion.div>
          ) : active === "history" ? (
            <motion.div key="history" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <HistoryPage />
            </motion.div>
          ) : active === "cost-graph" ? (
            <motion.div key="cost-graph" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              <CostGraphPage />
            </motion.div>
          ) : active in DATA_PAGES ? (
            <motion.div key={active} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
              {!data && !error && <Skeleton />}
              {error && (
                <div className="rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
                  Couldn't load dashboard data: {error}
                </div>
              )}
              {data && DATA_PAGES[active](data)}
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
                    <CarbonFootprintCard
                      totalBytes={data.summary.TotalBytesTx + data.summary.TotalBytesRx}
                    />
                  </motion.div>

                  <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                    <Panel title="Spend distribution" scrollContainerRef={scrollRef}>
                      <DonutChart slices={classSlices} centerLabel="total spend" />
                    </Panel>

                    <Panel title="Cluster topology" scrollContainerRef={scrollRef}>
                      <ClusterTopologyCard clusters={data.clusters} />
                    </Panel>

                    <Panel title="Spend by provider" scrollContainerRef={scrollRef}>
                      <SpendByProviderCard clouds={data.spend_by_cloud} />
                    </Panel>
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
    </SessionContext.Provider>
  );
}

export default function App() {
  const [phase, setPhase] = useState<"checking" | "landing" | "login" | "authed">("checking");
  const [session, setSession] = useState<Session | null>(null);

  // Real auth state comes from Supabase (it persists/refreshes the
  // session in localStorage on its own) -- a Supabase session existing is
  // what decides whether to even attempt the control-plane resolve/
  // auto-provision call below, rather than always hitting the API first
  // and inferring auth state from whether it 401s.
  useEffect(() => {
    let cancelled = false;

    async function resolve() {
      const {
        data: { session: supaSession },
      } = await supabase.auth.getSession();

      if (!supaSession) {
        if (!cancelled) setPhase("landing");
        return;
      }

      const res = await apiFetch("/api/v1/auth/me");
      if (cancelled) return;
      if (!res.ok) {
        setPhase("landing");
        return;
      }
      const s: Session = await res.json();
      setSession(s);
      setPhase("authed");
    }

    resolve();

    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange((event) => {
      if (event === "SIGNED_OUT") {
        setSession(null);
        setPhase("landing");
      }
    });

    return () => {
      cancelled = true;
      subscription.unsubscribe();
    };
  }, []);

  function handleLoggedIn(s: Session) {
    setSession(s);
    setPhase("authed");
  }

  async function handleLogout() {
    await supabase.auth.signOut();
    setSession(null);
    setPhase("landing");
  }

  return (
    <AnimatePresence mode="wait">
      {phase === "checking" ? null : phase === "landing" ? (
        <motion.div key="landing" exit={{ opacity: 0 }}>
          <LandingPage onEnter={() => setPhase("login")} />
        </motion.div>
      ) : phase === "login" ? (
        <motion.div key="login" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
          <LoginPage onLoggedIn={handleLoggedIn} />
        </motion.div>
      ) : (
        <motion.div key="dashboard" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
          <Dashboard session={session!} onLogout={handleLogout} />
        </motion.div>
      )}
    </AnimatePresence>
  );
}
