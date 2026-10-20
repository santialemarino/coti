'use client';

import { useState } from 'react';
import { MessagesSquareIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  DropdownChevron,
  MetaList,
  PendingButton,
  Separator,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import {
  QUOTE_LINK_PLACEHOLDER,
  type ConversationBatch,
  type ConversationMessage,
  type ConversationResponse,
  type HandlerSellerDecision,
} from '@/lib/api/rfqs';
import { retryConversationBatch } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { HandlerRecipeNote } from './handler-recipe-note';

const SELLER_DECISION_TONES = {
  APPROVED_AS_IS: 'success',
  EDITED: 'brand',
  REJECTED: 'neutral',
  MANUAL_OVERRIDE: 'neutral',
} as const satisfies Record<HandlerSellerDecision, string>;

type TimelineEntry =
  | { kind: 'batch'; at: number; batch: ConversationBatch }
  | { kind: 'send'; at: number; message: ConversationMessage };

interface MessageListProps {
  messages: ConversationMessage[];
}

function MessageList({ messages }: MessageListProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.conversation');

  return (
    <ul className="flex flex-col gap-y-2">
      {messages.map((message, index) => {
        const fromSeller = message.author === 'SELLER';
        return (
          <li
            key={`${message.received_at}-${index}`}
            className={
              fromSeller
                ? 'flex flex-col px-3 py-2 gap-y-0.5 bg-accent rounded-lg'
                : 'flex flex-col px-3 py-2 gap-y-0.5 bg-muted rounded-lg'
            }
          >
            <MetaList
              className="text-paragraph-xs"
              items={[fromSeller ? t('seller') : t('customer'), fmt.timestamp(message.received_at)]}
            />
            <p className="text-paragraph-sm text-foreground whitespace-pre-line wrap-anywhere">
              {message.body.replaceAll(QUOTE_LINK_PLACEHOLDER, t('quoteLink'))}
            </p>
          </li>
        );
      })}
    </ul>
  );
}

// A batch sits where its first message arrived, so each send lands between the rounds it split.
function timelineOf(conversation: ConversationResponse): TimelineEntry[] {
  const entries: TimelineEntry[] = [
    ...conversation.batches.map((batch) => ({
      kind: 'batch' as const,
      at: Date.parse(batch.messages[0]?.received_at ?? batch.closed_at ?? ''),
      batch,
    })),
    ...conversation.seller_messages.map((message) => ({
      kind: 'send' as const,
      at: Date.parse(message.received_at),
      message,
    })),
  ];
  return entries.sort((a, b) => (a.at || 0) - (b.at || 0));
}

interface BatchOutcomeProps {
  batch: ConversationBatch;
  retrying: boolean;
  onRetry: () => void;
}

function BatchOutcome({ batch, retrying, onRetry }: BatchOutcomeProps) {
  const t = useTranslations('rfqs.detail.conversation');
  const { decision } = batch;

  if (batch.status === 'FAILED') {
    return (
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <p className="text-paragraph-sm text-danger-foreground">
          {t(`failure.${batch.failure_reason ?? 'ATTEMPTS_EXHAUSTED'}`)}
        </p>
        {batch.retryable ? (
          <PendingButton
            type="button"
            variant="outline"
            size="sm"
            onClick={onRetry}
            pending={retrying}
            pendingLabel={t('retrying')}
          >
            {t('retry')}
          </PendingButton>
        ) : null}
      </div>
    );
  }
  if (!decision) {
    return (
      <Badge tone="neutral" size="sm" className="self-start">
        {t(`status.${batch.status}`)}
      </Badge>
    );
  }
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
      <div className="flex flex-col min-w-0 gap-y-1">
        <p className="text-paragraph-sm text-foreground-muted">
          {decision.outcome === 'APPLIED'
            ? t('decision.applied', { interpretation: decision.interpretation })
            : t(`decision.noMatch.${decision.no_match_reason ?? 'REQUIRES_SELLER'}`)}
        </p>
        <HandlerRecipeNote
          strategies={decision.strategies_tried}
          adaptation={decision.adaptation}
        />
      </div>
      {decision.seller_decision ? (
        <Badge tone={SELLER_DECISION_TONES[decision.seller_decision]} size="sm">
          {t(`sellerDecision.${decision.seller_decision}`)}
        </Badge>
      ) : (
        <Badge tone="outline" size="sm">
          {t('sellerDecision.pending')}
        </Badge>
      )}
    </div>
  );
}

interface RfqConversationProps {
  conversation: ConversationResponse;
  quoteId: string;
  branchId: string;
  onChanged: () => Promise<void>;
  // Inside another card: the thread alone, with no card or header of its own.
  embedded?: boolean;
}

/*
 * Everything the customer wrote after a send, one block per batch the AI read, with what the AI
 * made of it and what the seller then did, between the sends that framed it. A failed batch says
 * why, and offers a retry only when one can help.
 */
export function RfqConversation({
  conversation,
  quoteId,
  branchId,
  onChanged,
  embedded = false,
}: RfqConversationProps) {
  const t = useTranslations('rfqs.detail.conversation');
  const errorMessage = useApiErrorMessage('rfqs.detail.conversation.errors');
  const [retryingId, setRetryingId] = useState<string | null>(null);

  const timeline = timelineOf(conversation);

  async function handleRetry(batchId: string) {
    setRetryingId(batchId);
    try {
      await retryConversationBatch(quoteId, batchId, branchId);
      toast.success(t('retried'));
      await onChanged();
    } catch (error) {
      toast.error(errorMessage(errorCodeOf(error)));
    } finally {
      setRetryingId(null);
    }
  }

  const thread = (
    <ol className="flex flex-col gap-y-4">
      {timeline.map((entry, index) =>
        entry.kind === 'send' ? (
          <li key={`send-${entry.message.received_at}-${index}`} className="flex flex-col gap-y-3">
            {index > 0 ? <Separator /> : null}
            <MessageList messages={[entry.message]} />
          </li>
        ) : (
          <li key={entry.batch.id} className="flex flex-col gap-y-3">
            {index > 0 ? <Separator /> : null}
            <MessageList messages={entry.batch.messages} />
            <BatchOutcome
              batch={entry.batch}
              retrying={retryingId === entry.batch.id}
              onRetry={() => void handleRetry(entry.batch.id)}
            />
          </li>
        ),
      )}
      {conversation.pending.length > 0 ? (
        <li className="flex flex-col gap-y-3">
          {timeline.length > 0 ? <Separator /> : null}
          <MessageList messages={conversation.pending} />
          <p className="text-paragraph-sm text-foreground-muted">{t('pending')}</p>
        </li>
      ) : null}
    </ol>
  );

  if (embedded) return thread;

  /* Collapsed by default: the history is reference, and the page's job is the quote below it. */
  return (
    <Card>
      <Collapsible>
        <CardHeader>
          <CollapsibleTrigger className="group/panel flex w-full items-center gap-x-2 rounded-md outline-none transition-colors duration-200 ease-out-soft hover:text-primary focus-visible:text-primary">
            <MessagesSquareIcon aria-hidden="true" className="size-4 text-foreground-muted" />
            <CardTitle>{t('title')}</CardTitle>
            <DropdownChevron className="ml-auto group-data-[state=open]/panel:rotate-180" />
          </CollapsibleTrigger>
        </CardHeader>
        <CollapsibleContent>
          <CardContent className="pt-4">{thread}</CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  );
}
