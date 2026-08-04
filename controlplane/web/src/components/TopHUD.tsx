import { useEffect, useState, type ComponentType } from "react";
import { AnimatePresence, motion } from "framer-motion";
import DecryptedTextRaw from "./DecryptedText";
import { IconCalendar, IconDownload, IconFilter } from "../icons";
import { ChidrixxMark } from "./ChidrixxMark";
import { AnomalyAlertBell } from "./AnomalyAlertBell";
import { CommandPalette } from "./CommandPalette";
import { NAV_GROUPS } from "./CommandRail";
import type { Session } from "../session";
import type { DashboardSummary } from "../types";

const DecryptedText = DecryptedTextRaw as ComponentType<any>;

function currentLabel(active: string): string {
  for (const group of NAV_GROUPS) {
    const found = group.items.find((i) => i.id === active);
    if (found) return found.label;
  }
  return active === "settings" ? "Settings" : "Overview";
}

// The Top HUD: a single, precise instrument row -- replaces the old
// greeting + description header. No page title reads as "a document you
// scroll past once"; this reads as "a location you're currently at,"
// consistent with web/DESIGN_VISION.md's Mission Control framing. Every
// element here is flat, monochrome, and functional -- no glow, no
// decorative color.
export function TopHUD({
  active,
  session,
  data,
  onLogout,
  onNavigate,
}: {
  active: string;
  session: Session;
  data: DashboardSummary | null;
  onLogout: () => void;
  onNavigate: (id: string) => void;
}) {
  const [soonMsg, setSoonMsg] = useState<string | null>(null);
  const [accountOpen, setAccountOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);

  function showSoon(label: string) {
    setSoonMsg(label);
    window.setTimeout(() => setSoonMsg(null), 2000);
  }

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

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
    <>
      <header className="flex h-16 flex-shrink-0 items-center gap-3 border-b border-[var(--border)] bg-[var(--surface)] px-5">
        <div className="flex items-center gap-2">
          <ChidrixxMark className="h-6 w-6" />
          <span className="font-mono text-sm font-semibold tracking-tight">
            <DecryptedText
              text="chidrixx"
              animateOn="view"
              sequential
              speed={35}
              className="text-[var(--ink)]"
              encryptedClassName="text-[var(--ink-muted)]"
            />
          </span>
        </div>

        <div className="h-4 w-px bg-[var(--border)]" />
        <span className="font-mono text-[0.8rem] text-[var(--ink-muted)]">{currentLabel(active)}</span>

        <button
          onClick={() => setPaletteOpen(true)}
          className="mx-auto flex w-72 items-center gap-2 rounded-lg border border-[var(--border)] bg-[var(--page)] px-3 py-1.5 text-left text-xs text-[var(--ink-muted)] transition-colors hover:border-[var(--ink-faint)]"
        >
          <svg viewBox="0 0 20 20" className="h-3.5 w-3.5 flex-shrink-0" fill="none" stroke="currentColor" strokeWidth={1.6}>
            <circle cx="9" cy="9" r="6" />
            <path d="M17 17l-4-4" strokeLinecap="round" />
          </svg>
          <span className="flex-1">Search or ask…</span>
          <span className="rounded border border-[var(--border)] px-1 font-mono text-[0.6rem]">⌘K</span>
        </button>

        <div className="ml-auto flex items-center gap-2">
          <span className="flex items-center gap-1.5 rounded-lg border border-[var(--border)] bg-[var(--page)] px-2.5 py-1.5 text-[0.7rem] text-[var(--ink-muted)]">
            <span className="relative flex h-1.5 w-1.5">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--ink)] opacity-40" />
              <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-[var(--ink)]" />
            </span>
            Live · 15s
          </span>

          <button
            onClick={() => showSoon("Custom date ranges — coming soon (data is cumulative-since-agent-start today)")}
            title={today}
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--page)] text-[var(--ink-secondary)] hover:bg-[var(--surface-sunken)]"
          >
            <IconCalendar className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => showSoon("Dimension filters — coming soon")}
            title="Filters"
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--page)] text-[var(--ink-secondary)] hover:bg-[var(--surface-sunken)]"
          >
            <IconFilter className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={downloadSnapshot}
            disabled={!data}
            title="Download this snapshot as JSON"
            className="flex h-8 w-8 items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--page)] text-[var(--ink-secondary)] hover:bg-[var(--surface-sunken)] disabled:opacity-40"
          >
            <IconDownload className="h-3.5 w-3.5" />
          </button>

          <AnomalyAlertBell />

          <div className="relative">
            <button
              onClick={() => setAccountOpen((o) => !o)}
              title="Account menu"
              className="flex h-9 w-9 items-center justify-center rounded-full border border-[var(--border)] bg-[var(--page)] text-xs font-semibold text-[var(--ink)]"
            >
              {session.username.slice(0, 2).toUpperCase() || "??"}
            </button>
            <AnimatePresence>
              {accountOpen && (
                <motion.div
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -6 }}
                  className="absolute right-0 z-20 mt-2 w-48 rounded-lg border border-[var(--border)] bg-[var(--surface)] p-2 shadow-[var(--card-shadow)]"
                >
                  <div className="truncate px-2 py-1 text-xs font-medium text-[var(--ink)]">{session.username || "Unknown"}</div>
                  <div className="truncate px-2 pb-2 text-[0.7rem] capitalize text-[var(--ink-muted)]">{session.role || "unknown role"}</div>
                  <button
                    onClick={onLogout}
                    className="w-full rounded-md px-2 py-1.5 text-left text-xs text-[var(--ink-secondary)] hover:bg-[var(--surface-sunken)]"
                  >
                    Log out
                  </button>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </div>
      </header>

      <AnimatePresence>
        {soonMsg && (
          <motion.div
            initial={{ opacity: 0, y: -8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            className="pointer-events-none absolute right-5 top-[4.5rem] z-30 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-1.5 text-xs text-[var(--ink-secondary)] shadow-[var(--card-shadow)]"
          >
            {soonMsg}
          </motion.div>
        )}
      </AnimatePresence>

      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} onNavigate={onNavigate} />
    </>
  );
}
