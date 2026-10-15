'use client';

import * as React from 'react';

import { cn } from '../lib/utils';
import { Tooltip, TooltipContent, TooltipTrigger } from './tooltip';

const LINE_CLAMP = {
  1: 'line-clamp-1',
  2: 'line-clamp-2',
  3: 'line-clamp-3',
} as const;

interface ClampedTextProps {
  children: string;
  lines?: keyof typeof LINE_CLAMP;
  // Off inside a control that already takes focus; the tooltip then answers to the pointer only.
  focusable?: boolean;
  className?: string;
}

/*
 * Text that stops after a few lines with an ellipsis. The full text shows in a tooltip only when
 * something was actually cut, and only then does the text take focus, so a keyboard reaches it.
 * The tooltip is always mounted and held closed when nothing is cut: swapping the tree would
 * detach the element being measured.
 */
function ClampedText({ children, lines = 2, focusable = true, className }: ClampedTextProps) {
  const ref = React.useRef<HTMLSpanElement>(null);
  const [clamped, setClamped] = React.useState(false);

  React.useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () =>
      setClamped(
        element.scrollHeight > element.clientHeight + 1 ||
          element.scrollWidth > element.clientWidth + 1,
      );
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [children, lines]);

  return (
    <Tooltip open={clamped ? undefined : false}>
      <TooltipTrigger asChild>
        <span
          ref={ref}
          tabIndex={clamped && focusable ? 0 : undefined}
          className={cn(
            LINE_CLAMP[lines],
            'rounded-sm outline-none decoration-dotted underline-offset-2 wrap-anywhere',
            clamped && 'focus-visible:underline',
            className,
          )}
        >
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent>{children}</TooltipContent>
    </Tooltip>
  );
}

export { ClampedText };
