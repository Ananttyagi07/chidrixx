import { motion } from "framer-motion";
import type { DashboardSummary } from "../types";
import { BudgetCard } from "./BudgetCard";
import { AnomalyCard } from "./AnomalyCard";
import { TrendProjectionCard } from "./TrendProjectionCard";
import { PredictiveDriverCard } from "./PredictiveDriverCard";
import { TopFixesTable } from "./TopFixesTable";
import { AnimatedNumber } from "./AnimatedNumber";
import { formatINR } from "../format";

// These four reuse the exact same components/data as their Overview
// counterparts -- the sidebar nav item and the Overview card are two
// views onto the same real feature, not two different claims about what
// exists.
const pageMotion = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0.3 },
};

export function BudgetsPage({ data }: { data: DashboardSummary }) {
  return (
    <motion.div {...pageMotion} className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">Budgets</h2>
      <div className="max-w-md">
        <BudgetCard spentINR={data.summary.TotalCostHighINR} />
      </div>
    </motion.div>
  );
}

export function AnomaliesPage({ data }: { data: DashboardSummary }) {
  return (
    <motion.div {...pageMotion} className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">Anomalies</h2>
      <p className="max-w-xl text-sm text-[var(--ink-muted)]">
        Compares each cluster's two most recent snapshots and flags any whose total cost grew
        2x or more. A cluster needs at least two real snapshots ever ingested before it can
        appear here.
      </p>
      <div className="max-w-md">
        <AnomalyCard anomalies={data.anomalies} />
      </div>
    </motion.div>
  );
}

export function ForecastingPage({ data }: { data: DashboardSummary }) {
  return (
    <motion.div {...pageMotion} className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">Forecasting</h2>
      <p className="max-w-xl text-sm text-[var(--ink-muted)]">
        Holt's linear (double exponential smoothing) trend model, fit over the real recent
        snapshot history with its parameters chosen by minimizing real in-sample forecast error —
        not an arbitrary curve fit. Still not a calendar-day forecast or an ML model: chidrixx's
        data is cumulative snapshots at whatever cadence agents ship at, not a fixed time series.
        The shaded band is a real 80% interval computed from the model's own residual error.
      </p>
      <div className="flex max-w-2xl flex-col gap-3">
        <TrendProjectionCard points={data.trend} />
        <PredictiveDriverCard trend={data.trend} />
      </div>
    </motion.div>
  );
}

export function SavingsAdvisorPage({ data }: { data: DashboardSummary }) {
  const potentialSavings = data.top_fixes.reduce((s, f) => s + f.cost_high_inr, 0);

  return (
    <motion.div {...pageMotion} className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">Savings Advisor</h2>
      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)] max-w-xs">
        <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Potential savings
        </div>
        <AnimatedNumber value={potentialSavings} format={formatINR} className="mt-1.5 text-2xl font-semibold tabular-nums" />
        <div className="mt-0.5 text-xs text-[var(--ink-secondary)]">
          {data.top_fixes.length} flagged flow{data.top_fixes.length === 1 ? "" : "s"} — real cost of
          each path chidrixx flagged as fixable, not an estimate of what fixing it would save.
        </div>
      </div>
      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
        <TopFixesTable
          findings={data.top_fixes}
          emptyMessage="No flagged flows yet — nothing wasteful found (or nothing shipped yet)."
        />
      </div>
    </motion.div>
  );
}
