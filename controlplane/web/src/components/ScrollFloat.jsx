import { useEffect, useMemo, useRef } from 'react';
import { gsap } from 'gsap';
import { ScrollTrigger } from 'gsap/ScrollTrigger';

import './ScrollFloat.css';

gsap.registerPlugin(ScrollTrigger);

const ScrollFloat = ({
  children,
  scrollContainerRef,
  containerClassName = '',
  textClassName = '',
  animationDuration = 1,
  ease = 'back.inOut(2)',
  scrollStart = 'center bottom+=50%',
  scrollEnd = 'bottom bottom-=40%',
  stagger = 0.03
}) => {
  const containerRef = useRef(null);

  const splitText = useMemo(() => {
    const text = typeof children === 'string' ? children : '';
    return text.split('').map((char, index) => (
      <span className="char" key={index}>
        {char === ' ' ? '\u00A0' : char}
      </span>
    ));
  }, [children]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const scroller = scrollContainerRef && scrollContainerRef.current ? scrollContainerRef.current : window;

    const charElements = el.querySelectorAll('.char');

    const tween = gsap.fromTo(
      charElements,
      {
        willChange: 'opacity, transform',
        opacity: 0,
        yPercent: 120,
        scaleY: 2.3,
        scaleX: 0.7,
        transformOrigin: '50% 0%'
      },
      {
        duration: animationDuration,
        ease: ease,
        opacity: 1,
        yPercent: 0,
        scaleY: 1,
        scaleX: 1,
        stagger: stagger,
        scrollTrigger: {
          trigger: el,
          scroller,
          start: scrollStart,
          end: scrollEnd,
          scrub: true
        }
      }
    );

    // A scrubbed ScrollTrigger computes its start/end pixel positions once,
    // against whatever the surrounding layout measures at the moment this
    // effect runs -- real, reproducible bug found live: real async layout
    // settling shortly after mount (a chart library's ResponsiveContainer
    // measuring itself via ResizeObserver a frame or two late, font
    // swap-in, etc.) leaves the cached trigger positions stale, freezing
    // the scrub partway through its character reveal (a garbled
    // half-revealed title) until a real scroll event happens to force
    // GSAP to recompute. Rather than guess a fixed delay, watch real
    // layout with a real ResizeObserver on the document body and refresh
    // ScrollTrigger whenever it actually fires, for a bounded window after
    // mount (2s -- generous next to real observed settle times, then
    // disconnected so this isn't a permanent per-title observer).
    const raf = requestAnimationFrame(() => ScrollTrigger.refresh());
    const settleTimers = [200, 600, 1200].map((ms) => setTimeout(() => ScrollTrigger.refresh(), ms));
    const onLoad = () => ScrollTrigger.refresh();
    window.addEventListener('load', onLoad, { once: true });

    const resizeObserver = new ResizeObserver(() => ScrollTrigger.refresh());
    resizeObserver.observe(document.body);
    const stopObserving = setTimeout(() => resizeObserver.disconnect(), 2000);

    return () => {
      cancelAnimationFrame(raf);
      settleTimers.forEach(clearTimeout);
      clearTimeout(stopObserving);
      resizeObserver.disconnect();
      window.removeEventListener('load', onLoad);
      tween.scrollTrigger && tween.scrollTrigger.kill();
      tween.kill();
    };
  }, [scrollContainerRef, animationDuration, ease, scrollStart, scrollEnd, stagger]);

  return (
    <h2 ref={containerRef} className={`scroll-float ${containerClassName}`}>
      <span className={`scroll-float-text ${textClassName}`}>{splitText}</span>
    </h2>
  );
};

export default ScrollFloat;
