'use client';

import { useEffect, useRef } from 'react';

// Fires a little before the section's top reaches the bottom edge, so the rise is seen, not missed.
const REVEAL_MARGIN = '0px 0px -10% 0px';

interface RevealProps {
  children: React.ReactNode;
  className?: string;
}

/*
 * Holds a group of `revealItem`s that is still below the fold after hydration, and lets them rise in
 * the first time it scrolls into view. Everything else — the screen the page opened on, a visit
 * without JavaScript, reduced motion — keeps the entrance the items play on first paint. The state
 * lives on the DOM node rather than in React state: it changes nothing React renders.
 */
export function Reveal({ children, className }: RevealProps) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const node = ref.current;
    if (!node || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    if (node.getBoundingClientRect().top < window.innerHeight) return;

    node.dataset.reveal = 'waiting';
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry?.isIntersecting) return;
        node.dataset.reveal = 'shown';
        observer.disconnect();
      },
      { rootMargin: REVEAL_MARGIN },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return (
    <div ref={ref} className={className}>
      {children}
    </div>
  );
}
