'use client';

import * as React from 'react';
import { ArrowDownIcon } from 'lucide-react';
import { animate, useReducedMotion, type AnimationPlaybackControls } from 'motion/react';

import { EASE, MOTION } from '../lib/motion';
import { cn } from '../lib/utils';
import { Button } from './button';
import { Tooltip, TooltipContent, TooltipTrigger } from './tooltip';

/* The sticky header and the phone tab bar cover this much of each edge; behind them is not "in view". */
const EDGE_COVER_PX = 64;
/* Breathing room left between the arrival point and the edge of the screen. */
const ARRIVAL_GAP_PX = 24;

interface JumpToButtonProps {
  /* What the button takes the caller to, e.g. a form's submit row. Focus lands on it on arrival. */
  target: React.RefObject<HTMLElement | null>;
  label: string;
}

/*
 * A floating arrow, shown only while `target` is off screen, that scrolls the page to it — down
 * when it is below, up when it is above — and moves focus there, so a keyboard caller continues
 * from the action itself. The scroll is eased with motion rather than native smooth scrolling, whose
 * curve and length the browser picks, and any wheel or touch hands control back to the caller.
 */
function JumpToButton({ target, label }: JumpToButtonProps) {
  const reduced = useReducedMotion();
  const [direction, setDirection] = React.useState<'up' | 'down' | null>(null);
  const flight = React.useRef<AnimationPlaybackControls | null>(null);

  React.useEffect(() => {
    const element = target.current;
    if (!element) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry) return;
        if (entry.isIntersecting) setDirection(null);
        else setDirection(entry.boundingClientRect.top > 0 ? 'down' : 'up');
      },
      { rootMargin: `-${EDGE_COVER_PX}px 0px -${EDGE_COVER_PX}px 0px` },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [target]);

  React.useEffect(() => {
    const cancel = () => flight.current?.stop();
    window.addEventListener('wheel', cancel, { passive: true });
    window.addEventListener('touchstart', cancel, { passive: true });
    return () => {
      cancel();
      window.removeEventListener('wheel', cancel);
      window.removeEventListener('touchstart', cancel);
    };
  }, []);

  function jump() {
    const element = target.current;
    if (!element) return;
    const box = element.getBoundingClientRect();
    const bottom = document.documentElement.scrollHeight - window.innerHeight;
    const destination =
      direction === 'up'
        ? box.top + window.scrollY - EDGE_COVER_PX - ARRIVAL_GAP_PX
        : box.bottom + window.scrollY - window.innerHeight + EDGE_COVER_PX + ARRIVAL_GAP_PX;
    // Held to the page's own range, or the curve would run on past where the page stops.
    const top = Math.min(Math.max(destination, 0), bottom);
    flight.current?.stop();
    flight.current = animate(window.scrollY, top, {
      duration: reduced ? 0 : MOTION.slower,
      ease: EASE.inOutSoft,
      onUpdate: (y) => window.scrollTo(0, y),
      onComplete: () => focusWithin(element),
    });
  }

  const shown = direction !== null;

  return (
    <div
      data-slot="jump-to-button"
      data-state={shown ? 'visible' : 'hidden'}
      inert={!shown}
      className={cn(
        'fixed right-4 bottom-[calc(1rem+var(--toast-lift,0px))] z-30 sm:right-6 sm:bottom-[calc(1.5rem+var(--toast-lift,0px))]',
        'transition-[opacity,translate,scale] duration-200 ease-out-soft motion-reduce:transition-none',
        shown
          ? 'translate-y-0 scale-100 opacity-100'
          : 'pointer-events-none translate-y-2 scale-90 opacity-0',
      )}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon-lg"
            aria-label={label}
            onClick={jump}
            className="rounded-full shadow-e3"
          >
            <ArrowDownIcon
              aria-hidden="true"
              className={cn(
                'size-5 transition-[rotate] duration-200 ease-out-soft',
                direction === 'up' ? 'rotate-180' : 'rotate-0',
              )}
            />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left" sideOffset={8}>
          {label}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

/*
 * The last enabled control in the target, since a row of actions ends with its primary one; with
 * none enabled, the target itself, so focus never falls back to the top of the page.
 */
function focusWithin(element: HTMLElement) {
  const controls = element.querySelectorAll<HTMLElement>(
    'button:not([disabled]), a[href], input:not([disabled])',
  );
  const focusable = controls[controls.length - 1];
  if (focusable) {
    focusable.focus({ preventScroll: true });
    return;
  }
  if (!element.hasAttribute('tabindex')) element.setAttribute('tabindex', '-1');
  element.focus({ preventScroll: true });
}

export { JumpToButton };
