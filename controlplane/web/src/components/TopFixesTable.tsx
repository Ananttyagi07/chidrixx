import { useState } from "react";
import type { FindingRow } from "../types";
import { PATH_CLASS_LABEL } from "../palette";
import { formatINR } from "../format";
import { ConfidenceChip } from "./ConfidenceChip";

export function TopFixesTable({ findings }: { findings: FindingRow[] }) {
  if (findings.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-[var(--ink-muted)]">
        No flagged flows yet — nothing wasteful found (or nothing shipped yet).
      </div>
    );
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
            <th className="py-2 font-normal">Fix</th>
          </tr>
        </thead>
        <tbody>
          {findings.map((f, i) => (
            <FixRow key={i} finding={f} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function FixRow({ finding: f }: { finding: FindingRow }) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <tr className="border-b border-[var(--border)] align-top">
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
        <td className="max-w-[16rem] py-2 text-xs text-[var(--ink-secondary)]">
          {f.fix_hint}
          {f.fix_manifest && (
            <button
              onClick={() => setOpen((o) => !o)}
              className="ml-2 text-[var(--series-blue)] hover:underline"
            >
              {open ? "hide manifest" : "view manifest"}
            </button>
          )}
        </td>
      </tr>
      {open && f.fix_manifest && (
        <tr className="border-b border-[var(--border)]">
          <td colSpan={7} className="bg-[var(--page)] p-3">
            <pre className="overflow-x-auto whitespace-pre rounded-md border border-[var(--border)] bg-[var(--surface)] p-3 font-mono text-xs">
              {f.fix_manifest}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}
