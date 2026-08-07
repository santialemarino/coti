'use client';

import { useCallback, useEffect, useLayoutEffect, useRef } from 'react';
import { useAnimate, useReducedMotion, type AnimationPlaybackControls } from 'motion/react';

import { EASE, MOTION } from '@repo/ui/lib';

interface AnimatedHeightProps {
  /* Changing this is what arms the box, and the only thing that does. */
  trigger: string;
  children: React.ReactNode;
}

/*
 * Grows and shrinks into the size its content just became, instead of snapping to it in a single
 * frame. The auth card is vertically centred, so a height that changes abruptly moves the card's top
 * edge with it and the whole screen appears to jump.
 *
 * The height is measured and animated directly rather than with motion's `layout`, which
 * scale-corrects its children and would squash a form's inputs mid-flight. It is animated
 * imperatively, from an explicit pair of keyframes, because the box has to be told where it is
 * leaving from and where it is going in one instruction — as two renders it pins to the old height
 * and the move that should follow never arrives.
 *
 * A new `trigger` only **arms** the box; the travel starts on the resize that follows. The two are
 * not the same moment: an atomic swap resizes in the commit the trigger changes, but a crossfade
 * holds the outgoing stage for its exit first, so measuring at trigger time would read the size the
 * box already has and conclude there was nothing to animate. Between swaps the box is left at its
 * natural height, so a message opening under a field still resizes the card live — that is already
 * smooth, and following it would only make the card lag behind it.
 */
export function AnimatedHeight({ trigger, children }: AnimatedHeightProps) {
  const reduced = useReducedMotion();
  const [box, animate] = useAnimate<HTMLDivElement>();
  const content = useRef<HTMLDivElement>(null);
  const restingHeight = useRef<number>(undefined);
  const armed = useRef(false);
  const travel = useRef<AnimationPlaybackControls>(undefined);

  /*
   * Arming also freezes the box at the size it has right now, before the browser paints again. The
   * content resizes either in this very commit (an atomic swap) or once the outgoing stage has left
   * (a crossfade), and a ResizeObserver notification lands a frame *after* that new size is painted
   * — so a box left loose flashes its destination height and then jumps back to travel from.
   */
  useLayoutEffect(() => {
    armed.current = true;
    if (reduced || !box.current || restingHeight.current === undefined) return;
    box.current.style.height = `${restingHeight.current}px`;
  }, [trigger, reduced, box]);

  const onResize = useCallback(() => {
    // Measured off the inner element, which never carries the animating height.
    const next = content.current?.offsetHeight;
    if (next === undefined) return;

    const previous = restingHeight.current;
    restingHeight.current = next;
    if (!armed.current || reduced || !box.current) return;
    armed.current = false;
    // Nothing to travel from on the first observation, which is what disarms the mount.
    if (previous === undefined || previous === next) {
      // Releases the freeze above when the stage that armed it turned out to be the same height.
      box.current.style.height = '';
      return;
    }

    const playback = animate(
      box.current,
      { height: [previous, next] },
      { duration: MOTION.slow, ease: EASE.outSoft },
    );
    travel.current = playback;
    void playback.then(() => {
      /*
       * Released, or the box would hold this height while the content under it kept changing — but
       * only by the travel still running. A second swap starting mid-flight supersedes this one,
       * and clearing the height then would drop the new animation on the frame it resolved.
       */
      if (travel.current === playback && box.current) box.current.style.height = '';
    });
  }, [animate, box, reduced]);

  /*
   * Read through a ref so the observer outlives every re-render. Re-observing delivers a fresh
   * measurement immediately, and that delivery would spend the arm on a size that has not changed
   * yet — which is exactly the state a crossfade is in while it holds the outgoing stage.
   */
  const latestResize = useRef(onResize);
  latestResize.current = onResize;

  useEffect(() => {
    const element = content.current;
    if (!element) return;

    const observer = new ResizeObserver(() => latestResize.current());
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  return (
    /*
     * Clipped, or the taller incoming stage spills past the card until the box catches up. `clip`
     * with a margin rather than `hidden`, so the card's own elevation is still painted outside the
     * border box — `hidden` would cut it off on all four sides, and it is what separates the card
     * from the page.
     */
    <div ref={box} className="overflow-clip [overflow-clip-margin:0.5rem]">
      <div ref={content}>{children}</div>
    </div>
  );
}
