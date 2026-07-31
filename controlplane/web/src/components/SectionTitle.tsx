import type { ComponentType, RefObject } from "react";
import ScrollFloatRaw from "./ScrollFloat";

const ScrollFloat = ScrollFloatRaw as ComponentType<any>;

// Section headings float up character-by-character, scrubbed to scroll
// position, as each one enters the viewport — ScrollFloat's default
// styling (huge centered display type) is overridden to match our small
// uppercase panel-label convention.
//
// scrollContainerRef must point at the actual scrolling element: the
// dashboard scrolls inside `main` (overflow-y-auto), not the window, and
// GSAP's ScrollTrigger defaults to tracking window scroll -- without this,
// it computes each title's reveal progress once against the static
// document position and then never updates it, since window itself never
// fires a scroll event. Titles below the first viewport would silently
// stay stuck at their initial (invisible) frame.
export function SectionTitle({
  children,
  scrollContainerRef,
}: {
  children: string;
  scrollContainerRef: RefObject<HTMLElement>;
}) {
  return (
    <ScrollFloat
      scrollContainerRef={scrollContainerRef}
      containerClassName="!m-0 !leading-none text-left"
      textClassName="!text-xs !font-medium !leading-none uppercase tracking-wide text-[var(--ink-muted)]"
      animationDuration={0.6}
      stagger={0.02}
      scrollStart="top bottom-=10%"
      scrollEnd="bottom center"
    >
      {children}
    </ScrollFloat>
  );
}
