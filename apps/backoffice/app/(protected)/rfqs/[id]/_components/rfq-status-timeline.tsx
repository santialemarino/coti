'use client';

import type { ComponentProps } from 'react';
import { HistoryIcon, SendIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DropdownChevron,
  EmptyState,
  Separator,
  Stepper,
} from '@repo/ui/components';
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

interface TimelinePanelProps {
  title: string;
  icon: typeof HistoryIcon;
  count: number;
  children: React.ReactNode;
}

/*
 * A folded panel under the rail. Both of these are records, not the answer to "where is this order
 * now?" — that is the rail, and the header's badge. Open by default they doubled the height of the
 * screen with material a seller reads once a week.
 */
function TimelinePanel({ title, icon: Icon, count, children }: TimelinePanelProps) {
  return (
    <Collapsible className="min-w-0">
      <CollapsibleTrigger className="group/panel flex w-full items-center gap-x-2 py-1 rounded-md outline-none text-paragraph-sm-medium text-foreground transition-colors duration-200 ease-out-soft hover:text-primary focus-visible:text-primary">
        <Icon aria-hidden="true" className="size-4 text-foreground-muted" />
        {title}
        <Badge tone="neutral" size="sm">
          {count}
        </Badge>
        {/* The trigger owns the open state, so the rotation is driven off its data-state. */}
        <DropdownChevron className="ml-auto group-data-[state=open]/panel:rotate-180" />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="pt-3">{children}</div>
      </CollapsibleContent>
    </Collapsible>
  );
}

interface RfqStatusTimelineProps {
  detail: RfqDetailResponse;
}

export function RfqStatusTimeline({ detail }: RfqStatusTimelineProps) {
  const t = useTranslations('rfqs');
  const fmt = useFormatters();
  const { rfq } = detail;

  const currentStatus = normalizeRfqStatus(rfq.status);
  const currentStep = stepperStatus(currentStatus);
  const events = statusEvents(detail);
  const deliveries = detail.deliveries ?? [];

  return (
    <section className="flex flex-col gap-y-4" aria-labelledby="rfq-tracking-title">
      <h3 id="rfq-tracking-title" className="sr-only">
        {t('detail.timeline.title')}
      </h3>

      {/*
       * The rail gets a rule above and below it. It is the one thing on this screen that is read at
       * a glance, and with the panels pressed against it nothing said where it started or ended.
       */}
      <Separator />
      <Stepper
        className="py-1"
        currentIndex={TIMELINE_STATES.indexOf(currentStep)}
        steps={TIMELINE_STATES.map((state) => {
          const date = statusDate(state, detail);
          return {
            id: state,
            label: t(`status.${state}`),
            meta: date ? fmt.dateNumeric(date) : undefined,
          };
        })}
      />
      <Separator />

      <div className="grid gap-x-6 gap-y-2 lg:grid-cols-2">
        <TimelinePanel
          title={t('detail.timeline.statusHistory')}
          icon={HistoryIcon}
          count={events.length}
        >
          {events.length > 0 ? (
            <ol className="flex flex-col gap-y-3">
              {events.map((event) => (
                <li key={`${event.source}-${event.id}`} className="flex gap-x-3">
                  <span
                    aria-hidden="true"
                    className="mt-1 size-2 shrink-0 bg-primary rounded-full"
                  />
                  <div className="min-w-0">
                    <p className="text-paragraph-sm text-foreground">
                      {t(`status.${statusLabelKey(event.newStatus)}`)}
                    </p>
                    <p className="text-paragraph-xs text-foreground-muted">
                      {t(`detail.timeline.sources.${event.source}`)} ·{' '}
                      {fmt.timestamp(event.changedAt)}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          ) : (
            <EmptyState
              icon={HistoryIcon}
              size="inline"
              title={t('detail.timeline.noStatusHistory')}
            />
          )}
        </TimelinePanel>

        <TimelinePanel
          title={t('detail.timeline.deliveries')}
          icon={SendIcon}
          count={deliveries.length}
        >
          {deliveries.length > 0 ? (
            <div className="flex flex-col gap-y-3">
              {deliveries.map((delivery) => {
                const channel = channelKey(delivery.channel);
                return (
                  <div
                    key={delivery.id}
                    className="flex flex-col p-3 gap-y-2 bg-muted border border-border rounded-lg"
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
                        <dd className="tabular-nums">{fmt.timestamp(deliveryTime(delivery))}</dd>
                      </div>
                      {delivery.expires_at ? (
                        <div className="flex justify-between gap-x-3">
                          <dt>{t('detail.timeline.expiresAt')}</dt>
                          <dd className="tabular-nums">{fmt.dateNumeric(delivery.expires_at)}</dd>
                        </div>
                      ) : null}
                    </dl>
                  </div>
                );
              })}
            </div>
          ) : (
            <EmptyState icon={SendIcon} size="inline" title={t('detail.timeline.noDeliveries')} />
          )}
        </TimelinePanel>
      </div>
    </section>
  );
}
