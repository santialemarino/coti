'use client';

import type { ComponentProps } from 'react';
import { CheckIcon, Clock3Icon, HistoryIcon, SendIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Badge } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import type {
  QuoteSendTrackingResponse,
  QuoteStatusChangeResponse,
  RfqChannel,
  RfqDetailResponse,
  RfqStatus,
  RfqStatusChangeResponse,
} from '@/lib/api/rfqs';
import { normalizeRfqStatus } from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';

const TIMELINE_STATES = ['RECEIVED', 'GENERATED', 'QUOTED', 'SENT', 'ACCEPTED'] as const;

type TimelineState = (typeof TIMELINE_STATES)[number];
type StatusSource = 'rfq' | 'quote';

interface StatusEvent {
  id: string;
  source: StatusSource;
  previousStatus: string | null;
  newStatus: string;
  changedAt: string;
}

function toRFQEvent(change: RfqStatusChangeResponse): StatusEvent {
  return {
    id: change.id,
    source: 'rfq',
    previousStatus: change.previous_status,
    newStatus: change.new_status,
    changedAt: change.changed_at,
  };
}

function toQuoteEvent(change: QuoteStatusChangeResponse): StatusEvent {
  return {
    id: change.id,
    source: 'quote',
    previousStatus: change.previous_status,
    newStatus: change.new_status,
    changedAt: change.changed_at,
  };
}

function statusEvents(detail: RfqDetailResponse): StatusEvent[] {
  return [
    ...(detail.rfq_status_history ?? []).map(toRFQEvent),
    ...(detail.quote_status_history ?? []).map(toQuoteEvent),
  ].sort((a, b) => Date.parse(a.changedAt) - Date.parse(b.changedAt));
}

function statusDate(state: TimelineState, detail: RfqDetailResponse): string | null {
  const found = statusEvents(detail)
    .filter((event) => normalizeRfqStatus(event.newStatus) === state)
    .at(-1);

  if (found) return found.changedAt;

  switch (state) {
    case 'RECEIVED':
      return detail.rfq.created_at;
    case 'GENERATED':
      return normalizeRfqStatus(detail.rfq.status) === 'RECEIVED' ? null : detail.rfq.created_at;
    case 'QUOTED':
      return detail.quote?.current_status === 'QUOTED' ? detail.quote.updated_at : null;
    case 'SENT':
      return detail.quote?.current_status === 'SENT' ? detail.quote.updated_at : null;
    case 'ACCEPTED':
      return detail.quote?.current_status === 'ACCEPTED' ? detail.quote.updated_at : null;
    default:
      return null;
  }
}

function stepperStatus(status: RfqStatus): TimelineState {
  if (status === 'CHANGE_REQUESTED' || status === 'REJECTED') return 'SENT';
  // FAILED is where reading the order stopped, so the stepper rests on the step it reached
  // instead of claiming a generation that never happened.
  if (status === 'FAILED') return 'RECEIVED';
  return status;
}

function statusLabelKey(status: string): string {
  return status.toUpperCase() === 'DRAFT' ? 'DRAFT' : normalizeRfqStatus(status);
}

function deliveryTime(delivery: QuoteSendTrackingResponse): string {
  return delivery.sent_at ?? delivery.created_at;
}

function deliveryTone(status: string): ComponentProps<typeof Badge>['tone'] {
  switch (status) {
    case 'SENT':
      return 'brand';
    case 'DELIVERED':
    case 'VIEWED':
      return 'success';
    case 'FAILED':
      return 'danger';
    case 'PENDING':
    default:
      return 'warning';
  }
}

function channelKey(channel: string): RfqChannel {
  return channel.toLowerCase() as RfqChannel;
}

interface DeliveryBadgeProps {
  status: string;
}

function DeliveryBadge({ status }: DeliveryBadgeProps) {
  const t = useTranslations('rfqs');

  return (
    <Badge tone={deliveryTone(status)} size="sm" dot>
      {t(`sendTracking.${status}`)}
    </Badge>
  );
}

interface RfqStatusTimelineProps {
  detail: RfqDetailResponse;
}

