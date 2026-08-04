import { motion } from "framer-motion";
import type { DashboardSummary } from "../types";
import { useSession } from "../session";

// Real, minimal, honest -- no fake toggles for capabilities that don't
// exist (no theme switch, since light is the enforced default by
// deliberate choice; no notification prefs, since there's no
// notification system beyond the agent's own webhook alerting).
export function SettingsPage({ data }: { data: DashboardSummary }) {
  const { username, role } = useSession();

  const rows: Array<[string, string]> = [
    ["Signed in as", `${username} (${role})`],
    ["Authentication", "Per-tenant login (bcrypt password, server-tracked session) — real multi-tenant isolation"],
    ["Storage", "SQLite (embedded, pure Go)"],
    ["Clusters connected", String(data.summary.ClusterCount)],
    ["Workloads tracked", String(data.summary.WorkloadCount)],
    ["Findings in latest snapshots", String(data.summary.FindingCount)],
    ["Dashboard refresh interval", "15 seconds"],
  ];

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">Settings</h2>

      <div className="max-w-lg rounded-2xl border border-[var(--border)] bg-[var(--surface)] shadow-[var(--card-shadow)]">
        {rows.map(([label, value], i) => (
          <div
            key={label}
            className={`flex items-center justify-between px-4 py-3 text-sm ${
              i !== rows.length - 1 ? "border-b border-[var(--border)]" : ""
            }`}
          >
            <span className="text-[var(--ink-secondary)]">{label}</span>
            <span className="font-mono text-xs tabular-nums">{value}</span>
          </div>
        ))}
      </div>

      <p className="max-w-md text-xs text-[var(--ink-muted)]">
        {role === "admin"
          ? "As an admin on this tenant, you can set the budget figure. Provisioning new tenants is an operator action (the create-tenant CLI subcommand), not a self-service signup flow."
          : "As a viewer on this tenant, you can see everything but can't change the budget figure — ask an admin on your tenant."}
      </p>

      <div className="max-w-lg rounded-2xl border border-dashed border-[var(--border)] p-3">
        <div className="text-xs font-medium text-[var(--ink-secondary)]">Plan status</div>
        <div className="mt-1 text-xs text-[var(--ink-muted)]">Coming soon — no billing/subscription system exists yet.</div>
      </div>
    </motion.div>
  );
}
