import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import type { DashboardSummary, RemediationDecision } from "../types";
import { PATH_CLASS_LABEL } from "../palette";
import { formatINR } from "../format";
import { IconDownload } from "../icons";
import { apiFetch } from "../apiFetch";

// Real generated NetworkPolicy manifests (agent/cmd/kharcha/fixengine.go),
// listed for review -- copy or download, never applied automatically.
// chidrixx has no write access to any cluster from the control plane;
// building that would be a much bigger, riskier capability than this page
// implies, and isn't something to add quietly.
export function AutomationsPage({ data }: { data: DashboardSummary }) {
  const withManifests = data.top_fixes.filter((f) => f.fix_manifest);

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="flex flex-col gap-4"
    >
      <h2 className="text-lg font-semibold">Automations</h2>

      <div className="rounded-xl border border-[var(--border)] bg-[var(--surface-sunken)] px-4 py-3 text-xs text-[var(--ink-secondary)]">
        chidrixx generates these manifests but <strong>never applies them automatically</strong> — it has
        no write access to any cluster. Review each one, then apply it yourself with{" "}
        <code className="rounded bg-[var(--surface)] px-1 py-0.5 font-mono">kubectl apply -f</code>.
      </div>

      {withManifests.length === 0 ? (
        <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] py-8 text-center text-sm text-[var(--ink-muted)] shadow-[var(--card-shadow)]">
          No generated manifests right now — nothing currently flagged has a mechanical fix available.
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {withManifests.map((f, i) => (
            <ManifestCard key={i} clusterId={f.ClusterID} source={f.source} destination={f.destination} pathClass={f.path_class} cost={f.cost_high_inr} manifest={f.fix_manifest} />
          ))}
        </div>
      )}

      <RemediationPreview />
    </motion.div>
  );
}

// Real, computed-on-demand simulation (controlplane/remediation.go) of
// what a future auto-remediation feature would decide to do right now,
// under a real, deterministic policy -- and, transparently, what it
// would skip and why. This never applies anything: it's the evidence-
// gathering phase before ever requesting write access to a real
// cluster, not a preview of a feature that secretly already runs.
function RemediationPreview() {
  const [decisions, setDecisions] = useState<RemediationDecision[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    apiFetch("/api/v1/remediation/preview")
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((d: { decisions: RemediationDecision[] }) => !cancelled && setDecisions(d.decisions))
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)));
    return () => {
      cancelled = true;
    };
  }, []);

  const wouldApply = (decisions ?? []).filter((d) => d.would_auto_apply);
  const wouldSkip = (decisions ?? []).filter((d) => !d.would_auto_apply);

  return (
    <div className="mt-2 flex flex-col gap-3">
      <div>
        <h3 className="text-sm font-semibold">If auto-remediation were enabled</h3>
        <p className="mt-1 max-w-2xl text-xs text-[var(--ink-muted)]">
          A real, deterministic policy (real generated manifest + high confidence + positive predicted
          savings) evaluated against your current real flagged fixes. Nothing here is applied — chidrixx
          has no write access to any cluster. This is the evidence-gathering step before that would ever
          become a real, explicitly opt-in feature.
        </p>
      </div>

      {error && (
        <div className="rounded-xl border border-[var(--status-critical)]/40 bg-[var(--status-critical)]/10 px-4 py-3 text-sm text-[var(--status-critical)]">
          Couldn't load the remediation preview: {error}
        </div>
      )}

      {decisions && decisions.length === 0 && (
        <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] py-6 text-center text-sm text-[var(--ink-muted)] shadow-[var(--card-shadow)]">
          No currently flagged fixes to evaluate.
        </div>
      )}

      {wouldApply.length > 0 && (
        <div className="rounded-2xl border border-[var(--status-good)]/30 bg-[var(--status-good)]/5 p-4 shadow-[var(--card-shadow)]">
          <div className="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--status-good)]">
            Would apply ({wouldApply.length})
          </div>
          <div className="flex flex-col gap-2">
            {wouldApply.map((d, i) => (
              <DecisionRow key={i} d={d} />
            ))}
          </div>
        </div>
      )}

      {wouldSkip.length > 0 && (
        <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
          <div className="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
            Would skip ({wouldSkip.length})
          </div>
          <div className="flex flex-col gap-2">
            {wouldSkip.map((d, i) => (
              <DecisionRow key={i} d={d} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function DecisionRow({ d }: { d: RemediationDecision }) {
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <span className="font-mono text-[var(--ink-secondary)]">{d.cluster_id}</span>
          <span className="mx-1 text-[var(--ink-muted)]">·</span>
          <span className="font-mono">{d.source}</span>
          <span className="mx-1 text-[var(--ink-muted)]">→</span>
          <span className="font-mono">{d.destination}</span>
        </div>
        <span className="font-mono tabular-nums text-[var(--ink)]">{formatINR(d.savings_high_inr)}</span>
      </div>
      <div className="mt-1 text-[var(--ink-muted)]">{d.reasons.join("; ")}</div>
    </div>
  );
}

function ManifestCard({
  clusterId,
  source,
  destination,
  pathClass,
  cost,
  manifest,
}: {
  clusterId: string;
  source: string;
  destination: string;
  pathClass: string;
  cost: number;
  manifest: string;
}) {
  const [copied, setCopied] = useState(false);

  function copy() {
    navigator.clipboard.writeText(manifest).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }

  function download() {
    const blob = new Blob([manifest], { type: "text/yaml" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${clusterId}-${destination.replace(/[^a-z0-9.]/gi, "-")}.yaml`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm">
          <span className="font-mono text-xs text-[var(--ink-secondary)]">{clusterId}</span>
          <span className="mx-1.5 text-[var(--ink-muted)]">·</span>
          <span className="font-mono text-xs">{source}</span>
          <span className="mx-1 text-[var(--ink-muted)]">→</span>
          <span className="font-mono text-xs">{destination}</span>
        </div>
        <div className="flex items-center gap-2 text-xs text-[var(--ink-muted)]">
          <span>{PATH_CLASS_LABEL[pathClass] ?? pathClass}</span>
          <span className="font-mono tabular-nums text-[var(--ink)]">{formatINR(cost)}</span>
        </div>
      </div>
      <pre className="overflow-x-auto whitespace-pre rounded-lg border border-[var(--border)] bg-[var(--page)] p-3 font-mono text-xs">
        {manifest}
      </pre>
      <div className="mt-2 flex gap-2">
        <button
          onClick={copy}
          className="rounded-lg border border-[var(--border)] px-3 py-1 text-xs text-[var(--ink-secondary)]"
        >
          {copied ? "Copied!" : "Copy"}
        </button>
        <button
          onClick={download}
          className="flex items-center gap-1 rounded-lg border border-[var(--border)] px-3 py-1 text-xs text-[var(--ink-secondary)]"
        >
          <IconDownload className="h-3 w-3" />
          Download
        </button>
      </div>
    </div>
  );
}
