'use client';

import { CheckIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { cn } from '@repo/ui/lib';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

/*
 * Simplified lifecycle shown in the detail view. Only the five states the seller
 * cares about — intermediate states (CHANGE_REQUESTED, REJECTED) are
 * omitted from the visual timeline. DRAFT is internal (see normalizeRfqStatus) and
 * never reaches this component as a business state.
 */
const TIMELINE_STATES = ['RECEIVED', 'GENERATED', 'QUOTED', 'SENT', 'ACCEPTED'] as const;

type TimelineState = (typeof TIMELINE_STATES)[number];

/*
 * Maps each simplified state to the best available timestamp from the API response.
 * States that haven't happened yet return null.
 */
function stateDate(state: TimelineState, detail: RfqDetailResponse): string | null {
  const { rfq, quote } = detail;
  const currentStatus = normalizeRfqStatus(rfq.status);

  const stateRank: Record<string, number> = {
    RECEIVED: 0,
    GENERATED: 1,
    QUOTED: 3,
    SENT: 4,
    CHANGE_REQUESTED: 5,
    ACCEPTED: 6,
    REJECTED: 7,
  };

  const currentRank = stateRank[currentStatus] ?? -1;

  switch (state) {
    case 'RECEIVED':
      return rfq.created_at;
    case 'GENERATED':
      return rfq.created_at;
    case 'QUOTED':
      return quote?.created_at ?? null;
    case 'SENT':
      return currentRank >= 4 && quote ? quote.updated_at : null;
    case 'ACCEPTED':
      return currentRank >= 6 && quote ? quote.updated_at : null;
    default:
      return null;
  }
}

interface RfqStatusTimelineProps {
  detail: RfqDetailResponse;
}

export function RfqStatusTimeline({ detail }: RfqStatusTimelineProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const { rfq, version } = detail;

  const currentStatus = normalizeRfqStatus(rfq.status);
  const currentRank = TIMELINE_STATES.indexOf(currentStatus as TimelineState);

  return (
    <div className="flex flex-col gap-y-3">
      {/* Horizontal stepper */}
      <div className="flex items-center">
        {TIMELINE_STATES.map((state, index) => {
          const isCurrent = state === currentStatus;
          const isPast = currentRank >= 0 && index < currentRank;
          const date = stateDate(state, detail);

          return (
            <div key={state} className="flex flex-1 items-center last:flex-none">
              {/* Dot + label + date */}
              <div className="flex flex-col items-center gap-y-1">
                <div
                  className={cn(
                    'flex size-5 items-center justify-center rounded-full border-2 transition-colors',
                    isCurrent
                      ? 'border-primary bg-primary'
                      : isPast
                        ? 'border-primary bg-primary/20'
                        : 'border-border bg-background',
                  )}
                >
                  {isCurrent && <div className="size-1.5 rounded-full bg-primary-foreground" />}
                  {isPast && <CheckIcon className="size-2.5 text-primary" />}
                </div>
                <span
                  className={cn(
                    'whitespace-nowrap text-center text-paragraph-mini',
                    isCurrent
                      ? 'font-medium text-foreground'
                      : isPast
                        ? 'text-foreground'
                        : 'text-foreground-subtle',
                  )}
                >
                  {t(`status.${state}`)}
                </span>
                {date && (
                  <span className="whitespace-nowrap text-center text-paragraph-mini text-foreground-muted">
                    {fmt.date(date)}
                  </span>
                )}
              </div>

              {/* Connector line */}
              {index < TIMELINE_STATES.length - 1 && (
                <div
                  className={cn(
                    'mx-1 h-px flex-1',
                    isPast || isCurrent ? 'bg-primary' : 'bg-border',
                  )}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Metadata row */}
      {version && (
        <div className="flex items-center gap-x-4 text-paragraph-xs text-foreground-muted">
          <span>v{version.version_number}</span>
          {version.total && (
            <>
              <span aria-hidden="true" className="text-foreground-subtle">
                ·
              </span>
              <span className="font-medium text-foreground">{fmt.currency(version.total)}</span>
            </>
          )}
        </div>
      )}
    </div>
  );
}
