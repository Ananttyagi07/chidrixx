import { useState } from "react";
import { motion } from "framer-motion";
import { supabase } from "../supabaseClient";
import type { Session } from "../session";
import { apiFetch } from "../apiFetch";

// Real Supabase Auth: signUp/signInWithPassword talk directly to your
// Supabase project, not a chidrixx-hosted credential store. On a brand
// new signup, this Supabase project requires email confirmation before
// a session is issued (confirmed by testing against the real project) --
// so signup and "check your email" are a real, separate state here, not
// glossed over as an instant login.
export function LoginPage({ onLoggedIn }: { onLoggedIn: (session: Session) => void }) {
  const [mode, setMode] = useState<"sign-in" | "sign-up">("sign-in");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [confirmationSent, setConfirmationSent] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  async function handleSignIn(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const { data, error: signInError } = await supabase.auth.signInWithPassword({ email, password });
      if (signInError) {
        setError(signInError.message);
        return;
      }
      if (!data.session) {
        setError("Sign-in succeeded but no session was returned — please try again.");
        return;
      }

      // Resolve (and on this Supabase user's very first login, auto-
      // provision) this control plane's own tenant/role for the real
      // session we just got.
      const res = await apiFetch("/api/v1/auth/me");
      if (!res.ok) {
        setError(`Signed in, but the control plane rejected the session (HTTP ${res.status}).`);
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

  async function handleSignUp(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const { data, error: signUpError } = await supabase.auth.signUp({ email, password });
      if (signUpError) {
        setError(signUpError.message);
        return;
      }
      if (data.session) {
        // This project has email confirmation disabled -- a session
        // came back immediately, so provision and go straight in.
        const res = await apiFetch("/api/v1/auth/me");
        if (res.ok) {
          onLoggedIn(await res.json());
          return;
        }
      }
      // The real, expected path for this project: no session yet,
      // confirmation email sent.
      setConfirmationSent(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  if (confirmationSent) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--page)] px-6">
        <motion.div
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          className="flex w-full max-w-sm flex-col gap-3 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 text-center shadow-[var(--card-shadow)]"
        >
          <div className="text-lg font-semibold text-[var(--ink)]">Check your email</div>
          <div className="text-sm text-[var(--ink-secondary)]">
            We sent a confirmation link to <span className="font-medium">{email}</span>. Click it,
            then come back and sign in.
          </div>
          <button
            onClick={() => {
              setConfirmationSent(false);
              setMode("sign-in");
            }}
            className="mt-2 text-sm text-[var(--accent)] hover:underline"
          >
            Back to sign in
          </button>
        </motion.div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--page)] px-6">
      <motion.form
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, ease: [0.16, 1, 0.3, 1] }}
        onSubmit={mode === "sign-in" ? handleSignIn : handleSignUp}
        className="flex w-full max-w-sm flex-col gap-4 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-[var(--card-shadow)]"
      >
        <div>
          <div className="text-lg font-semibold text-[var(--ink)]">
            {mode === "sign-in" ? "Sign in to chidrixx" : "Create your chidrixx account"}
          </div>
          <div className="mt-1 text-xs text-[var(--ink-muted)]">
            {mode === "sign-in"
              ? "Real login, backed by Supabase Auth."
              : "Your own real tenant is created automatically on first sign-in."}
          </div>
        </div>

        <label className="flex flex-col gap-1 text-xs text-[var(--ink-secondary)]">
          Email
          <input
            autoFocus
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-2 text-sm outline-none focus:border-[var(--accent)]"
            autoComplete="email"
          />
        </label>

        <label className="flex flex-col gap-1 text-xs text-[var(--ink-secondary)]">
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-2 text-sm outline-none focus:border-[var(--accent)]"
            autoComplete={mode === "sign-in" ? "current-password" : "new-password"}
          />
        </label>

        {error && (
          <div className="rounded-lg border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-3 py-2 text-xs text-[var(--status-critical)]">
            {error}
          </div>
        )}

        <button
          type="submit"
          disabled={submitting || !email || !password}
          className="rounded-lg bg-[var(--accent)] px-4 py-2 text-sm font-semibold text-white disabled:opacity-50"
        >
          {submitting ? "Working…" : mode === "sign-in" ? "Sign in" : "Sign up"}
        </button>

        <button
          type="button"
          onClick={() => {
            setMode(mode === "sign-in" ? "sign-up" : "sign-in");
            setError(null);
          }}
          className="text-center text-xs text-[var(--ink-muted)] hover:text-[var(--accent)]"
        >
          {mode === "sign-in" ? "Need an account? Sign up" : "Already have an account? Sign in"}
        </button>
      </motion.form>
    </div>
  );
}
