import { useEffect, useState } from "react";
import { apiFetch } from "../apiFetch";
import { motion } from "framer-motion";
import type { DashboardSummary, Invite, TeamOwnership } from "../types";
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
    apiFetch("/api/v1/teams")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setOwnership)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }

  useEffect(reload, []);

  // MembersCard is admin-only (the backend's /api/v1/invites route is
  // gated behind requireAdmin for every method, including GET) -- a
  // viewer never even attempts the request.

  async function addMapping(e: React.FormEvent) {
    e.preventDefault();
    if (!namespace.trim() || !team.trim()) return;
    setSaving(true);
    try {
      const res = await apiFetch("/api/v1/teams", {
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
      const res = await apiFetch(`/api/v1/teams?namespace=${encodeURIComponent(ns)}`, { method: "DELETE" });
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

      {canEdit && <MembersCard />}

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

// MembersCard: real self-service teammate invites -- the replacement for
// needing shell access to the control plane to run the create-user CLI.
// An invite is just a (tenant, email, role) row (controlplane/invite.go);
// the invitee joins this exact tenant with this exact role the first
// time they ever sign in with that email, no token or link to copy
// around, since Supabase's own signup/login with the matching email is
// the real credential the invite is keyed on.
function MembersCard() {
  const [invites, setInvites] = useState<Invite[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<"admin" | "viewer">("viewer");
  const [saving, setSaving] = useState(false);

  function reload() {
    apiFetch("/api/v1/invites")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setInvites)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }

  useEffect(reload, []);

  async function sendInvite(e: React.FormEvent) {
    e.preventDefault();
    if (!email.trim()) return;
    setSaving(true);
    try {
      const res = await apiFetch("/api/v1/invites", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: email.trim(), role: inviteRole }),
      });
      if (!res.ok) throw new Error(await res.text());
      setEmail("");
      reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  async function revokeInvite(inviteEmail: string) {
    try {
      const res = await apiFetch(`/api/v1/invites?email=${encodeURIComponent(inviteEmail)}`, { method: "DELETE" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
      <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
        Invite a teammate
      </div>

      {error && (
        <div className="mb-3 rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-3 py-2 text-xs text-[var(--status-critical)]">
          {error}
        </div>
      )}

      {invites === null ? (
        <div className="text-sm text-[var(--ink-muted)]">Loading…</div>
      ) : (
        <>
          {invites.length > 0 && (
            <table className="mb-3 w-full border-collapse text-sm">
              <thead>
                <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--ink-muted)]">
                  <th className="py-2 pr-3 font-normal">Email</th>
                  <th className="py-2 pr-3 font-normal">Role</th>
                  <th className="py-2 font-normal" />
                </tr>
              </thead>
              <tbody>
                {invites.map((inv) => (
                  <tr key={inv.email} className="border-b border-[var(--border)] last:border-0">
                    <td className="py-2 pr-3 font-mono text-xs">{inv.email}</td>
                    <td className="py-2 pr-3 capitalize">{inv.role}</td>
                    <td className="py-2 text-right">
                      <button
                        onClick={() => revokeInvite(inv.email)}
                        className="text-xs text-[var(--ink-muted)] hover:text-[var(--status-critical)]"
                      >
                        revoke
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <form onSubmit={sendInvite} className="flex flex-wrap items-center gap-2">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="teammate@company.com"
              className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2.5 py-1.5 text-sm outline-none focus:border-[var(--accent)]"
            />
            <select
              value={inviteRole}
              onChange={(e) => setInviteRole(e.target.value as "admin" | "viewer")}
              className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-2.5 py-1.5 text-sm outline-none focus:border-[var(--accent)]"
            >
              <option value="viewer">Viewer</option>
              <option value="admin">Admin</option>
            </select>
            <button
              type="submit"
              disabled={saving}
              className="rounded-lg bg-[var(--accent)] px-3 py-1.5 text-xs font-medium text-white disabled:opacity-50"
            >
              {saving ? "Inviting…" : "Send invite"}
            </button>
          </form>
          <div className="mt-2 text-[0.68rem] text-[var(--ink-muted)]">
            They'll join this tenant automatically the first time they sign up or log in with this
            email — no link to send, no shell access needed.
          </div>
        </>
      )}
    </div>
  );
}
