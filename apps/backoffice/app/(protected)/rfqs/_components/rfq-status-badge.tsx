'use client';

import { useTranslations } from 'next-intl';

import { Spinner } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import type { RfqStatus } from '@/lib/api/rfqs';

// The order the domain documents them in, used by the tabs and by sorting.
export const STATUS_ORDER: readonly RfqStatus[] = [
  'RECEIVED',
  'GENERATED',
  'DRAFT',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED',
];

/*
 * The single mapping from a domain state to its colour, written out verbatim so Tailwind emits each
 * utility. The colour is inherited by the label and by the backdrop (bg-current at 20% opacity), so
 * a state always looks the same in the table and the filter tabs. RECEIVED has no colour of its own
 * (it never renders a badge — it always shows the ingestion spinner) and stays neutral.
 */
export const STATUS_COLOUR: Record<RfqStatus, string> = {
  RECEIVED: '',
  GENERATED: 'text-status-generated',
  DRAFT: 'text-status-draft',
  QUOTED: 'text-status-quoted',
  SENT: 'text-status-sent',
  CHANGE_REQUESTED: 'text-status-change-requested',
  ACCEPTED: 'text-status-accepted',
  REJECTED: 'text-status-rejected',
};

/*
 * Archivado is an orthogonal flag, not a lifecycle state (see docs/internal/domain/estados.md), so
 * it maps to its own neutral colour instead of joining the table above. The flag wins over the real
 * status when both are set.
 */
const ARCHIVED_COLOUR = 'text-status-archived';

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
}

export function RfqStatusBadge({
  status,
  processing = false,
  archived = false,
}: RfqStatusBadgeProps) {
  const t = useTranslations('rfqs');

  // RECEIVED always means the ingestion pipeline is still working on it. The container keeps the
  // badge's height so the row never jumps while a state resolves to a spinner.
  if (processing || status === 'RECEIVED') {
    return (
      <span className="inline-flex h-[22px] items-center gap-x-1.5 whitespace-nowrap text-paragraph-xs-medium text-foreground-muted">
        <Spinner size="xs" />
        {processing ? t('processing.quote') : t('processing.ingestion')}
      </span>
    );
  }

  /*
   * The Figma status label: the state colour paints the text and, through bg-current, the backdrop
   * at 20% opacity — a tinted chip, never a solid pill. The backdrop bleeds a hair past the box the
   * way the mockup draws it.
   */
  return (
    <span
      className={cn(
        'relative inline-flex h-[22px] items-center justify-center whitespace-nowrap px-1.5',
        archived ? ARCHIVED_COLOUR : STATUS_COLOUR[status],
      )}
    >
      <span
        aria-hidden="true"
        className="absolute inset-x-0 -top-[4.55%] h-[109.09%] rounded-[3px] bg-current opacity-20"
      />
      <span className="relative text-paragraph-xs-semibold">
        {t(archived ? 'status.ARCHIVED' : `status.${status}`)}
      </span>
    </span>
  );
}
