import { useState } from "react";
import { motion } from "framer-motion";
import { cardMotion } from "../motion";
import { formatINR } from "../format";
import { IconTrendingUp } from "../icons";
import type { AnomalyPoint } from "../types";
import { apiFetch } from "../apiFetch";

type NarrateState = "idle" | "loading" | "done" | "unavailable" | "error";

// Real, on-demand narration -- not run automatically for every anomaly
// on every dashboard load (that would mean an LLM call per anomaly per
// page view). One click, one real completion, grounded in the exact
// numbers already shown above it (see controlplane/anomaly_narrator.go).
function ExplainAnomaly({ clusterId }: { clusterId: string }) {
  const [state, setState] = useState<NarrateState>("idle");
  const [narrative, setNarrative] = useState("");

  async function explain() {
    setState("loading");
    try {
      const res = await apiFetch("/api/v1/anomalies/narrate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cluster_id: clusterId }),
      });
      if (res.status === 503) {
        setState("unavailable");
        return;
      }
      if (!res.ok) {
        setState("error");
        return;
      }
      const data: { narrative: string } = await res.json();
      setNarrative(data.narrative);
      setState("done");
    } catch {
      setState("error");
    }
  }

  if (state === "done") {
    return <div className="mt-2 text-[0.7rem] text-[var(--ink-secondary)]">{narrative}</div>;
  }
  if (state === "unavailable") {
    return <div className="mt-2 text-[0.68rem] text-[var(--ink-muted)]">Assistant not configured.</div>;
  }
  if (state === "error") {
    return <div className="mt-2 text-[0.68rem] text-[var(--status-critical)]">Couldn't explain this, try again.</div>;
  }
  return (
    <button
      onClick={explain}
      disabled={state === "loading"}
      className="mt-2 text-[0.68rem] text-[var(--accent)] hover:underline disabled:opacity-50"
    >
      {state === "loading" ? "Explaining…" : "Explain this"}
    </button>
  );
}

// A real cross-snapshot cost comparison per cluster (controlplane/anomaly.go)
// — no invented "spike vs usual" narrative, just the two real numbers and
// the ratio between them. A cluster needs at least two real snapshots
// ever ingested before it can appear here at all.
export function AnomalyCard({ anomalies }: { anomalies: AnomalyPoint[] }) {
  return (
    <motion.div
      {...cardMotion}
      className="flex h-full flex-col rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
        Anomaly detection
      </div>

      {anomalies.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-1 text-center">
          <div className="text-sm text-[var(--ink-secondary)]">No anomalies detected</div>
          <div className="text-xs text-[var(--ink-muted)]">
            Compares each cluster's two most recent snapshots — nothing has grown 2x or
            more since last check.
          </div>
        </div>
      ) : (
        <div className="flex flex-1 flex-col gap-2">
          {anomalies.map((a) => (
            <div
              key={a.cluster_id}
              className="rounded-lg border border-[var(--status-critical)]/30 bg-[var(--status-critical)]/5 px-3 py-2"
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <IconTrendingUp className="h-4 w-4 text-[var(--status-critical)]" />
                  <div>
                    <div className="text-xs font-semibold">{a.cluster_id}</div>
                    <div className="text-[0.7rem] text-[var(--ink-muted)]">
                      {formatINR(a.previous_cost_inr)} → {formatINR(a.current_cost_inr)}
                    </div>
                  </div>
                </div>
                <span className="rounded-full bg-[var(--status-critical)]/15 px-2 py-0.5 text-xs font-semibold text-[var(--status-critical)]">
                  {a.growth_ratio.toFixed(1)}x
                </span>
              </div>
              {a.likely_cause && (
                <div className="mt-2 rounded-lg border border-dashed border-[var(--border)] bg-[var(--surface-sunken)] px-2.5 py-1.5 text-[0.68rem] text-[var(--ink-secondary)]">
                  A real Deployment event in{" "}
                  <span className="font-mono">
                    {a.likely_cause.namespace}/{a.likely_cause.name}
                  </span>{" "}
                  ({a.likely_cause.message}) happened shortly before this jump — worth checking,
                  not proven causation.
                </div>
              )}
              <ExplainAnomaly clusterId={a.cluster_id} />
            </div>
          ))}
        </div>
      )}
    </motion.div>
  );
}
