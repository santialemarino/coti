'use client';

import { MessagesSquareIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DropdownChevron,
  Separator,
} from '@repo/ui/components';
import type {
  ChangeRequestDiff,
  ClientMessage,
  ConversationResponse,
  DiffChange,
  DiffDiscount,
  DiffLine,
  DiffLineItem,
  HandlerNoMatchReason,
} from '@/lib/api/rfqs';
import { useFormatters } from '@/lib/i18n/formatters';
import { shortUnit } from '@/lib/utils/units';
import { RfqConversation } from './rfq-conversation';

const CHANGE_TONES = {
  MODIFIED: 'warning',
  ADDED: 'success',
  REMOVED: 'danger',
} as const satisfies Record<Exclude<DiffChange, 'UNCHANGED'>, string>;

const NO_MATCH_OUTCOMES = {
  REQUIRES_SELLER: 'outcome.noMatch.REQUIRES_SELLER',
  AI_DISABLED: 'outcome.noMatch.AI_DISABLED',
  PROPOSAL_REFUSED: 'outcome.noMatch.PROPOSAL_REFUSED',
  PIPELINE_EXHAUSTED: 'outcome.noMatch.PIPELINE_EXHAUSTED',
  PROMOTION_NOT_EARNED: 'outcome.noMatch.PROMOTION_NOT_EARNED',
} as const satisfies Record<HandlerNoMatchReason, string>;

type Formatters = ReturnType<typeof useFormatters>;

function lineName(item: DiffLineItem): string {
  return item.product_name ?? item.description;
}

function lineQuantity(item: DiffLineItem, fmt: Formatters): string {
  const unit = item.unit ? ` ${shortUnit(item.unit)}` : '';
  return `${fmt.value(Number(item.quantity))}${unit}`;
}

// "old → new" when a value moved, the value alone when it did not.
function Moved({ from, to }: { from: string | null; to: string | null }) {
  if (from === to || from === null) return <span>{to}</span>;
  if (to === null) return <span className="text-foreground-muted line-through">{from}</span>;
  return (
    <span>
      <span className="text-foreground-muted line-through">{from}</span>
      <span aria-hidden="true" className="px-1 text-foreground-subtle">
        →
      </span>
      <span>{to}</span>
    </span>
  );
}

interface ChangedLineProps {
  line: DiffLine;
}

function ChangedLine({ line }: ChangedLineProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.changeRequest');
  const change = line.change as Exclude<DiffChange, 'UNCHANGED'>;
  const { original, requested } = line;
  const price = (item: DiffLineItem | null) =>
    item?.unit_price != null ? fmt.currency(item.unit_price) : null;

  return (
    <li className="flex items-start py-2 gap-x-3 border-b border-border last:border-b-0">
      <Badge tone={CHANGE_TONES[change]} size="sm" className="mt-0.5 shrink-0">
        {t(`change.${change}`)}
      </Badge>
      <div className="flex flex-col min-w-0 gap-y-0.5 text-paragraph-sm text-foreground">
        <Moved
          from={original ? lineName(original) : null}
          to={requested ? lineName(requested) : null}
        />
        {original && requested ? (
          <span className="text-paragraph-xs text-foreground-muted tabular-nums">
            <Moved from={lineQuantity(original, fmt)} to={lineQuantity(requested, fmt)} />
            {price(requested) ? (
              <>
                <span aria-hidden="true"> · </span>
                <Moved from={price(original)} to={price(requested)} />
              </>
            ) : null}
          </span>
        ) : (
          <span className="text-paragraph-xs text-foreground-muted tabular-nums">
            {lineQuantity((requested ?? original) as DiffLineItem, fmt)}
          </span>
        )}
      </div>
    </li>
  );
}

function ChangedDiscount({ discount }: { discount: DiffDiscount }) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.changeRequest');
  const change = discount.change as Exclude<DiffChange, 'UNCHANGED'>;
  const amount = (side: DiffDiscount['original']) =>
    side ? (side.suppressed ? t('suppressed') : `−${fmt.currency(side.amount)}`) : null;

  return (
    <li className="flex items-start py-2 gap-x-3 border-b border-border last:border-b-0">
      <Badge tone={CHANGE_TONES[change]} size="sm" className="mt-0.5 shrink-0">
        {t(`change.${change}`)}
      </Badge>
      <div className="flex flex-col min-w-0 gap-y-0.5 text-paragraph-sm text-foreground">
        <span>{discount.name}</span>
        <span className="text-paragraph-xs text-foreground-muted tabular-nums">
          <Moved from={amount(discount.original)} to={amount(discount.requested)} />
        </span>
      </div>
    </li>
  );
}

interface RfqChangeRequestCardProps {
  diff: ChangeRequestDiff;
  // A customer's request quotes their words; a seller reopening a closed quote has none.
  variant: 'customer-request' | 'seller-revision';
  conversation?: ConversationResponse;
  quoteId: string;
  branchId: string;
  canDiscard: boolean;
  onDiscard: () => void;
  onChanged: () => Promise<void>;
}

/*
 * Everything a seller needs to act on a change request, in reading order: what the customer wrote,
 * how it was read and what that did, then only what changed against the version they received.
 * The whole conversation is one click away rather than on the page.
 */