export function RfqStatusTimeline({ detail }: RfqStatusTimelineProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const { rfq, version } = detail;

  const currentStatus = normalizeRfqStatus(rfq.status);
  const currentStep = stepperStatus(currentStatus);
  const currentRank = TIMELINE_STATES.indexOf(currentStep);
  const events = statusEvents(detail);
  const deliveries = detail.deliveries ?? [];

  return (
    <section className="flex flex-col gap-y-4" aria-labelledby="rfq-tracking-title">
      <div className="flex flex-wrap items-start gap-x-6 gap-y-3">
        <div className="min-w-0">
          <h3 id="rfq-tracking-title" className="text-heading-5 text-foreground">
            {t('detail.timeline.title')}
          </h3>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-2 text-paragraph-sm text-foreground-muted">
            <span className="inline-flex items-center gap-x-1.5">
              <Clock3Icon className="size-4" aria-hidden="true" />
              {t('detail.timeline.current')}
            </span>
            <RfqStatusBadge status={currentStatus} size="sm" />
            {version && (
              <>
                <span aria-hidden="true" className="text-foreground-subtle">
                  |
                </span>
                <span>v{version.version_number}</span>
                <span className="font-medium text-foreground">{fmt.currency(version.total)}</span>
              </>
            )}
          </div>
        </div>
      </div>

      <div className="flex items-center overflow-x-auto pb-1">
        {TIMELINE_STATES.map((state, index) => {
          const isCurrent = state === currentStep;
          const isPast = currentRank >= 0 && index < currentRank;
          const date = statusDate(state, detail);

          return (
            <div key={state} className="flex min-w-[96px] flex-1 items-center last:flex-none">
              <div className="flex flex-col items-center gap-y-1">
                <div
                  className={cn(
                    'flex size-5 items-center justify-center rounded-full border-2',
                    isCurrent
                      ? 'border-primary bg-primary'
                      : isPast
                        ? 'border-primary bg-primary/20'
                        : 'border-border bg-background',
                  )}
                >
                  {isCurrent && <div className="size-1.5 rounded-full bg-primary-foreground" />}
                  {isPast && <CheckIcon className="size-2.5 text-primary" aria-hidden="true" />}
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

              {index < TIMELINE_STATES.length - 1 && (
                <div className={cn('mx-1 h-px flex-1', isPast ? 'bg-primary' : 'bg-border')} />
              )}
            </div>
          );
        })}
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.9fr)]">
        <div className="min-w-0">
          <div className="mb-3 flex items-center gap-x-2 text-paragraph-sm-medium text-foreground">
            <HistoryIcon className="size-4 text-foreground-muted" aria-hidden="true" />
            {t('detail.timeline.statusHistory')}
          </div>
          {events.length > 0 ? (
            <ol className="space-y-3">
              {events.map((event) => (
                <li key={`${event.source}-${event.id}`} className="flex gap-x-3">
                  <span
                    aria-hidden="true"
                    className="mt-1 size-2 shrink-0 rounded-full bg-primary"
                  />
                  <div className="min-w-0">
                    <p className="text-paragraph-sm text-foreground">
                      {t(`status.${statusLabelKey(event.newStatus)}`)}
                    </p>
                    <p className="text-paragraph-xs text-foreground-muted">
                      {t(`detail.timeline.sources.${event.source}`)} |{' '}
                      {fmt.timestamp(event.changedAt)}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          ) : (
            <p className="text-paragraph-sm text-foreground-muted">
              {t('detail.timeline.noStatusHistory')}
            </p>
          )}
        </div>

        <div className="min-w-0">
          <div className="mb-3 flex items-center gap-x-2 text-paragraph-sm-medium text-foreground">
            <SendIcon className="size-4 text-foreground-muted" aria-hidden="true" />
            {t('detail.timeline.deliveries')}
          </div>
          {deliveries.length > 0 ? (
            <div className="space-y-3">
              {deliveries.map((delivery) => {
                const channel = channelKey(delivery.channel);
                return (
                  <div
                    key={delivery.id}
                    className="flex flex-col gap-y-2 rounded-lg border border-border bg-muted p-3"
                  >
                    <div className="flex items-center justify-between gap-x-3">
                      <span className="truncate text-paragraph-sm-medium text-foreground">
                        {t(`channels.${channel}`)}
                      </span>
                      <DeliveryBadge status={delivery.tracking_status} />
                    </div>
                    <dl className="grid gap-y-1 text-paragraph-xs text-foreground-muted">
                      <div className="flex justify-between gap-x-3">
                        <dt>{t('detail.timeline.sentAt')}</dt>
                        <dd>{fmt.timestamp(deliveryTime(delivery))}</dd>
                      </div>
                      {delivery.expires_at && (
                        <div className="flex justify-between gap-x-3">
                          <dt>{t('detail.timeline.expiresAt')}</dt>
                          <dd>{fmt.date(delivery.expires_at)}</dd>
                        </div>
                      )}
                    </dl>
                  </div>
                );
              })}
            </div>
          ) : (
            <p className="text-paragraph-sm text-foreground-muted">
              {t('detail.timeline.noDeliveries')}
            </p>
          )}
        </div>
      </div>
    </section>
  );
}
