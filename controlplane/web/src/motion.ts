// Shared entrance/hover motion so every card in the dashboard moves the
// same way — real content still renders synchronously; this only
// animates its presentation.
export const cardMotion = {
  initial: { opacity: 0, y: 12 },
  animate: { opacity: 1, y: 0 },
  whileHover: { y: -3 },
  transition: { type: "spring" as const, stiffness: 300, damping: 24 },
};

export const container = {
  hidden: { opacity: 1 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.06 },
  },
};

export const item = {
  hidden: { opacity: 0, y: 14 },
  show: { opacity: 1, y: 0, transition: { type: "spring" as const, stiffness: 300, damping: 26 } },
};
