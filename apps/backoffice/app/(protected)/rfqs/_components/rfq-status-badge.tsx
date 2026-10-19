'use client';

import { useTranslations } from 'next-intl';

import { Spinner } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import type { RfqStatus } from '@/lib/api/rfqs';
import { STATUS_COLOUR, STATUS_DOT } from '@/lib/constants/rfq-status';

/*
 * Archivado is an orthogonal flag, not a lifecycle state (see docs/internal/domain/estados.md), so
 * it maps to its own neutral family instead of joining the table above. The flag wins over the real
 * status when both are set.
 */
const ARCHIVED_COLOUR =
  'bg-status-archived-subtle border-status-archived-border text-status-archived-foreground';
const ARCHIVED_DOT = 'bg-status-archived';

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
