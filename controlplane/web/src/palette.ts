// Validated default categorical palette from the dataviz skill
// (references/palette.md) — used unchanged, so no re-validation is
// needed; only substitute-and-rerun would require that.
//
// PathClass -> categorical slot is a FIXED, one-time mapping (never
// cycled, never reassigned) tied to the classifier's own declared order
// (agent/cmd/kharcha/classify.go's PathClass constants), not to a
// subjective "how costly does this feel" ranking.
export const CATEGORICAL = {
  blue: { light: "#2a78d6", dark: "#3987e5" },
  orange: { light: "#eb6834", dark: "#d95926" },
  aqua: { light: "#1baf7a", dark: "#199e70" },
  yellow: { light: "#eda100", dark: "#c98500" },
  magenta: { light: "#e87ba4", dark: "#d55181" },
  green: { light: "#008300", dark: "#008300" },
  violet: { light: "#4a3aa7", dark: "#9085e9" },
  red: { light: "#e34948", dark: "#e66767" },
} as const;

export const PATH_CLASS_COLOR: Record<string, keyof typeof CATEGORICAL> = {
  SAME_NODE: "blue",
  SAME_AZ: "orange",
  CROSS_AZ: "aqua",
  CROSS_REGION: "yellow",
  MANAGED_SERVICE: "magenta",
  PRIVATE_OFFCLUSTER: "green",
  NAT_EGRESS: "violet",
  INTERNET_EGRESS: "red",
};

export const PATH_CLASS_LABEL: Record<string, string> = {
  SAME_NODE: "Same node",
  SAME_AZ: "Same zone",
  CROSS_AZ: "Cross-zone",
  CROSS_REGION: "Cross-region",
  MANAGED_SERVICE: "Managed service",
  PRIVATE_OFFCLUSTER: "Private off-cluster",
  NAT_EGRESS: "NAT egress",
  INTERNET_EGRESS: "Internet egress",
};

// Status palette (fixed, never themed) — for confidence, never for
// identity. Always paired with an icon + label, never color alone: light
// mode "warning"/"serious" are sub-3:1 contrast by design.
export const STATUS = {
  good: { light: "#0ca30c", dark: "#0ca30c" },
  warning: { light: "#fab219", dark: "#fab219" },
  serious: { light: "#ec835a", dark: "#ec835a" },
  critical: { light: "#d03b3b", dark: "#d03b3b" },
} as const;

export const CONFIDENCE_STATUS: Record<string, keyof typeof STATUS> = {
  high: "good",
  med: "warning",
  low: "serious",
};

// Sequential blue ramp, light -> dark, for single-series magnitude
// (the spend trend line/area, cluster sparklines).
export const SEQUENTIAL_BLUE = {
  100: "#cde2fb",
  200: "#9ec5f4",
  300: "#6da7ec",
  400: "#3987e5",
  500: "#256abf",
  600: "#184f95",
  700: "#0d366b",
};

export function colorVar(name: keyof typeof CATEGORICAL): string {
  return `var(--series-${name})`;
}
