// Minimal inline line icons (no external icon package/CDN — keeps the
// bundle self-contained). Stroke-based, 20px, matching the sidebar's
// icon style in the reference design.
type IconProps = { className?: string };

const base = "stroke-current fill-none";

export function IconGrid({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <rect x="2.5" y="2.5" width="6" height="6" rx="1.2" />
      <rect x="11.5" y="2.5" width="6" height="6" rx="1.2" />
      <rect x="2.5" y="11.5" width="6" height="6" rx="1.2" />
      <rect x="11.5" y="11.5" width="6" height="6" rx="1.2" />
    </svg>
  );
}

export function IconBulb({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M10 2.5a5 5 0 00-3 9c.6.5 1 1.2 1 2v.5h4V13.5c0-.8.4-1.5 1-2a5 5 0 00-3-9z" strokeLinejoin="round" />
      <path d="M8 17h4" strokeLinecap="round" />
    </svg>
  );
}

export function IconSearch({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <circle cx="8.5" cy="8.5" r="5" />
      <path d="M16 16l-3.5-3.5" strokeLinecap="round" />
    </svg>
  );
}

export function IconLayers({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M10 2.5l7 3.5-7 3.5-7-3.5 7-3.5z" strokeLinejoin="round" />
      <path d="M3 10.5l7 3.5 7-3.5" strokeLinejoin="round" />
      <path d="M3 14l7 3.5 7-3.5" strokeLinejoin="round" />
    </svg>
  );
}

export function IconReceipt({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M5 2.5h10v15l-2-1.2-1.5 1.2-1.5-1.2-1.5 1.2-1.5-1.2-2 1.2v-15z" strokeLinejoin="round" />
      <path d="M7 6.5h6M7 9.5h6M7 12.5h4" strokeLinecap="round" />
    </svg>
  );
}

export function IconWallet({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <rect x="2.5" y="5" width="15" height="11" rx="1.5" />
      <path d="M2.5 8h15" />
      <circle cx="14" cy="11.5" r="1" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function IconShieldCheck({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M10 2.5l6 2.2v5c0 4-2.5 6.7-6 7.8-3.5-1.1-6-3.8-6-7.8v-5l6-2.2z" strokeLinejoin="round" />
      <path d="M7.3 10l1.8 1.8 3.6-3.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function IconTrendingUp({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M2.5 14.5l5-5 3 3 6.5-6.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M13.5 5.5h3.5V9" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function IconBell({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M5 8a5 5 0 0110 0c0 3.5 1 4.5 1 4.5h-12s1-1 1-4.5z" strokeLinejoin="round" />
      <path d="M8.5 15.5a1.7 1.7 0 003 0" strokeLinecap="round" />
    </svg>
  );
}

export function IconFile({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M6 2.5h5.5L15 6v11.5H6v-15z" strokeLinejoin="round" />
      <path d="M11.5 2.5V6H15" strokeLinejoin="round" />
      <path d="M8 10h4M8 13h4" strokeLinecap="round" />
    </svg>
  );
}

export function IconGear({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <circle cx="10" cy="10" r="2.6" />
      <path
        d="M10 3v1.6M10 15.4V17M17 10h-1.6M4.6 10H3M14.8 5.2l-1.1 1.1M6.3 13.7l-1.1 1.1M14.8 14.8l-1.1-1.1M6.3 6.3L5.2 5.2"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function IconSun({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <circle cx="10" cy="10" r="3.2" />
      <path d="M10 2.5v2M10 15.5v2M17.5 10h-2M4.5 10h-2M15.3 4.7l-1.4 1.4M6.1 13.9l-1.4 1.4M15.3 15.3l-1.4-1.4M6.1 6.1L4.7 4.7" strokeLinecap="round" />
    </svg>
  );
}

export function IconMoon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M15.5 12.8A6.5 6.5 0 017.2 4.5a6.5 6.5 0 108.3 8.3z" strokeLinejoin="round" />
    </svg>
  );
}

export function IconDownload({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M10 3v9.5M6.5 9l3.5 3.5L13.5 9" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M3.5 15.5h13" strokeLinecap="round" />
    </svg>
  );
}

export function IconFilter({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M3 4h14l-5.5 6.5V16l-3-1.6v-3.9L3 4z" strokeLinejoin="round" />
    </svg>
  );
}

export function IconCalendar({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <rect x="2.5" y="4" width="15" height="13" rx="1.5" />
      <path d="M2.5 8h15M6.5 2.5v3M13.5 2.5v3" strokeLinecap="round" />
    </svg>
  );
}

export function IconArrowRight({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M4 10h12M11 5.5L16 10l-5 4.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function IconConstruction({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" strokeWidth={1.6} className={`${base} ${className ?? ""}`}>
      <path d="M2.5 17l5-9 2 3.5 2-3.5 5 9z" strokeLinejoin="round" />
      <path d="M6 12h8" strokeLinecap="round" />
    </svg>
  );
}
