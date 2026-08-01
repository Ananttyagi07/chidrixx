import { useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import type { FindingRow } from "../types";
import { PATH_CLASS_LABEL } from "../palette";
import { TopFixesTable } from "./TopFixesTable";
import { IconSearch } from "../icons";

const ALL_CLASSES = Object.keys(PATH_CLASS_LABEL);
const ALL_CONFIDENCE = ["high", "med", "low"];

// A power-user filter over the exact same /api/v1/findings data
// Costs & Usage shows -- multi-dimension filtering (path class,
// confidence) rather than a single search box, for narrowing down to a
// specific investigation rather than browsing everything.
export function ExplorerPage() {
  const [findings, setFindings] = useState<FindingRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [classes, setClasses] = useState<Set<string>>(new Set());
  const [confidences, setConfidences] = useState<Set<string>>(new Set());
  const [sortBy, setSortBy] = useState<"cost" | "bytes">("cost");

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

  function toggle(set: Set<string>, setSet: (s: Set<string>) => void, value: string) {
    const next = new Set(set);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    setSet(next);
  }

  const filtered = useMemo(() => {
    if (!findings) return [];
    const q = search.trim().toLowerCase();
    const rows = findings.filter((f) => {
      if (classes.size > 0 && !classes.has(f.path_class)) return false;
      if (confidences.size > 0 && !confidences.has(f.confidence)) return false;
      if (!q) return true;
      return f.source.toLowerCase().includes(q) || f.destination.toLowerCase().includes(q);
    });
    rows.sort((a, b) =>
      sortBy === "bytes" ? b.bytes_tx + b.bytes_rx - (a.bytes_tx + a.bytes_rx) : b.cost_high_inr - a.cost_high_inr,
    );
    return rows;
  }, [findings, search, classes, confidences, sortBy]);

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">Explorer</h2>

      <div className="relative">
        <IconSearch className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--ink-muted)]" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search source or destination…"
          className="w-full rounded-lg border border-[var(--border)] bg-[var(--surface)] py-1.5 pl-8 pr-3 text-sm shadow-[var(--card-shadow)] outline-none focus:border-[var(--accent)]"
        />
      </div>

      <div className="flex flex-wrap items-center gap-4 text-xs">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-[var(--ink-muted)]">Path:</span>
          {ALL_CLASSES.map((c) => (
            <button
              key={c}
              onClick={() => toggle(classes, setClasses, c)}
              className={`rounded-full border px-2 py-1 ${
                classes.has(c)
                  ? "border-[var(--accent)] bg-[var(--accent-wash)] text-[var(--accent)]"
                  : "border-[var(--border)] text-[var(--ink-secondary)]"
              }`}
            >
              {PATH_CLASS_LABEL[c]}
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-4 text-xs">
        <div className="flex items-center gap-1.5">
          <span className="text-[var(--ink-muted)]">Confidence:</span>
          {ALL_CONFIDENCE.map((c) => (
            <button
              key={c}
              onClick={() => toggle(confidences, setConfidences, c)}
              className={`rounded-full border px-2 py-1 ${
                confidences.has(c)
                  ? "border-[var(--accent)] bg-[var(--accent-wash)] text-[var(--accent)]"
                  : "border-[var(--border)] text-[var(--ink-secondary)]"
              }`}
            >
              {c}
            </button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          <span className="text-[var(--ink-muted)]">Sort:</span>
          {(["cost", "bytes"] as const).map((k) => (
            <button
              key={k}
              onClick={() => setSortBy(k)}
              className={`rounded-full px-2.5 py-1 ${
                sortBy === k ? "bg-[var(--accent)] text-white" : "border border-[var(--border)] text-[var(--ink-secondary)]"
              }`}
            >
              {k}
            </button>
          ))}
        </div>
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
            {filtered.length} of {findings.length} flows match
          </div>
          <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
            <TopFixesTable
              findings={filtered}
              emptyMessage={
                findings.length === 0 ? "No findings yet." : "No flows match these filters."
              }
            />
          </div>
        </>
      )}
    </motion.div>
  );
}
