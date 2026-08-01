import { useState } from "react";
import { motion } from "framer-motion";
import type { Session } from "../session";

// A real login: username + password checked against a real bcrypt hash
// server-side (controlplane/tenant.go's AuthenticateUser), resulting in a
// real server-tracked session cookie -- not the browser's native Basic
// Auth dialog against one secret every tenant used to share.
export function LoginPage({ onLoggedIn }: { onLoggedIn: (session: Session) => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const res = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      if (!res.ok) {
        setError(res.status === 401 ? "Invalid username or password." : `Login failed (HTTP ${res.status}).`);
        return;
      }
      const session: Session = await res.json();
      onLoggedIn(session);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--page)] px-6">
      <motion.form
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, ease: [0.16, 1, 0.3, 1] }}
        onSubmit={submit}
        className="flex w-full max-w-sm flex-col gap-4 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-[var(--card-shadow)]"
      >
        <div>
          <div className="text-lg font-semibold text-[var(--ink)]">Sign in to chidrixx</div>
          <div className="mt-1 text-xs text-[var(--ink-muted)]">
            Your tenant's real login — provisioned by whoever set up this control plane.
          </div>
        </div>

        <label className="flex flex-col gap-1 text-xs text-[var(--ink-secondary)]">
          Username
          <input
            autoFocus
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-2 text-sm outline-none focus:border-[var(--accent)]"
            autoComplete="username"
          />
        </label>

        <label className="flex flex-col gap-1 text-xs text-[var(--ink-secondary)]">
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-2 text-sm outline-none focus:border-[var(--accent)]"
            autoComplete="current-password"
          />
        </label>

        {error && (
          <div className="rounded-lg border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-3 py-2 text-xs text-[var(--status-critical)]">
            {error}
          </div>
        )}

        <button
          type="submit"
          disabled={submitting || !username || !password}
          className="rounded-lg bg-[var(--accent)] px-4 py-2 text-sm font-semibold text-white disabled:opacity-50"
        >
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </motion.form>
    </div>
  );
}
