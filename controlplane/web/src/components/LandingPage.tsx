import { useRef, useState, type ComponentType } from "react";
import { motion } from "framer-motion";
import RotatingTextRaw from "./RotatingText";
import VariableProximityRaw from "./VariableProximity";
import DecryptedTextRaw from "./DecryptedText";
import { ChidrixxMark } from "./ChidrixxMark";

// These three ship from the react-bits registry as plain .jsx with no
// generated types; cast once here rather than hand-maintaining prop
// types for vendored components we don't otherwise touch.
const RotatingText = RotatingTextRaw as ComponentType<any>;
const VariableProximity = VariableProximityRaw as ComponentType<any>;
const DecryptedText = DecryptedTextRaw as ComponentType<any>;
import {
  IconArrowRight,
  IconBell,
  IconChat,
  IconLayers,
  IconShareNetwork,
  IconShieldCheck,
  IconTrendingUp,
} from "../icons";

// Real feature phrases only — this is the one screen shown before any
// data has loaded, so it's the one place a fabricated claim would be
// most visible. Every line here matches something chidrixx actually does.
const FEATURES = [
  "attributes network cost to real Kubernetes workloads",
  "classifies every flow: same-node, cross-zone, cross-region, NAT, internet",
  "generates real NetworkPolicy manifests for wasteful paths",
  "watches every tenant for real cost anomalies in the background",
  "answers real questions about your infrastructure, grounded in tool calls",
];

const FEATURE_CARDS = [
  { icon: IconLayers, label: "Path classification", note: "SAME_NODE → INTERNET_EGRESS, priced per class from a real, cited price book" },
  { icon: IconShieldCheck, label: "Fix engine", note: "Real NetworkPolicy manifests, not just plain-text hints" },
  { icon: IconShareNetwork, label: "Multi-cluster", note: "One control plane, every real cluster shipping to it" },
  { icon: IconChat, label: "Grounded AI assistant", note: "Answers real questions via tool calls against your own data — never an invented number" },
  { icon: IconBell, label: "Proactive anomaly watch", note: "A background loop that notices a real cost spike before you go looking" },
  { icon: IconTrendingUp, label: "Placement simulator", note: "Real graph-partitioning math shows what re-zoning would actually save" },
];

const STEPS = [
  { n: "01", label: "Deploy the agent", note: "One DaemonSet, real eBPF hooks on cgroup_skb — no sidecars, no sampling." },
  { n: "02", label: "Every real byte gets classified & priced", note: "Same-node, cross-zone, cross-region, NAT, internet — each priced from a real price book." },
  { n: "03", label: "Real fixes get generated", note: "NetworkPolicy manifests you review and apply yourself. chidrixx never touches your cluster." },
];

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a href={href} className="text-sm text-[var(--ink-secondary)] transition-colors hover:text-[var(--ink)]">
      {children}
    </a>
  );
}

function DemoVideo() {
  const [errored, setErrored] = useState(false);

  if (errored) {
    return (
      <div className="flex aspect-video w-full flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-[var(--border)] bg-[var(--surface)] text-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-full border border-[var(--border)] text-[var(--ink-muted)]">
          <svg viewBox="0 0 20 20" className="h-5 w-5 translate-x-0.5" fill="currentColor">
            <path d="M6 4l10 6-10 6V4z" />
          </svg>
        </div>
        <div className="text-sm font-medium text-[var(--ink-secondary)]">Demo video coming soon</div>
      </div>
    );
  }

  return (
    <video
      className="aspect-video w-full rounded-2xl border border-[var(--border)] bg-[var(--ink)] shadow-[var(--card-shadow)]"
      controls
      preload="metadata"
      poster="/demo-poster.png"
      onError={() => setErrored(true)}
    >
      <source src="/demo.mp4" type="video/mp4" />
    </video>
  );
}

