import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { IconBell } from "../icons";
import { apiFetch } from "../apiFetch";
import { formatINR, relativeTime } from "../format";

interface AnomalyAlert {
  id: number;
  cluster_id: string;
  detected_at: string;
  previous_cost_inr: number;
  current_cost_inr: number;
  growth_ratio: number;
}

const POLL_MS = 15_000;

// The real "notice" half of "notice, investigate, recommend, notify"
// (controlplane/anomaly_watch.go): anomaly detection itself already
// existed, but until now it only ever ran when an operator opened the
// Anomalies page or asked the chat assistant. A background loop now
// checks every real tenant every 5 minutes server-side; this bell polls
// for whatever it's proactively found, on every page, not just Anomalies.
export function AnomalyAlertBell() {
  const [alerts, setAlerts] = useState<AnomalyAlert[]>([]);
  const [open, setOpen] = useState(false);
  const [busyID, setBusyID] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    function poll() {
      apiFetch("/api/v1/anomalies/alerts")
        .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
        .then((d: AnomalyAlert[]) => !cancelled && setAlerts(d))
        .catch(() => {
          /* silent -- a real bell should never crash the whole dashboard over a poll failure */
        });
    }
    poll();
    const id = window.setInterval(poll, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  async function acknowledge(id: number) {
    setBusyID(id);
    try {
      const res = await apiFetch("/api/v1/anomalies/alerts/acknowledge", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id }),
      });
      if (res.ok || res.status === 404) {
        setAlerts((prev) => prev.filter((a) => a.id !== id));
      }
    } finally {
      setBusyID(null);
    }
  }

  return (
    <div className="relative">
      <motion.button
        whileTap={{ scale: 0.94 }}
        onClick={() => setOpen((o) => !o)}
        className="relative flex h-9 w-9 items-center justify-center rounded-full border border-[var(--border)] bg-[var(--surface)] text-[var(--ink-secondary)]"
        title="Proactively detected anomalies"
      >
        {alerts.length > 0 && (
          <motion.span
            className="absolute inset-0 rounded-full border border-[var(--ink)]"
            animate={{ scale: [1, 1.45], opacity: [0.5, 0] }}
            transition={{ duration: 2, repeat: Infinity, ease: "easeOut" }}
          />
        )}
        <IconBell className="h-4 w-4" />
        {alerts.length > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-3.5 min-w-[0.875rem] items-center justify-center rounded-full bg-[var(--ink)] px-1 text-[0.55rem] font-semibold text-[var(--surface)]">
            {alerts.length}
          </span>
        )}
      </motion.button>

      <AnimatePresence>
        {open && (
          <motion.div
            data-testid="anomaly-alert-panel"
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            className="absolute right-0 z-20 mt-2 w-80 rounded-xl border border-[var(--border)] bg-[var(--surface)] p-3 shadow-[var(--card-shadow)]"
          >
            <div className="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
              Proactively detected anomalies
            </div>
            {alerts.length === 0 ? (
              <div className="py-4 text-center text-xs text-[var(--ink-muted)]">
                No new anomalies since the last check.
              </div>
            ) : (
              <div className="flex max-h-80 flex-col gap-2 overflow-y-auto">
                {alerts.map((a) => (
                  <div key={a.id} className="rounded-lg border border-[var(--border)] bg-[var(--page)] p-2.5 text-xs">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-[var(--ink-secondary)]">{a.cluster_id}</span>
                      <span className="text-[var(--ink-muted)]">{relativeTime(a.detected_at)}</span>
                    </div>
                    <div className="mt-1 text-[var(--ink)]">
                      {formatINR(a.previous_cost_inr)} → {formatINR(a.current_cost_inr)} ({a.growth_ratio.toFixed(1)}x)
                    </div>
                    <button
                      onClick={() => acknowledge(a.id)}
                      disabled={busyID === a.id}
                      className="mt-1.5 rounded-lg border border-[var(--border)] px-2 py-1 text-[0.65rem] text-[var(--ink-secondary)] disabled:opacity-50"
                    >
                      {busyID === a.id ? "Dismissing…" : "Dismiss"}
                    </button>
                  </div>
                ))}
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
