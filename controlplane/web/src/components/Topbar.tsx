import { useState, type ComponentType } from "react";
import { AnimatePresence, motion } from "framer-motion";
import DecryptedTextRaw from "./DecryptedText";
import { IconCalendar, IconDownload, IconFilter, IconSun } from "../icons";

const DecryptedText = DecryptedTextRaw as ComponentType<any>;
import type { DashboardSummary } from "../types";

function greeting(): string {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 17) return "Good afternoon";
  return "Good evening";
}

const tapPress = { whileTap: { scale: 0.96 }, whileHover: { y: -1 } };

export function Topbar({ data }: { data: DashboardSummary | null }) {
  const [soonMsg, setSoonMsg] = useState<string | null>(null);

  function showSoon(label: string) {
    setSoonMsg(label);
    window.setTimeout(() => setSoonMsg(null), 2000);
  }

  function downloadSnapshot() {
    if (!data) return;
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `chidrixx-dashboard-${new Date().toISOString().slice(0, 19)}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const today = new Date().toLocaleDateString(undefined, { day: "2-digit", month: "short", year: "numeric" });

  return (
    <header className="mb-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <DecryptedText
              text={greeting()}
              animateOn="view"
              sequential
              speed={28}
              className="text-[var(--ink)]"
              encryptedClassName="text-[var(--ink-muted)]"
            />
            <motion.span
              animate={{ rotate: 360 }}
              transition={{ duration: 16, repeat: Infinity, ease: "linear" }}
              className="inline-flex"
            >
              <IconSun className="h-5 w-5 text-[var(--accent)]" />
            </motion.span>
          </h1>
          <p className="mt-0.5 text-sm text-[var(--ink-muted)]">
            Real-time network cost attribution across{" "}
            {data ? `${data.summary.ClusterCount} cluster${data.summary.ClusterCount === 1 ? "" : "s"}` : "your clusters"}.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <motion.button
            {...tapPress}
            onClick={() => showSoon("Custom date ranges — coming soon (data is cumulative-since-agent-start today)")}
            className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]"
          >
            <IconCalendar className="h-3.5 w-3.5" />
            {today}
          </motion.button>
          <span className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]">
            <span className="relative flex h-1.5 w-1.5">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--accent)] opacity-60" />
              <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />
            </span>
            Live · refreshes every 15s
          </span>
          <motion.button
            {...tapPress}
            onClick={() => showSoon("Dimension filters — coming soon")}
            className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]"
          >
            <IconFilter className="h-3.5 w-3.5" />
            Filters
          </motion.button>
          <motion.button
            {...tapPress}
            onClick={downloadSnapshot}
            disabled={!data}
            title="Download this snapshot as JSON"
            className="flex items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--surface)] p-1.5 text-[var(--ink-secondary)] shadow-[var(--card-shadow)] disabled:opacity-40"
          >
            <IconDownload className="h-4 w-4" />
          </motion.button>
        </div>
      </div>

      <AnimatePresence>
        {soonMsg && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            className="mt-2 inline-flex items-center gap-2 rounded-lg bg-[var(--surface-sunken)] px-3 py-1.5 text-xs text-[var(--ink-secondary)]"
          >
            {soonMsg}
          </motion.div>
        )}
      </AnimatePresence>
    </header>
  );
}
