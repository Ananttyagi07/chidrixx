import type { CloudSpend } from "../types";
import { DonutChart, type DonutSlice } from "./DonutChart";
import { CATEGORICAL } from "../palette";

const SLOT_ORDER = Object.keys(CATEGORICAL) as Array<keyof typeof CATEGORICAL>;

// Real infrastructure: every agent ships the Cloud/Region it was
// configured with (from its own price book), and this groups real spend
// by that value -- controlplane/store.go's SpendByCloud. Only one price
// book (AWS) is in use across any agent today, so this correctly shows a
// single 100% slice; it becomes a genuine multi-cloud breakdown the
// moment a second cluster ships with a different one, not before.
export function SpendByProviderCard({ clouds }: { clouds: CloudSpend[] }) {
  const slices: DonutSlice[] = clouds.map((c, i) => ({
    key: `${c.Cloud}/${c.Region}`,
    label: c.Cloud === "unknown" ? "Unknown (pre-upgrade data)" : `${c.Cloud} · ${c.Region}`,
    value: c.CostHighINR,
    count: c.FindingCount,
    color: CATEGORICAL[SLOT_ORDER[i % SLOT_ORDER.length]].light,
  }));

  return <DonutChart slices={slices} centerLabel="total spend" />;
}