export function RfqChangeRequestCard({
  diff,
  variant,
  conversation,
  quoteId,
  branchId,
  canDiscard,
  onDiscard,
  onChanged,
}: RfqChangeRequestCardProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.changeRequest');

  const messages: ClientMessage[] = diff.client_messages ?? [];
  const handler = diff.handler;
  const changedLines = diff.lines.filter((line) => line.change !== 'UNCHANGED');
  const changedDiscounts = diff.discounts.filter((discount) => discount.change !== 'UNCHANGED');
  const unchanged = diff.lines.length - changedLines.length;
  const original = Number(diff.original_total);
  const requested = Number(diff.requested_total);
  const delta = original > 0 ? (requested - original) / original : 0;
  const rounds = conversation
    ? conversation.batches.length + (conversation.pending.length > 0 ? 1 : 0)
    : 0;
  const outcome = !handler
    ? null
    : handler.seller_decision === 'REJECTED'
      ? t('outcome.discarded')
      : handler.outcome === 'APPLIED'
        ? t('outcome.applied')
        : t(NO_MATCH_OUTCOMES[handler.no_match_reason ?? 'REQUIRES_SELLER']);

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-x-4">
        <CardTitle className="text-heading-5">
          {t(variant === 'customer-request' ? 'title' : 'revisionTitle')}
        </CardTitle>
        {canDiscard ? (
          <Button type="button" variant="outline" size="sm" onClick={onDiscard}>
            {t('discard')}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-y-4">
        {variant === 'customer-request' && messages.length > 0 ? (
          <blockquote className="flex flex-col py-2 pl-3 pr-3 gap-y-2 bg-muted border-l-2 border-border-strong rounded-r-lg">
            {messages.map((message, index) => (
              <div key={`${message.received_at}-${index}`} className="flex flex-col gap-y-0.5">
                <p className="text-paragraph-sm text-foreground whitespace-pre-line wrap-anywhere">
                  {message.body}
                </p>
                <span className="text-paragraph-xs text-foreground-muted">
                  {fmt.timestamp(message.received_at)}
                </span>
              </div>
            ))}
          </blockquote>
        ) : variant === 'customer-request' && diff.reason ? (
          <blockquote className="py-2 pl-3 pr-3 bg-muted border-l-2 border-border-strong rounded-r-lg text-paragraph-sm text-foreground whitespace-pre-line wrap-anywhere">
            {diff.reason}
          </blockquote>
        ) : null}

        {handler ? (
          <div className="flex flex-col gap-y-1">
            {handler.interpretation ? (
              <p className="text-paragraph-sm text-foreground">
                <span className="text-paragraph-sm-semibold">{t('interpretation')} </span>
                {handler.interpretation}
              </p>
            ) : null}
            {outcome ? <p className="text-paragraph-sm text-foreground-muted">{outcome}</p> : null}
          </div>
        ) : null}

        <Separator />

        <section className="flex flex-col gap-y-1">
          <h3 className="text-paragraph-xs-semibold text-foreground-muted">{t('changesTitle')}</h3>
          {changedLines.length + changedDiscounts.length > 0 ? (
            <ul className="flex flex-col">
              {changedLines.map((line, index) => (
                <ChangedLine key={`line-${index}`} line={line} />
              ))}
              {changedDiscounts.map((discount) => (
                <ChangedDiscount key={`discount-${discount.name}`} discount={discount} />
              ))}
            </ul>
          ) : (
            <p className="py-1 text-paragraph-sm text-foreground-muted">{t('noChanges')}</p>
          )}
          {unchanged > 0 ? (
            <p className="text-paragraph-xs text-foreground-muted">
              {t('unchanged', { count: unchanged })}
            </p>
          ) : null}
          <div className="flex flex-wrap items-baseline justify-between pt-2 gap-x-3 border-t border-border">
            <span className="text-paragraph-sm-semibold text-foreground">{t('total')}</span>
            <span className="text-paragraph-sm tabular-nums text-foreground">
              <Moved
                from={original === requested ? null : fmt.currency(diff.original_total)}
                to={fmt.currency(diff.requested_total)}
              />
              {original !== requested && original > 0 ? (
                <span className="pl-2 text-paragraph-xs text-foreground-muted">
                  ({delta > 0 ? '+' : ''}
                  {fmt.ratePct(delta)})
                </span>
              ) : null}
            </span>
          </div>
        </section>

        {conversation ? (
          <Collapsible className="min-w-0">
            <CollapsibleTrigger className="group/panel flex w-full items-center py-1 gap-x-2 rounded-md outline-none text-paragraph-sm-medium text-foreground transition-colors duration-200 ease-out-soft hover:text-primary focus-visible:text-primary">
              <MessagesSquareIcon aria-hidden="true" className="size-4 text-foreground-muted" />
              {t('conversation')}
              <Badge tone="neutral" size="sm">
                {t('rounds', { count: rounds })}
              </Badge>
              <DropdownChevron className="ml-auto group-data-[state=open]/panel:rotate-180" />
            </CollapsibleTrigger>
            <CollapsibleContent>
              <div className="pt-3">
                <RfqConversation
                  conversation={conversation}
                  quoteId={quoteId}
                  branchId={branchId}
                  onChanged={onChanged}
                  embedded
                />
              </div>
            </CollapsibleContent>
          </Collapsible>
        ) : null}
      </CardContent>
    </Card>
  );
}
