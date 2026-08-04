import { useState } from "react";
import { motion } from "framer-motion";
import {
  IconBell,
  IconBulb,
  IconCalendar,
  IconChat,
  IconFile,
  IconGear,
  IconGrid,
  IconLayers,
  IconReceipt,
  IconSearch,
  IconShareNetwork,
  IconShieldCheck,
  IconTrendingUp,
  IconUsers,
  IconWallet,
} from "../icons";

export interface NavItem {
  id: string;
  label: string;
  icon: (props: { className?: string }) => JSX.Element;
}

interface NavGroup {
  label: string;
  items: NavItem[];
}

// Grouped into constellations rather than one flat 16-item list -- see
// web/DESIGN_VISION.md §4.2. Kept the same real page IDs/labels as the
// previous flat Sidebar so nothing that depends on them (routing,
// existing E2E selectors) has to change in this pass; only the shape and
// organization of the rail itself changed.
export const NAV_GROUPS: NavGroup[] = [
  {
    label: "Observe",
    items: [
      { id: "overview", label: "Overview", icon: IconGrid },
      { id: "cost-graph", label: "Cost Graph", icon: IconShareNetwork },
      { id: "workloads", label: "Workloads", icon: IconLayers },
      { id: "costs", label: "Costs & Usage", icon: IconReceipt },
    ],
  },
  {
    label: "Understand",
    items: [
      { id: "assistant", label: "Assistant", icon: IconChat },
      { id: "insights", label: "Insights", icon: IconBulb },
      { id: "explorer", label: "Explorer", icon: IconSearch },
      { id: "forecasting", label: "Forecasting", icon: IconTrendingUp },
      { id: "anomalies", label: "Anomalies", icon: IconBell },
      { id: "history", label: "History", icon: IconCalendar },
    ],
  },
  {
    label: "Act",
    items: [
      { id: "automations", label: "Automations", icon: IconGear },
      { id: "savings", label: "Savings Advisor", icon: IconShieldCheck },
      { id: "budgets", label: "Budgets", icon: IconWallet },
    ],
  },
  {
    label: "Organize",
    items: [
      { id: "teams", label: "Teams", icon: IconUsers },
      { id: "reports", label: "Reports", icon: IconFile },
    ],
  },
];

export function CommandRail({ active, onSelect }: { active: string; onSelect: (id: string) => void }) {
  const [pinned, setPinned] = useState(true);

  return (
    <motion.aside
      animate={{ width: pinned ? 232 : 72 }}
      transition={{ type: "spring", stiffness: 420, damping: 38 }}
      className="sticky top-0 flex h-screen flex-shrink-0 flex-col justify-between overflow-hidden border-r border-[var(--border)] bg-[var(--surface)] py-4"
    >
      <nav className="flex flex-col gap-5 overflow-y-auto px-3">
        {NAV_GROUPS.map((group) => (
          <div key={group.label} className="flex flex-col gap-0.5">
            <div
              className={`px-2 pb-1 text-[0.65rem] font-medium uppercase tracking-[0.08em] text-[var(--ink-muted)] transition-opacity ${
                pinned ? "opacity-100" : "opacity-0"
              }`}
            >
              {group.label}
            </div>
            {group.items.map((navItem) => {
              const Icon = navItem.icon;
              const isActive = active === navItem.id;
              return (
                <button
                  key={navItem.id}
                  onClick={() => onSelect(navItem.id)}
                  title={navItem.label}
                  className="group relative flex h-11 items-center gap-3 rounded-lg px-2.5 text-left text-sm transition-colors hover:bg-[var(--surface-sunken)]"
                >
                  {isActive && (
                    <motion.span
                      layoutId="rail-active-bar"
                      className="absolute -left-3 top-1/2 h-5 w-[2px] -translate-y-1/2 bg-[var(--ink)]"
                      transition={{ type: "spring", stiffness: 420, damping: 38 }}
                    />
                  )}
                  <Icon
                    className={`h-[18px] w-[18px] flex-shrink-0 ${isActive ? "text-[var(--ink)]" : "text-[var(--ink-faint)] group-hover:text-[var(--ink-secondary)]"}`}
                  />
                  <span
                    className={`whitespace-nowrap transition-opacity ${
                      isActive ? "font-medium text-[var(--ink)]" : "text-[var(--ink-secondary)]"
                    } ${pinned ? "opacity-100" : "opacity-0"}`}
                  >
                    {navItem.label}
                  </span>
                </button>
              );
            })}
          </div>
        ))}
      </nav>

      <div className="flex flex-col gap-0.5 border-t border-[var(--border)] px-3 pt-3">
        <button
          onClick={() => onSelect("settings")}
          title="Settings"
          className="group relative flex h-11 items-center gap-3 rounded-lg px-2.5 text-left text-sm transition-colors hover:bg-[var(--surface-sunken)]"
        >
          {active === "settings" && (
            <motion.span
              layoutId="rail-active-bar"
              className="absolute -left-3 top-1/2 h-5 w-[2px] -translate-y-1/2 bg-[var(--ink)]"
              transition={{ type: "spring", stiffness: 420, damping: 38 }}
            />
          )}
          <IconGear
            className={`h-[18px] w-[18px] flex-shrink-0 ${active === "settings" ? "text-[var(--ink)]" : "text-[var(--ink-muted)] group-hover:text-[var(--ink-secondary)]"}`}
          />
          <span className={`whitespace-nowrap transition-opacity ${pinned ? "opacity-100" : "opacity-0"}`}>Settings</span>
        </button>

        <button
          onClick={() => setPinned((p) => !p)}
          title={pinned ? "Collapse rail" : "Expand rail"}
          className="flex h-11 items-center gap-3 rounded-lg px-2.5 text-left text-sm text-[var(--ink-muted)] transition-colors hover:bg-[var(--surface-sunken)] hover:text-[var(--ink-secondary)]"
        >
          <svg viewBox="0 0 20 20" className="h-[18px] w-[18px] flex-shrink-0" fill="none" stroke="currentColor" strokeWidth={1.6}>
            {pinned ? (
              <path d="M13 4l-6 6 6 6" strokeLinecap="round" strokeLinejoin="round" />
            ) : (
              <path d="M7 4l6 6-6 6" strokeLinecap="round" strokeLinejoin="round" />
            )}
          </svg>
          <span className={`whitespace-nowrap transition-opacity ${pinned ? "opacity-100" : "opacity-0"}`}>Collapse</span>
        </button>
      </div>
    </motion.aside>
  );
}
