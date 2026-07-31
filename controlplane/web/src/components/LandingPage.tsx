import { useRef, type ComponentType } from "react";
import { motion } from "framer-motion";
import RotatingTextRaw from "./RotatingText";
import VariableProximityRaw from "./VariableProximity";
import DecryptedTextRaw from "./DecryptedText";

// These three ship from the react-bits registry as plain .jsx with no
// generated types; cast once here rather than hand-maintaining prop
// types for vendored components we don't otherwise touch.
const RotatingText = RotatingTextRaw as ComponentType<any>;
const VariableProximity = VariableProximityRaw as ComponentType<any>;
const DecryptedText = DecryptedTextRaw as ComponentType<any>;
import { IconArrowRight, IconLayers, IconShieldCheck, IconTrendingUp } from "../icons";

// Real feature phrases only — this is the one screen shown before any
// data has loaded, so it's the one place a fabricated claim would be
// most visible. Every line here matches something chidrixx actually does.
const FEATURES = [
  "attributes network cost to real Kubernetes workloads",
  "classifies every flow: same-node, cross-zone, cross-region, NAT, internet",
  "generates real NetworkPolicy manifests for wasteful paths",
  "aggregates cost across every cluster shipping to it",
];

export function LandingPage({ onEnter }: { onEnter: () => void }) {
  const proximityRef = useRef<HTMLDivElement>(null);

  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center overflow-hidden bg-[var(--page)] px-6">
      <motion.div
        className="pointer-events-none absolute -left-40 -top-40 h-[28rem] w-[28rem] rounded-full bg-[var(--accent)] opacity-[0.08] blur-3xl"
        animate={{ x: [0, 30, 0], y: [0, 20, 0] }}
        transition={{ duration: 12, repeat: Infinity, ease: "easeInOut" }}
      />
      <motion.div
        className="pointer-events-none absolute -right-32 bottom-0 h-96 w-96 rounded-full bg-[var(--series-blue)] opacity-[0.07] blur-3xl"
        animate={{ x: [0, -20, 0], y: [0, -30, 0] }}
        transition={{ duration: 14, repeat: Infinity, ease: "easeInOut" }}
      />

      <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.6, ease: [0.16, 1, 0.3, 1] }}
        className="relative z-10 flex max-w-2xl flex-col items-center gap-6 text-center"
      >
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--accent-wash)]">
          <svg viewBox="0 0 20 20" className="h-8 w-8 text-[var(--accent)]" fill="none" stroke="currentColor" strokeWidth={1.6}>
            <path d="M10 2l7 3.7v8.6L10 18l-7-3.7V5.7L10 2z" strokeLinejoin="round" />
            <path d="M10 2v16M3 5.7l7 3.7 7-3.7" strokeLinejoin="round" />
          </svg>
        </div>

        <div ref={proximityRef} className="select-none">
          <VariableProximity
            label="chidrixx"
            className="text-5xl tracking-tight text-[var(--ink)] sm:text-6xl"
            style={{ fontFamily: "'Geist Variable', sans-serif" }}
            fromFontVariationSettings="'wght' 400"
            toFontVariationSettings="'wght' 800"
            containerRef={proximityRef}
            radius={140}
            falloff="exponential"
          />
        </div>

        <div className="text-lg text-[var(--ink-secondary)] sm:text-xl">
          <DecryptedText
            text="An eBPF agent that"
            animateOn="view"
            sequential
            speed={22}
            className="text-[var(--ink)]"
            encryptedClassName="text-[var(--ink-muted)]"
          />
        </div>

        <div className="flex h-8 items-center text-base font-medium text-[var(--accent)] sm:text-lg">
          <RotatingText
            texts={FEATURES}
            rotationInterval={2800}
            staggerDuration={0.01}
            splitBy="words"
            mainClassName="justify-center"
          />
        </div>

        <div className="mt-2 grid grid-cols-1 gap-3 text-left sm:grid-cols-3">
          {[
            { icon: IconLayers, label: "Path classification", note: "SAME_NODE → INTERNET_EGRESS, priced per class" },
            { icon: IconShieldCheck, label: "Fix engine", note: "Real NetworkPolicy manifests, not just hints" },
            { icon: IconTrendingUp, label: "Multi-cluster", note: "One control plane, every cluster shipping to it" },
          ].map(({ icon: Icon, label, note }) => (
            <div
              key={label}
              className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-3 shadow-[var(--card-shadow)]"
            >
              <Icon className="h-4 w-4 text-[var(--accent)]" />
              <div className="mt-1.5 text-xs font-semibold text-[var(--ink)]">{label}</div>
              <div className="mt-0.5 text-[0.7rem] text-[var(--ink-muted)]">{note}</div>
            </div>
          ))}
        </div>

        <motion.button
          onClick={onEnter}
          whileHover={{ scale: 1.03 }}
          whileTap={{ scale: 0.97 }}
          className="mt-4 flex items-center gap-2 rounded-xl bg-[var(--accent)] px-5 py-2.5 text-sm font-semibold text-white shadow-lg shadow-[var(--accent)]/25"
        >
          View live dashboard
          <IconArrowRight className="h-4 w-4" />
        </motion.button>
        <p className="text-xs text-[var(--ink-muted)]">
          You'll be asked for the shared dashboard token — no account, no signup.
        </p>
      </motion.div>
    </div>
  );
}
