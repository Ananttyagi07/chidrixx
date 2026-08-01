import { motion } from "framer-motion";
import type { DashboardSummary } from "../types";

// Real, minimal, honest -- no fake toggles for capabilities that don't
// exist (no theme switch, since light is the enforced default by
// deliberate choice; no notification prefs, since there's no
// notification system beyond the agent's own webhook alerting).
export function SettingsPage({ data }: { data: DashboardSummary }) {
  const rows: Array<[string, string]> = [
    ["Authentication", "Basic Auth, one shared token — no per-user accounts"],
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
        There's no per-user role or notification preference to configure — one shared token
        controls everything behind it. To rotate it, update the{" "}
        <code className="rounded bg-[var(--surface-sunken)] px-1 py-0.5">CHIDRIXX_AUTH_TOKEN</code> secret
        and restart the control plane.
      </p>
    </motion.div>
  );
}
