import { motion } from "framer-motion";
import { IconConstruction } from "../icons";
import { cardMotion } from "../motion";

// Used for any widget/page whose underlying feature genuinely doesn't
// exist yet (multi-cloud attribution, forecasting, budgets, anomaly
// detection, automations...) — shown honestly rather than filled with
// invented numbers. Matches the real widget's card footprint so the
// layout holds its shape.
export function ComingSoonCard({ title, note }: { title: string; note?: string }) {
  return (
    <motion.div
      {...cardMotion}
      className="flex h-full flex-col rounded-2xl border border-dashed border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">{title}</div>
      <div className="flex flex-1 flex-col items-center justify-center gap-2 py-6 text-center">
        <motion.div
          animate={{ y: [0, -4, 0] }}
          transition={{ duration: 2.4, repeat: Infinity, ease: "easeInOut" }}
        >
          <IconConstruction className="h-6 w-6 text-[var(--ink-muted)]" />
        </motion.div>
        <div className="text-sm font-medium text-[var(--ink-secondary)]">Coming soon</div>
        {note && <div className="max-w-[16rem] text-xs text-[var(--ink-muted)]">{note}</div>}
      </div>
    </motion.div>
  );
}
