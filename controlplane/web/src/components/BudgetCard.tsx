import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import { cardMotion } from "../motion";
import { formatINR } from "../format";

interface BudgetResponse {
  budget_inr: number;
  is_set: boolean;
}

// A real user-set figure compared against real total spend — no
// "AI-recommended budget," just what was typed in, persisted server-side
// (controlplane/budget_api.go).
export function BudgetCard({ spentINR }: { spentINR: number }) {
  const [budget, setBudget] = useState<BudgetResponse | null>(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    fetch("/api/v1/budget")
      .then((r) => r.json())
      .then(setBudget)
      .catch(() => setBudget({ budget_inr: 0, is_set: false }));
  }, []);

  async function save() {
    const amount = parseFloat(draft);
    if (!Number.isFinite(amount) || amount < 0) return;

    setSaving(true);
    try {
      const res = await fetch("/api/v1/budget", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ budget_inr: amount }),
      });
      const updated = await res.json();
      setBudget(updated);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  }

  const pct = budget?.is_set && budget.budget_inr > 0 ? Math.min((spentINR / budget.budget_inr) * 100, 999) : 0;
  const over = budget?.is_set && spentINR > budget.budget_inr;

  return (
    <motion.div
      {...cardMotion}
      className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">Budget status</div>

      {budget === null ? (
        <div className="flex flex-1 items-center justify-center text-sm text-[var(--ink-muted)]">Loading…</div>
      ) : editing ? (
        <div className="flex flex-1 flex-col justify-center gap-2">
          <label className="text-xs text-[var(--ink-secondary)]">Set an overall spend budget (INR)</label>
          <input
            autoFocus
            type="number"
            min="0"
            step="0.01"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && save()}
            className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2.5 py-1.5 text-sm tabular-nums outline-none focus:border-[var(--accent)]"
            placeholder="e.g. 5000"
          />
          <div className="flex gap-2">
            <button
              onClick={save}
              disabled={saving}
              className="rounded-lg bg-[var(--accent)] px-3 py-1 text-xs font-medium text-white disabled:opacity-50"
            >
              {saving ? "Saving…" : "Save"}
            </button>
            <button
              onClick={() => setEditing(false)}
              className="rounded-lg border border-[var(--border)] px-3 py-1 text-xs text-[var(--ink-secondary)]"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : !budget.is_set ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-center">
          <div className="text-sm text-[var(--ink-muted)]">No budget set yet.</div>
          <button
            onClick={() => {
              setDraft("");
              setEditing(true);
            }}
            className="rounded-lg bg-[var(--accent)] px-3 py-1.5 text-xs font-medium text-white"
          >
            Set a budget
          </button>
        </div>
      ) : (
        <div className="flex flex-1 flex-col justify-center gap-2">
          <div className="flex items-baseline justify-between">
            <span className="text-lg font-semibold tabular-nums">{formatINR(spentINR)}</span>
            <span className="text-xs text-[var(--ink-muted)]">of {formatINR(budget.budget_inr)}</span>
          </div>
          <div className="h-2 rounded-full bg-[var(--gridline)]">
            <motion.div
              initial={{ width: 0 }}
              animate={{ width: `${Math.min(pct, 100)}%` }}
              transition={{ duration: 0.6, ease: [0.16, 1, 0.3, 1] }}
              className="h-2 rounded-full"
              style={{ background: over ? "var(--status-critical)" : "var(--accent)" }}
            />
          </div>
          <div className="flex items-center justify-between">
            <span className={`text-xs ${over ? "text-[var(--status-critical)]" : "text-[var(--ink-secondary)]"}`}>
              {pct.toFixed(1)}% {over ? "— over budget" : "used"}
            </span>
            <button
              onClick={() => {
                setDraft(String(budget.budget_inr));
                setEditing(true);
              }}
              className="text-xs text-[var(--accent)] hover:underline"
            >
              edit
            </button>
          </div>
        </div>
      )}
    </motion.div>
  );
}
