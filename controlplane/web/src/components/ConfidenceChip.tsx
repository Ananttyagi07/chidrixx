import { CONFIDENCE_STATUS, STATUS } from "../palette";

// Status color, always paired with a label (never color alone) — light
// mode "warning"/"serious" are sub-3:1 contrast by design in the palette,
// so the text label is load-bearing, not decorative.
export function ConfidenceChip({ confidence }: { confidence: string }) {
  const status = CONFIDENCE_STATUS[confidence] ?? "warning";
  const color = STATUS[status].light;

  return (
    <span
      className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[0.68rem] font-mono"
      style={{ borderColor: `${color}4d`, color, background: `${color}1a` }}
    >
      <span className="h-1.5 w-1.5 rounded-full" style={{ background: color }} />
      {confidence}
    </span>
  );
}
