'use client';

import * as React from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';

import { EASE, MOTION } from '../lib/motion';
import { cn } from '../lib/utils';
import { Button } from './button';
import { Spinner } from './spinner';

const MotionButton = motion.create(Button);

/* motion redefines the drag and animation handlers, so those DOM ones cannot be forwarded. */
type ForwardedButtonProps = Omit<
  React.ComponentProps<typeof Button>,
  'children' | 'onDrag' | 'onDragStart' | 'onDragEnd' | 'onAnimationStart'
>;

interface PendingButtonProps extends ForwardedButtonProps {
  pending?: boolean;
  /* Replaces the label, next to a spinner, while pending. Pass a translated string. */
  pendingLabel: string;
  children: React.ReactNode;
}

/*
 * The button for an action that takes long enough to say so. Its two labels are different widths, and
 * a content-driven `width: auto` cannot be transitioned in CSS at all — no `transition-*` will ever
 * animate it — so the resize is a layout animation and the labels crossfade through AnimatePresence.
 * Without both, the box jumps to the new width and the words swap on top of each other.
 *
 * `popLayout` rather than `wait`: it takes the outgoing label out of flow in the same commit that
 * swaps `pending`, so the box's new width is measured on the render where `layout` runs. Under `wait`
 * the incoming label mounts a beat later, in a commit this component never re-renders for, and the
 * resize snaps. The button is `relative` because a popped label is positioned against it.
 */
function PendingButton({
  pending = false,
  pendingLabel,
  children,
  disabled,
  className,
  ...props
}: PendingButtonProps) {
  const reduced = useReducedMotion();

  /*
   * Reduced motion keeps the same tree and zeroes the timings rather than rendering a plain button:
   * motion inlines its own styles during SSR, and a swapped element would strand them.
   */
  const transition = { duration: reduced ? 0 : MOTION.fast, ease: EASE.outSoft };

  return (
    <MotionButton
      layout={!reduced}
      transition={transition}
      disabled={disabled || pending}
      aria-busy={pending}
      className={cn('relative', className)}
      {...props}
    >
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={pending ? 'pending' : 'idle'}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={transition}
          className="inline-flex items-center gap-x-2 whitespace-nowrap"
        >
          {pending ? (
            <>
              <Spinner size="sm" />
              {pendingLabel}
            </>
          ) : (
            children
          )}
        </motion.span>
      </AnimatePresence>
    </MotionButton>
  );
}

export { PendingButton };
