import { type ComponentType } from "react";
import { motion } from "framer-motion";
import DecryptedTextRaw from "./DecryptedText";
import {
  IconBell,
  IconBulb,
  IconFile,
  IconGear,
  IconGrid,
  IconLayers,
  IconReceipt,
  IconSearch,
  IconShieldCheck,
  IconTrendingUp,
  IconWallet,
} from "../icons";

const DecryptedText = DecryptedTextRaw as ComponentType<any>;

export interface NavItem {
  id: string;
  label: string;
  icon: (props: { className?: string }) => JSX.Element;
  real: boolean;
}

export const NAV_ITEMS: NavItem[] = [
  { id: "overview", label: "Overview", icon: IconGrid, real: true },
  { id: "insights", label: "Insights", icon: IconBulb, real: true },
  { id: "explorer", label: "Explorer", icon: IconSearch, real: true },
  { id: "workloads", label: "Workloads", icon: IconLayers, real: true },
  { id: "costs", label: "Costs & Usage", icon: IconReceipt, real: true },
  { id: "budgets", label: "Budgets", icon: IconWallet, real: true },
  { id: "savings", label: "Savings Advisor", icon: IconShieldCheck, real: true },
  { id: "forecasting", label: "Forecasting", icon: IconTrendingUp, real: true },
  { id: "anomalies", label: "Anomalies", icon: IconBell, real: true },
  { id: "reports", label: "Reports", icon: IconFile, real: true },
  { id: "automations", label: "Automations", icon: IconGear, real: true },
  { id: "settings", label: "Settings", icon: IconGear, real: true },
];

export function Sidebar({ active, onSelect }: { active: string; onSelect: (id: string) => void }) {
  return (
    <aside className="sticky top-0 flex h-screen w-60 flex-shrink-0 flex-col justify-between overflow-y-auto border-r border-[var(--border)] bg-[var(--surface)] p-4">
      <div>
        <div className="mb-6 flex items-center gap-2 px-2 pt-1">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-[var(--accent-wash)]">
            <svg viewBox="0 0 20 20" className="h-5 w-5 text-[var(--accent)]" fill="none" stroke="currentColor" strokeWidth={1.8}>
              <path d="M10 2l7 3.7v8.6L10 18l-7-3.7V5.7L10 2z" strokeLinejoin="round" />
              <path d="M10 2v16M3 5.7l7 3.7 7-3.7" strokeLinejoin="round" />
            </svg>
          </div>
          <div>
            <div className="font-mono text-base font-semibold leading-tight tracking-tight">
              <DecryptedText
                text="chidrixx"
                animateOn="view"
                sequential
                speed={35}
                className="text-[var(--ink)]"
                encryptedClassName="text-[var(--ink-muted)]"
              />
            </div>
            <div className="text-[0.68rem] leading-tight text-[var(--ink-muted)]">Network cost attribution</div>
          </div>
        </div>

        <nav className="flex flex-col gap-0.5">
          {NAV_ITEMS.map((item) => {
            const Icon = item.icon;
            const isActive = active === item.id;
            return (
              <button
                key={item.id}
                onClick={() => onSelect(item.id)}
                className={`relative flex items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                  isActive ? "font-medium text-[var(--accent)]" : "text-[var(--ink-secondary)] hover:bg-[var(--surface-sunken)]"
                }`}
              >
                {isActive && (
                  <motion.span
                    layoutId="nav-active-pill"
                    className="absolute inset-0 rounded-lg bg-[var(--accent-wash)]"
                    transition={{ type: "spring", stiffness: 400, damping: 32 }}
                  />
                )}
                <Icon className={`relative z-10 h-4 w-4 flex-shrink-0 ${isActive ? "text-[var(--accent)]" : "text-[var(--ink-muted)]"}`} />
                <span className="relative z-10">{item.label}</span>
                {!item.real && (
                  <span className="relative z-10 ml-auto rounded-full bg-[var(--surface-sunken)] px-1.5 py-0.5 text-[0.6rem] text-[var(--ink-muted)]">
                    soon
                  </span>
                )}
              </button>
            );
          })}
        </nav>
      </div>

      <div className="flex flex-col gap-3">
        <div className="rounded-xl border border-dashed border-[var(--border)] p-3">
          <div className="text-xs font-medium text-[var(--ink-secondary)]">Plan status</div>
          <div className="mt-1 text-xs text-[var(--ink-muted)]">Coming soon — no billing/subscription system exists yet.</div>
        </div>
        <div className="flex items-center gap-2 rounded-xl px-1 py-1">
          <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[var(--accent-wash)] text-xs font-semibold text-[var(--accent)]">
            AD
          </div>
          <div className="min-w-0">
            <div className="truncate text-xs font-medium">Admin</div>
            <div className="truncate text-[0.68rem] text-[var(--ink-muted)]">Shared token access</div>
          </div>
        </div>
      </div>
    </aside>
  );
}
