import { Command } from "cmdk";
import { NAV_GROUPS } from "./CommandRail";

// A real ⌘K command palette (web/DESIGN_VISION.md §4.4) -- a genuine
// usability upgrade for a product with 16 real sections, not just
// decoration. Unstyled by design (cmdk ships no CSS), styled here to
// match the rest of the shell: flat, monochrome, no glow.
export function CommandPalette({
  open,
  onClose,
  onNavigate,
}: {
  open: boolean;
  onClose: () => void;
  onNavigate: (id: string) => void;
}) {
  return (
    <Command.Dialog
      open={open}
      onOpenChange={(v) => !v && onClose()}
      label="Command palette"
      overlayClassName="fixed inset-0 z-40 bg-[var(--ink)]/25"
      contentClassName="fixed left-1/2 top-[18vh] z-50 w-full max-w-xl -translate-x-1/2 overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--surface)] shadow-[var(--card-shadow)]"
      shouldFilter
    >
      <div className="flex items-center gap-2 border-b border-[var(--border)] px-3 py-2.5">
        <svg viewBox="0 0 20 20" className="h-4 w-4 flex-shrink-0 text-[var(--ink-muted)]" fill="none" stroke="currentColor" strokeWidth={1.6}>
          <circle cx="9" cy="9" r="6" />
          <path d="M17 17l-4-4" strokeLinecap="round" />
        </svg>
        <Command.Input
          autoFocus
          placeholder="Go to a section…"
          className="w-full bg-transparent text-sm text-[var(--ink)] outline-none placeholder:text-[var(--ink-muted)]"
        />
        <span className="rounded border border-[var(--border)] px-1 font-mono text-[0.6rem] text-[var(--ink-muted)]">esc</span>
      </div>
      <Command.List className="max-h-80 overflow-y-auto p-2">
        <Command.Empty className="px-2 py-6 text-center text-xs text-[var(--ink-muted)]">No matching section.</Command.Empty>
        {NAV_GROUPS.map((group) => (
          <Command.Group
            key={group.label}
            heading={group.label}
            className="mb-1 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-[0.65rem] [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.08em] [&_[cmdk-group-heading]]:text-[var(--ink-muted)]"
          >
            {group.items.map((navItem) => {
              const Icon = navItem.icon;
              return (
                <Command.Item
                  key={navItem.id}
                  value={navItem.label}
                  onSelect={() => {
                    onNavigate(navItem.id);
                    onClose();
                  }}
                  className="flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm text-[var(--ink-secondary)] data-[selected=true]:bg-[var(--surface-sunken)] data-[selected=true]:text-[var(--ink)]"
                >
                  <Icon className="h-4 w-4 flex-shrink-0" />
                  {navItem.label}
                </Command.Item>
              );
            })}
          </Command.Group>
        ))}
        <Command.Item
          value="Settings"
          onSelect={() => {
            onNavigate("settings");
            onClose();
          }}
          className="flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm text-[var(--ink-secondary)] data-[selected=true]:bg-[var(--surface-sunken)] data-[selected=true]:text-[var(--ink)]"
        >
          Settings
        </Command.Item>
      </Command.List>
    </Command.Dialog>
  );
}
