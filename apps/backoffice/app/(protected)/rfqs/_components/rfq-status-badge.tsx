'use client';

import { useTranslations } from 'next-intl';

import { Spinner } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import type { RfqStatus } from '@/lib/api/rfqs';

// The order the domain documents them in, used by the tabs and by sorting.
export const STATUS_ORDER: readonly RfqStatus[] = [
  'RECEIVED',
  'FAILED',
  'GENERATED',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED',
];

/*
 * The single mapping from a domain state to its colours, written out verbatim so Tailwind emits each
 * utility. Each state names a wash, a hairline and a label, exactly the way the badge's own
 * success/warning/danger tones are built — so a status pill is visibly one of Coti's pills that
 * happens to be purple, rather than a foreign object that happens to be on a Coti screen. RECEIVED
 * has no colours of its own: it never renders a badge, it always shows the ingestion spinner.
 */
export const STATUS_COLOUR: Record<RfqStatus, string> = {
  RECEIVED: '',
  FAILED: 'bg-status-failed-subtle border-status-failed-border text-status-failed-foreground',
  GENERATED:
    'bg-status-generated-subtle border-status-generated-border text-status-generated-foreground',
  QUOTED: 'bg-status-quoted-subtle border-status-quoted-border text-status-quoted-foreground',
  SENT: 'bg-status-sent-subtle border-status-sent-border text-status-sent-foreground',
  CHANGE_REQUESTED:
    'bg-status-change-requested-subtle border-status-change-requested-border text-status-change-requested-foreground',
  ACCEPTED:
    'bg-status-accepted-subtle border-status-accepted-border text-status-accepted-foreground',
  REJECTED:
    'bg-status-rejected-subtle border-status-rejected-border text-status-rejected-foreground',
};

/* The dot carries the full-strength hex, which is what keeps each state recognisable at a glance. */
const STATUS_DOT: Record<RfqStatus, string> = {
  RECEIVED: '',
  FAILED: 'bg-status-failed',
  GENERATED: 'bg-status-generated',
  QUOTED: 'bg-status-quoted',
  SENT: 'bg-status-sent',
  CHANGE_REQUESTED: 'bg-status-change-requested',
  ACCEPTED: 'bg-status-accepted',
  REJECTED: 'bg-status-rejected',
};

/*
 * Archivado is an orthogonal flag, not a lifecycle state (see docs/internal/domain/estados.md), so
 * it maps to its own neutral family instead of joining the table above. The flag wins over the real
 * status when both are set.
 */
const ARCHIVED_COLOUR =
  'bg-status-archived-subtle border-status-archived-border text-status-archived-foreground';
const ARCHIVED_DOT = 'bg-status-archived';

/*
 * Only statuses whose quote exists and is final enough to carry an amount show a total in the list;
 * the rest render "-". CHANGE_REQUESTED is included in the no-total set on purpose: until the
 * seller approves a number, the amount is not a promise the table should state.
 */
const QUOTE_TOTAL_STATUSES: ReadonlySet<RfqStatus> = new Set([
  'QUOTED',
  'SENT',
  'ACCEPTED',
  'REJECTED',
]);

export function hasQuoteTotal(status: RfqStatus): boolean {
  return QUOTE_TOTAL_STATUSES.has(status);
}

export interface RfqStatusBadgeProps {
  status: RfqStatus;
  // True while the AI is generating the quote; shows a spinner instead of the badge.
  processing?: boolean;
  // True for an archived pedido; shows the grey badge over the real status.
  archived?: boolean;
  // Compact variant for sidebars and tight spaces.
  size?: 'default' | 'sm';
}

export function RfqStatusBadge({
  status,
  processing = false,
  archived = false,
  size = 'default',
}: RfqStatusBadgeProps) {
  const t = useTranslations('rfqs');

  // RECEIVED always means the ingestion pipeline is still working on it. The container keeps the
  // badge's height so the row never jumps while a state resolves to a spinner.
  if (processing || status === 'RECEIVED') {
    return (
      <span
        className={cn(
          'inline-flex items-center gap-x-1.5 whitespace-nowrap text-foreground-muted',
          size === 'sm' ? 'h-4 text-paragraph-mini-medium' : 'h-[22px] text-paragraph-xs-medium',
        )}
      >
        <Spinner size="xs" />
        {processing ? t('processing.quote') : t('processing.ingestion')}
      </span>
    );
  }

  /*
   * Coti's pill, in the state's own colours: the same geometry, border and type scale as every other
   * badge in the app, with a full-strength dot so the hue still reads at a glance.
   */
  return (
    <span
      className={cn(
        'inline-flex w-fit shrink-0 items-center justify-center gap-x-1.5 border whitespace-nowrap rounded-full',
        'transition-[color,background-color,border-color] duration-200 ease-out-soft',
        size === 'sm'
          ? 'h-5 px-2 text-paragraph-mini-medium'
          : 'h-6 px-2.5 text-paragraph-xs-medium',
        archived ? ARCHIVED_COLOUR : STATUS_COLOUR[status],
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          archived ? ARCHIVED_DOT : STATUS_DOT[status],
        )}
      />
      {t(archived ? 'status.ARCHIVED' : `status.${status}`)}
    </span>
  );
}
