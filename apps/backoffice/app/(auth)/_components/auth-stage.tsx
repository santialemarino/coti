'use client';

import { AnimatePresence, motion, useReducedMotion } from 'motion/react';

import { MOTION } from '@repo/ui/lib';

interface AuthStageProps {
  /* Changing this key is what triggers the crossfade, so it must name the stage. */
  stageKey: string;
  children: React.ReactNode;
}

/*
 * Crossfades between the stages of an auth flow — the form, then the result. `mode="wait"` holds the
 * incoming stage until the outgoing one has finished leaving, so the two never overlap and the card
 * never jumps height mid-swap.
 *
 * Without this the result would replace the form on the same frame: the most visible moment in the
 * flow, and the one place a hard cut is most obvious.
 */
export function AuthStage({ stageKey, children }: AuthStageProps) {
  const reduced = useReducedMotion();

  return (
    <AnimatePresence mode="wait" initial={false}>
      <motion.div
        key={stageKey}
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        /*
         * Zeroed rather than dropped: motion inlines its own styles, so swapping the element out
         * under reduced motion would strand a stage at opacity 0.
         */
        transition={{ duration: reduced ? 0 : MOTION.default }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  );
}
