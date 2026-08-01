import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import type { DashboardSummary, TeamOwnership } from "../types";
import { formatINR } from "../format";
import { useSession } from "../session";

// Real namespace -> team mapping, admin-configured (controlplane/team.go).
// Spend by team is computed from the exact same findings every other card
// uses, grouped through this mapping -- anything from a namespace nobody's
// mapped yet (or a non-k8s-resolved cgroup-path source) honestly falls
// into "Unassigned" rather than a guessed owner.
export function TeamsPage({ data }: { data: DashboardSummary }) {
  const { role } = useSession();
  const canEdit = role === "admin";

  const [ownership, setOwnership] = useState<TeamOwnership[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [namespace, setNamespace] = useState("");
  const [team, setTeam] = useState("");
  const [saving, setSaving] = useState(false);

  function reload() {
    fetch("/api/v1/teams")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setOwnership)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }

  useEffect(reload, []);

  async function addMapping(e: React.FormEvent) {
    e.preventDefault();
    if (!namespace.trim() || !team.trim()) return;
    setSaving(true);
    try {
      const res = await fetch("/api/v1/teams", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ namespace: namespace.trim(), team: team.trim() }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setNamespace("");
      setTeam("");
      reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  async function removeMapping(ns: string) {
    try {
      const res = await fetch(`/api/v1/teams?namespace=${encodeURIComponent(ns)}`, { method: "DELETE" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">Teams</h2>

      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
        <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Spend by team
        </div>
        {data.spend_by_team.length === 0 ? (
          <div className="py-4 text-center text-sm text-[var(--ink-muted)]">No findings yet.</div>
        ) : (
          <div className="flex flex-col gap-2">
            {data.spend_by_team.map((t) => (
              <div key={t.team} className="flex items-center justify-between gap-3 text-sm">
                <span className={t.team === "Unassigned" ? "text-[var(--ink-muted)]" : "font-medium"}>{t.team}</span>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-[var(--ink-muted)]">{t.finding_count} flows</span>
                  <span className="font-mono tabular-nums">{formatINR(t.cost_high_inr)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
        <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Namespace ownership
        </div>

        {error && (
          <div className="mb-3 rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-3 py-2 text-xs text-[var(--status-critical)]">
            {error}
          </div>
        )}

        {ownership === null ? (
          <div className="text-sm text-[var(--ink-muted)]">Loading…</div>
        ) : (
          <>
            {ownership.length === 0 ? (
              <div className="py-2 text-sm text-[var(--ink-muted)]">
                No mappings configured yet — everything shows as "Unassigned" above.
              </div>
            ) : (
              <table className="mb-3 w-full border-collapse text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--ink-muted)]">
                    <th className="py-2 pr-3 font-normal">Namespace</th>
                    <th className="py-2 pr-3 font-normal">Team</th>
                    {canEdit && <th className="py-2 font-normal" />}
                  </tr>
                </thead>
                <tbody>
                  {ownership.map((o) => (
                    <tr key={o.namespace} className="border-b border-[var(--border)] last:border-0">
                      <td className="py-2 pr-3 font-mono text-xs">{o.namespace}</td>
                      <td className="py-2 pr-3">{o.team}</td>
                      {canEdit && (
                        <td className="py-2 text-right">
                          <button
                            onClick={() => removeMapping(o.namespace)}
                            className="text-xs text-[var(--ink-muted)] hover:text-[var(--status-critical)]"
                          >
                            remove
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            )}

            {canEdit ? (
              <form onSubmit={addMapping} className="flex flex-wrap items-center gap-2">
                <input
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                  placeholder="namespace (e.g. checkout)"
                  className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2.5 py-1.5 text-sm outline-none focus:border-[var(--accent)]"
                />
                <input
                  value={team}
                  onChange={(e) => setTeam(e.target.value)}
                  placeholder="team (e.g. Payments)"
                  className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2.5 py-1.5 text-sm outline-none focus:border-[var(--accent)]"
                />
                <button
                  type="submit"
                  disabled={saving}
                  className="rounded-lg bg-[var(--accent)] px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
                >
                  {saving ? "Adding…" : "Add mapping"}
                </button>
              </form>
            ) : (
              <div className="text-xs text-[var(--ink-muted)]">Ask an admin to change namespace ownership.</div>
            )}
          </>
        )}
      </div>
    </motion.div>
  );
}
