'use client';

import { useEffect, useRef } from 'react';

import { cn } from '@repo/ui/lib';

// Fires a little before the section's top reaches the bottom edge, so the rise is seen, not missed.
const REVEAL_MARGIN = '0px 0px -10% 0px';

interface RevealProps {
  children: React.ReactNode;
  className?: string;
}

/*
 * Fades a landing section up once as it first enters the screen. It only ever hides a section that
 * is still below the fold after hydration, so the content is visible without JavaScript, under
 * reduced motion and for whatever the page opened on. The state lives on the DOM node rather than in
 * React state: it changes nothing React renders.
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
    <div
      ref={ref}
      className={cn(
        'transition-[opacity,translate] duration-500 ease-out-soft motion-reduce:transition-none',
        'data-[reveal=waiting]:translate-y-4 data-[reveal=waiting]:opacity-0',
        className,
      )}
    >
      {children}
    </div>
  );
}
