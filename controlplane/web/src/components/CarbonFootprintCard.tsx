import { motion } from "framer-motion";
import { cardMotion } from "../motion";
import { AnimatedNumber } from "./AnimatedNumber";

// Real bytes transferred (already measured by the agent), multiplied by
// published, commonly-cited coefficients -- not measured, not sourced
// from any cloud provider's carbon API. Carbon-per-byte estimates are
// genuinely contested (published figures vary by 10-100x depending on
// methodology, energy mix, and what's counted), so this is labeled as a
// rough estimate with its formula always visible, the same way the price
// book's own numbers are labeled "list prices, not negotiated rates" --
// never presented with more precision than it actually has.
const KWH_PER_GB = 0.06; // widely-cited average network-transfer energy intensity
const GRID_G_CO2_PER_KWH = 475; // global average grid carbon intensity (IEA-range figure)

function estimateKgCO2e(totalBytes: number): number {
  const gb = totalBytes / 1e9;
  const kWh = gb * KWH_PER_GB;
  const gCO2e = kWh * GRID_G_CO2_PER_KWH;
  return gCO2e / 1000;
}

function formatCO2(kg: number): string {
  if (kg < 1) return `${(kg * 1000).toFixed(1)} g CO2e`;
  return `${kg.toFixed(2)} kg CO2e`;
}

export function CarbonFootprintCard({ totalBytes }: { totalBytes: number }) {
  const kg = estimateKgCO2e(totalBytes);

  return (
    <motion.div
      {...cardMotion}
      className="flex flex-col justify-between rounded-2xl border border-dashed border-[var(--border)] bg-[var(--surface)] p-4 shadow-[var(--card-shadow)]"
    >
      <div>
        <div className="text-xs font-medium uppercase tracking-wide text-[var(--ink-muted)]">
          Carbon footprint <span className="normal-case text-[var(--ink-muted)]">(estimate)</span>
        </div>
        <AnimatedNumber value={kg} format={formatCO2} className="mt-1.5 text-2xl font-semibold tabular-nums" />
      </div>
      <div className="mt-1.5 text-[0.68rem] leading-snug text-[var(--ink-muted)]">
        Rough estimate: {KWH_PER_GB} kWh/GB × {GRID_G_CO2_PER_KWH} g CO2e/kWh (global grid average). Not
        measured — actual intensity varies widely by provider, region, and energy mix.
      </div>
    </motion.div>
  );
}
