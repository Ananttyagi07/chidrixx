import { useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import type { FindingRow } from "../types";
import { PATH_CLASS_LABEL } from "../palette";
import { formatINR } from "../format";
import { ConfidenceChip } from "./ConfidenceChip";
import { apiFetch } from "../apiFetch";

// Real closed-loop tracking, not a UI-only checkbox: this POSTs to the
// same recommendation_outcomes row the dashboard auto-logs every time a
// fix is shown, so "mark applied" starts a real before/after cost
// comparison against the next real snapshot, not a fabricated one.
async function markApplied(f: FindingRow): Promise<boolean> {
  const res = await apiFetch("/api/v1/outcomes/apply", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      cluster_id: f.ClusterID,
      source: f.source,
      destination: f.destination,
      path_class: f.path_class,
    }),
  });
  return res.ok;
}

export function TopFixesTable({
  findings,
  emptyMessage = "No flagged flows yet — nothing wasteful found (or nothing shipped yet).",
}: {
  findings: FindingRow[];
  emptyMessage?: string;
}) {
  if (findings.length === 0) {
    return <div className="py-8 text-center text-sm text-[var(--ink-muted)]">{emptyMessage}</div>;
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--ink-muted)]">
            <th className="py-2 pr-3 font-normal">Cluster</th>
            <th className="py-2 pr-3 font-normal">Source</th>
            <th className="py-2 pr-3 font-normal">Destination</th>
            <th className="py-2 pr-3 font-normal">Path</th>
            <th className="py-2 pr-3 font-normal">Confidence</th>
            <th className="py-2 pr-3 text-right font-normal">Cost (INR)</th>
            <th className="py-2 pr-3 text-right font-normal">Potential savings</th>
            <th className="py-2 pr-3 font-normal">Fix</th>
            <th className="py-2 font-normal">Applied?</th>
          </tr>
        </thead>
        <tbody>
          {findings.map((f, i) => (
            <FixRow key={i} finding={f} index={i} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function FixRow({ finding: f, index }: { finding: FindingRow; index: number }) {
  const [open, setOpen] = useState(false);
  const [applyState, setApplyState] = useState<"idle" | "saving" | "applied" | "error">("idle");

  async function handleMarkApplied() {
    setApplyState("saving");
    const ok = await markApplied(f);
    setApplyState(ok ? "applied" : "error");
  }

  return (
    <>
      <motion.tr
        initial={{ opacity: 0, y: 6 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: Math.min(index * 0.04, 0.4), duration: 0.25 }}
        className="border-b border-[var(--border)] align-top transition-colors hover:bg-[var(--surface-sunken)]"
      >
        <td className="max-w-[8rem] truncate py-2 pr-3 font-mono text-xs text-[var(--ink-secondary)]" title={f.ClusterID}>
          {f.ClusterID}
        </td>
        <td className="max-w-[10rem] truncate py-2 pr-3 font-mono text-xs" title={f.source}>
          {f.source}
        </td>
        <td className="max-w-[10rem] truncate py-2 pr-3 font-mono text-xs" title={f.destination}>
          {f.destination}
        </td>
        <td className="py-2 pr-3 text-xs">{PATH_CLASS_LABEL[f.path_class] ?? f.path_class}</td>
        <td className="py-2 pr-3">
          <ConfidenceChip confidence={f.confidence} />
        </td>
        <td className="py-2 pr-3 text-right font-mono tabular-nums">
          {formatINR(f.cost_low_inr)}–{formatINR(f.cost_high_inr)}
        </td>
        <td className="py-2 pr-3 text-right font-mono tabular-nums">
          {f.savings_high_inr > 0 ? (
            <span className="text-[var(--status-good)]">
              {formatINR(f.savings_low_inr)}–{formatINR(f.savings_high_inr)}
            </span>
          ) : (
            <span className="text-[var(--ink-muted)]">—</span>
          )}
        </td>
        <td className="max-w-[16rem] py-2 pr-3 text-xs text-[var(--ink-secondary)]">
          {f.fix_hint}
          {f.fix_manifest && (
            <button
              onClick={() => setOpen((o) => !o)}
              className="ml-2 text-[var(--accent)] hover:underline"
            >
              {open ? "hide manifest" : "view manifest"}
            </button>
          )}
        </td>
        <td className="py-2 text-xs">
          {applyState === "applied" ? (
            <span className="text-[var(--status-good)]">Applied — tracking real cost impact</span>
          ) : applyState === "error" ? (
            <span className="text-[var(--status-critical)]">Couldn't save, try again</span>
          ) : (
            <button
              onClick={handleMarkApplied}
              disabled={applyState === "saving"}
              className="rounded-full border border-[var(--border)] px-2.5 py-1 text-[var(--ink-secondary)] hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:opacity-50"
            >
              {applyState === "saving" ? "Saving…" : "Mark as applied"}
            </button>
          )}
        </td>
      </motion.tr>
      <AnimatePresence>
        {open && f.fix_manifest && (
          <motion.tr
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="border-b border-[var(--border)]"
          >
            <td colSpan={9} className="bg-[var(--page)] p-3">
              <motion.pre
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.25 }}
                className="overflow-x-auto whitespace-pre rounded-md border border-[var(--border)] bg-[var(--surface)] p-3 font-mono text-xs"
              >
                {f.fix_manifest}
              </motion.pre>
            </td>
          </motion.tr>
        )}
      </AnimatePresence>
    </>
  );
}
