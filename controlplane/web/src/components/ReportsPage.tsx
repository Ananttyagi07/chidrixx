import { useState } from "react";
import { apiFetch } from "../apiFetch";
import { motion } from "framer-motion";
import type { DashboardSummary, FindingRow } from "../types";
import { SpendTrendChart } from "./SpendTrendChart";
import { formatINR } from "../format";
import { IconDownload } from "../icons";

function toCSV(rows: FindingRow[]): string {
  const headers = [
    "cluster_id",
    "reported_at",
    "source",
    "destination",
    "path_class",
    "confidence",
    "bytes_tx",
    "bytes_rx",
    "cost_low_inr",
    "cost_high_inr",
    "fix_hint",
  ];

  const escape = (v: string) => `"${v.replace(/"/g, '""')}"`;

  const lines = [headers.join(",")];
  for (const f of rows) {
    lines.push(
      [
        f.ClusterID,
        f.ReportedAt,
        f.source,
        f.destination,
        f.path_class,
        f.confidence,
        f.bytes_tx,
        f.bytes_rx,
        f.cost_low_inr,
        f.cost_high_inr,
        f.fix_hint,
      ]
        .map((v) => escape(String(v)))
        .join(","),
    );
  }
  return lines.join("\n");
}

// Real trend history (same data as Overview's Spend Trend) plus a real
// CSV export of the current findings snapshot -- generated client-side
// from /api/v1/findings, not a mocked-up download.
export function ReportsPage({ data }: { data: DashboardSummary }) {
  const [exporting, setExporting] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

  async function exportCSV() {
    setExporting(true);
    setExportError(null);
    try {
      const res = await apiFetch("/api/v1/findings");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const findings: FindingRow[] = await res.json();
      const csv = toCSV(findings);
      const blob = new Blob([csv], { type: "text/csv" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `chidrixx-findings-${new Date().toISOString().slice(0, 19)}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      setExportError(e instanceof Error ? e.message : String(e));
    } finally {
      setExporting(false);
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Reports</h2>
        <button
          onClick={exportCSV}
          disabled={exporting}
          className="flex items-center gap-1.5 rounded-lg bg-[var(--accent)] px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
        >
          <IconDownload className="h-3.5 w-3.5" />
          {exporting ? "Exporting…" : "Export findings as CSV"}
        </button>
      </div>

      {exportError && (
        <div className="rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
          Export failed: {exportError}
        </div>
      )}

      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
        <div className="mb-1 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Combined spend trend
        </div>
        <SpendTrendChart points={data.trend} />
      </div>

      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
        <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Per-cluster totals
        </div>
        {data.clusters.length === 0 ? (
          <div className="py-4 text-center text-sm text-[var(--ink-muted)]">No clusters have shipped data yet.</div>
        ) : (
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--ink-muted)]">
                <th className="py-2 pr-3 font-normal">Cluster</th>
                <th className="py-2 pr-3 font-normal">Last seen</th>
                <th className="py-2 pr-3 text-right font-normal">Findings</th>
                <th className="py-2 text-right font-normal">Cost (high est.)</th>
              </tr>
            </thead>
            <tbody>
              {data.clusters.map((c) => (
                <tr key={c.ClusterID} className="border-b border-[var(--border)] last:border-0">
                  <td className="py-2 pr-3 font-mono text-xs">{c.ClusterID}</td>
                  <td className="py-2 pr-3 text-xs text-[var(--ink-secondary)]">
                    {new Date(c.LastSeen).toLocaleString()}
                  </td>
                  <td className="py-2 pr-3 text-right tabular-nums">{c.FindingCount}</td>
                  <td className="py-2 text-right font-mono tabular-nums">{formatINR(c.CostHighINR)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </motion.div>
  );
}