export function LandingPage({ onEnter }: { onEnter: () => void }) {
  const proximityRef = useRef<HTMLDivElement>(null);

  return (
    <div className="relative min-h-screen overflow-x-hidden bg-[var(--page)]">
      {/* A precise dot-grid reference plane, not a glow -- see
          DESIGN_VISION.md's "no glow, ever" rule. Liveliness here comes
          from motion (a slow, subtle drift), not light. */}
      <motion.div
        className="pointer-events-none absolute inset-0 h-[48rem] opacity-[0.5]"
        style={{
          backgroundImage: "radial-gradient(var(--ink-faint) 1px, transparent 1px)",
          backgroundSize: "28px 28px",
        }}
        animate={{ backgroundPosition: ["0px 0px", "28px 28px"] }}
        transition={{ duration: 18, repeat: Infinity, ease: "linear" }}
      />

      {/* Nav */}
      <header className="sticky top-0 z-20 border-b border-[var(--border)] bg-[var(--surface)]/90 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-6">
          <div className="flex items-center gap-2">
            <ChidrixxMark className="h-7 w-7" />
            <span className="font-mono text-sm font-semibold tracking-tight">chidrixx</span>
          </div>
          <nav className="ml-4 hidden items-center gap-6 sm:flex">
            <NavLink href="#features">Features</NavLink>
            <NavLink href="#how-it-works">How it works</NavLink>
            <NavLink href="#demo">Demo</NavLink>
          </nav>
          <div className="ml-auto flex items-center gap-3">
            <button onClick={onEnter} className="text-sm text-[var(--ink-secondary)] hover:text-[var(--ink)]">
              Log in
            </button>
            <motion.button
              onClick={onEnter}
              whileHover={{ scale: 1.03 }}
              whileTap={{ scale: 0.97 }}
              className="rounded-lg bg-[var(--ink)] px-4 py-2 text-sm font-semibold text-white"
            >
              Get started
            </motion.button>
          </div>
        </div>
      </header>

      {/* Hero */}
      <section className="relative z-10 mx-auto flex max-w-4xl flex-col items-center gap-6 px-6 pb-20 pt-16 text-center sm:pt-24">
        <ChidrixxMark className="h-40 w-60 sm:h-56 sm:w-80 md:h-64 md:w-96" />

        <div ref={proximityRef} className="select-none">
          <VariableProximity
            label="chidrixx"
            className="text-3d-grey text-6xl font-bold tracking-tight sm:text-8xl"
            style={{ fontFamily: "'Geist Variable', sans-serif" }}
            fromFontVariationSettings="'wght' 500"
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

        <div className="flex h-8 items-center text-base font-medium text-[var(--ink)] sm:text-lg">
          <RotatingText
            texts={FEATURES}
            rotationInterval={2800}
            staggerDuration={0.01}
            splitBy="words"
            mainClassName="justify-center"
          />
        </div>

        <div className="mt-2 flex flex-wrap items-center justify-center gap-3">
          <motion.button
            onClick={onEnter}
            whileHover={{ scale: 1.03 }}
            whileTap={{ scale: 0.97 }}
            className="flex items-center gap-2 rounded-xl bg-[var(--ink)] px-5 py-2.5 text-sm font-semibold text-white shadow-[var(--card-shadow)]"
          >
            View live dashboard
            <IconArrowRight className="h-4 w-4" />
          </motion.button>
          <a
            href="#demo"
            className="flex items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--surface)] px-5 py-2.5 text-sm font-semibold text-[var(--ink-secondary)] shadow-[var(--card-shadow)] hover:text-[var(--ink)]"
          >
            Watch demo
          </a>
        </div>
        <p className="text-xs text-[var(--ink-muted)]">
          Sign up for your own real tenant, or sign in if you already have one.
        </p>

        {/* Real product screenshot, tilted like a mission-control readout, no fabricated mockup */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, delay: 0.15, ease: [0.16, 1, 0.3, 1] }}
          className="mt-10 w-full max-w-4xl"
          style={{ perspective: "1600px" }}
        >
          <div
            className="overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)] shadow-[0_30px_60px_-20px_rgba(22,22,15,0.25)]"
            style={{ transform: "rotateX(8deg) rotateY(-6deg)", transformStyle: "preserve-3d" }}
          >
            <div className="flex items-center gap-1.5 border-b border-[var(--border)] bg-[var(--surface-sunken)] px-3 py-2">
              <span className="h-2.5 w-2.5 rounded-full bg-[var(--ink-faint)]" />
              <span className="h-2.5 w-2.5 rounded-full bg-[var(--ink-faint)]" />
              <span className="h-2.5 w-2.5 rounded-full bg-[var(--ink-faint)]" />
            </div>
            <img src="/dashboard-preview.png" alt="The real chidrixx dashboard, showing live-ingested cost data" className="w-full" />
          </div>
        </motion.div>
      </section>

      {/* Features */}
      <section id="features" className="relative z-10 bg-[var(--surface-grey)] py-20">
        <div className="mx-auto max-w-6xl px-6">
          <h2 className="text-center text-2xl font-semibold sm:text-3xl">What it actually does</h2>
          <p className="mx-auto mt-2 max-w-lg text-center text-sm text-[var(--ink-muted)]">
            Every capability below is shipped and running against real ingested data — none of this is a roadmap slide.
          </p>
          <div className="mt-10 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {FEATURE_CARDS.map(({ icon: Icon, label, note }) => (
              <div key={label} className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-4 text-left shadow-[var(--card-shadow)]">
                <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--page)]">
                  <Icon className="h-4 w-4 text-[var(--ink-secondary)]" />
                </div>
                <div className="mt-3 text-sm font-semibold text-[var(--ink)]">{label}</div>
                <div className="mt-1 text-xs leading-relaxed text-[var(--ink-muted)]">{note}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* How it works */}
      <section id="how-it-works" className="relative z-10 bg-[var(--page)] py-20">
        <div className="mx-auto max-w-4xl px-6">
          <h2 className="text-center text-2xl font-semibold sm:text-3xl">How it works</h2>
          <div className="mt-10 grid grid-cols-1 gap-6 sm:grid-cols-3">
            {STEPS.map((s) => (
              <div key={s.n} className="text-left">
                <div className="text-3d-grey text-4xl font-bold">{s.n}</div>
                <div className="mt-2 text-sm font-semibold text-[var(--ink)]">{s.label}</div>
                <div className="mt-1 text-xs leading-relaxed text-[var(--ink-muted)]">{s.note}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Demo */}
      <section id="demo" className="relative z-10 bg-[var(--surface-grey)] py-20">
        <div className="mx-auto max-w-3xl px-6">
          <h2 className="text-center text-2xl font-semibold sm:text-3xl">See it in action</h2>
          <p className="mx-auto mt-2 max-w-lg text-center text-sm text-[var(--ink-muted)]">
            A real walkthrough of the live dashboard.
          </p>
          <div className="mt-8">
            <DemoVideo />
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="relative z-10 border-t border-[var(--border)] bg-[var(--surface)] py-8">
        <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-3 px-6 sm:flex-row">
          <div className="flex items-center gap-2">
            <ChidrixxMark className="h-5 w-5" />
            <span className="font-mono text-xs font-semibold">chidrixx</span>
            <span className="text-xs text-[var(--ink-muted)]">— self-hosted, no billing system yet</span>
          </div>
          <a
            href="https://github.com/Ananttyagi07/chidrixx"
            target="_blank"
            rel="noreferrer"
            className="text-xs text-[var(--ink-muted)] hover:text-[var(--ink)]"
          >
            GitHub
          </a>
        </div>
      </footer>
    </div>
  );
}
