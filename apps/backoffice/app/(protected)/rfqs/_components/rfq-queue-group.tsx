'use client';

import { useId } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';

import { DropdownChevron } from '@repo/ui/components';
import { cn, MOTION, SPRING, STAGGER } from '@repo/ui/lib';
import { RfqQueueCard } from '@/app/(protected)/rfqs/_components/rfq-queue-card';
import type { QueueStatusGroup } from '@/app/(protected)/rfqs/_components/rfq-queue-groups';
import { STATUS_DOT } from '@/app/(protected)/rfqs/_components/rfq-status-badge';

// Cards that show behind the top of a folded stack, and how far each one shows below the last.
const PEEK_COUNT = 2;
const PEEK_OFFSET_PX = 8;
// Each card behind is this much narrower and this much fainter than the one in front of it.
const PEEK_SCALE_STEP = 0.05;
const PEEK_FADE_STEP = 0.2;
// A card arriving as the stack unfolds drops in from just above its place.
const ENTER_OFFSET_PX = -8;

interface RfqQueueGroupProps {
  group: QueueStatusGroup;
  expanded: boolean;
  activeRfqId: string | null;
  onToggle: () => void;
}

/*
 * One status in the queue column, folded into a stack the way a phone stacks its notifications: the
 * top card in front, two edges peeking behind it, and a press on that card fans the rest out.
 *
 * Cards travel by `layout="position"` only: a size animation would scale-correct the text inside,
 * so the edges behind take the top card's box at once. Springs rather than durations, so a press
 * mid-flight reverses from where the cards are.
 */
export function RfqQueueGroup({ group, expanded, activeRfqId, onToggle }: RfqQueueGroupProps) {
  const t = useTranslations('rfqs');
  const reduced = useReducedMotion();
  const headingId = useId();
  const listId = useId();

  const { status, records } = group;
  const stackable = records.length > 1;
  const stacked = stackable && !expanded;
  const peeks = Math.min(records.length - 1, PEEK_COUNT);
  const visible = stacked ? records.slice(0, peeks + 1) : records;
  const spring = reduced ? { duration: 0 } : expanded ? SPRING.expand : SPRING.collapse;

  return (
    <motion.div
      layout={reduced ? false : 'position'}
      transition={spring}
      role="group"
      aria-labelledby={headingId}
      className="flex flex-col gap-y-1.5"
    >
      <div className="flex min-h-7 items-center justify-between pl-3 gap-x-2">
        <h3
          id={headingId}
          className="flex items-center gap-x-1.5 text-paragraph-xs-medium text-foreground-muted"
        >
          <span
            aria-hidden="true"
            className={cn('size-1.5 shrink-0 rounded-full', STATUS_DOT[status])}
          />
          {t(`status.${status}`)}
          <span className="text-foreground-subtle tabular-nums">{records.length}</span>
        </h3>
        {stackable ? (
          <button
            type="button"
            aria-expanded={expanded}
            aria-controls={listId}
            onClick={onToggle}
            className="group/toggle flex items-center px-1 gap-x-1 rounded-sm outline-none transition-colors duration-200 ease-out-soft hover:text-foreground focus-visible:text-foreground text-paragraph-mini-medium text-foreground-muted"
          >
            {expanded ? t('list.groups.collapse') : t('list.groups.expand')}
            <DropdownChevron
              open={expanded}
              className="size-3.5 group-focus-visible/toggle:animate-focus-bump-soft"
            />
          </button>
        ) : null}
      </div>
      {/* The padding is the room the peeking edges take below the top card. */}
      <motion.ul
        id={listId}
        initial={false}
        animate={{ paddingBottom: stacked ? peeks * PEEK_OFFSET_PX : 0 }}
        transition={spring}
        className="flex flex-col gap-y-1.5 relative"
      >
        <AnimatePresence mode="popLayout" initial={false}>
          {visible.map((rfq, index) => {
            const depth = stacked ? index : 0;
            const behind = depth > 0;

            return (
              <motion.li
                key={rfq.id}
                layout={reduced ? false : 'position'}
                initial={{ opacity: 0, y: ENTER_OFFSET_PX }}
                animate={{
                  opacity: 1 - depth * PEEK_FADE_STEP,
                  scale: 1 - depth * PEEK_SCALE_STEP,
                  y: depth * PEEK_OFFSET_PX,
                }}
                exit={{ opacity: 0, transition: { duration: reduced ? 0 : MOTION.fast } }}
                transition={{
                  ...spring,
                  delay: expanded && !reduced ? Math.min(index, STAGGER.max) * STAGGER.step : 0,
                }}
                inert={behind}
                aria-hidden={behind || undefined}
                // Each card sits under the one before it; behind the top one it takes the top card's
                // box and hides its content, so only an edge shows, whatever its own height.
                style={{
                  zIndex: visible.length - index,
                  transformOrigin: 'bottom center',
                  bottom: behind ? peeks * PEEK_OFFSET_PX : undefined,
                }}
                className={cn(
                  behind && 'pointer-events-none [&_button>*]:invisible absolute inset-x-0 top-0',
                )}
              >
                <RfqQueueCard
                  rfq={rfq}
                  active={rfq.id === activeRfqId}
                  stackCount={stacked && index === 0 ? records.length : undefined}
                  onExpand={onToggle}
                />
              </motion.li>
            );
          })}
        </AnimatePresence>
      </motion.ul>
    </motion.div>
  );
}
