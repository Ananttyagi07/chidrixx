import { useEffect, useRef } from "react";
import { animate, motion, useMotionValue, useTransform } from "framer-motion";

// Animates a real numeric value counting from its previous value to its
// new one whenever data refreshes (every 15s) — the number itself is
// never invented, only its transition is animated.
export function AnimatedNumber({
  value,
  format,
  className,
}: {
  value: number;
  format: (n: number) => string;
  className?: string;
}) {
  const motionValue = useMotionValue(value);
  const rounded = useTransform(motionValue, (v) => format(v));
  const prev = useRef(value);

  useEffect(() => {
    const controls = animate(motionValue, value, {
      duration: 0.6,
      ease: [0.16, 1, 0.3, 1],
    });
    prev.current = value;
    return controls.stop;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  return <motion.span className={className}>{rounded}</motion.span>;
}

// Same idea for a low–high range (total spend, potential savings) --
// both ends animate independently, composed by `format`.
export function AnimatedRange({
  low,
  high,
  format,
  className,
}: {
  low: number;
  high: number;
  format: (lo: number, hi: number) => string;
  className?: string;
}) {
  const lowMV = useMotionValue(low);
  const highMV = useMotionValue(high);
  const composed = useTransform([lowMV, highMV], ([l, h]) => format(l as number, h as number));

  useEffect(() => {
    const a = animate(lowMV, low, { duration: 0.6, ease: [0.16, 1, 0.3, 1] });
    const b = animate(highMV, high, { duration: 0.6, ease: [0.16, 1, 0.3, 1] });
    return () => {
      a.stop();
      b.stop();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [low, high]);

  return <motion.span className={className}>{composed}</motion.span>;
}
