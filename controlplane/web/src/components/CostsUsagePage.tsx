import { useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import type { FindingRow } from "../types";
import { TopFixesTable } from "./TopFixesTable";
import { formatINR } from "../format";
import { IconSearch } from "../icons";

// The full cross-cluster findings list (controlplane/api.go's
// /api/v1/findings) — every real flow currently attributed, not just the
// ones with a fix hint. Reuses TopFixesTable as-is since its rendering
// already handles the general case (a flow with no fix_hint just shows
// an empty Fix cell).
export function CostsUsagePage() {
  const [findings, setFindings] = useState<FindingRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [cluster, setCluster] = useState<string>("all");

  useEffect(() => {
    let cancelled = false;

    fetch("/api/v1/findings")
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then((d) => !cancelled && setFindings(d))
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)));

    return () => {
      cancelled = true;
    };
  }, []);

  const clusters = useMemo(() => {
    if (!findings) return [];
    return Array.from(new Set(findings.map((f) => f.ClusterID))).sort();
  }, [findings]);

  const filtered = useMemo(() => {
    if (!findings) return [];
    const q = search.trim().toLowerCase();
    return findings.filter((f) => {
      if (cluster !== "all" && f.ClusterID !== cluster) return false;
      if (!q) return true;
      return (
        f.source.toLowerCase().includes(q) ||
        f.destination.toLowerCase().includes(q) ||
        f.path_class.toLowerCase().includes(q)
      );
    });
  }, [findings, search, cluster]);

  const totalCost = filtered.reduce((s, f) => s + f.cost_high_inr, 0);

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">Costs &amp; Usage</h2>

      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-[14rem]">
          <IconSearch className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--ink-muted)]" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search source, destination, path class…"
            className="w-full rounded-lg border border-[var(--border)] bg-[var(--surface)] py-1.5 pl-8 pr-3 text-sm shadow-[var(--card-shadow)] outline-none focus:border-[var(--accent)]"
          />
        </div>
        <select
          value={cluster}
          onChange={(e) => setCluster(e.target.value)}
          className="rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-sm shadow-[var(--card-shadow)] outline-none"
        >
          <option value="all">All clusters</option>
          {clusters.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
      </div>

      {error && (
        <div className="rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
          Couldn't load findings: {error}
        </div>
      )}

      {!findings && !error && <div className="text-sm text-[var(--ink-muted)]">Loading…</div>}

      {findings && (
        <>
          <div className="text-xs text-[var(--ink-muted)]">
            {filtered.length} flow{filtered.length === 1 ? "" : "s"} · {formatINR(totalCost)} total (high estimate)
          </div>
          <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
            <TopFixesTable
              findings={filtered}
              emptyMessage={
                findings.length === 0
                  ? "No findings yet — nothing shipped to this control plane."
                  : "No flows match this search/filter."
              }
            />
          </div>
        </>
      )}
    </motion.div>
  );
}
