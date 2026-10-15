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
  MetaList,
  PendingButton,
  Separator,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import type {
  ConversationBatch,
  ConversationMessage,
  ConversationResponse,
  HandlerSellerDecision,
} from '@/lib/api/rfqs';
import { retryConversationBatch } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

const SELLER_DECISION_TONES = {
  APPROVED_AS_IS: 'success',
  EDITED: 'brand',
  REJECTED: 'neutral',
  MANUAL_OVERRIDE: 'neutral',
} as const satisfies Record<HandlerSellerDecision, string>;

interface MessageListProps {
  messages: ConversationMessage[];
}

function MessageList({ messages }: MessageListProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.conversation');

  return (
    <ul className="flex flex-col gap-y-2">
      {messages.map((message, index) => (
        <li
          key={`${message.received_at}-${index}`}
          className="flex flex-col px-3 py-2 gap-y-0.5 bg-muted rounded-lg"
        >
          <MetaList
            className="text-paragraph-xs"
            items={[t('customer'), fmt.timestamp(message.received_at)]}
          />
          <p className="text-paragraph-sm text-foreground whitespace-pre-line wrap-anywhere">
            {message.body}
          </p>
        </li>
      ))}
    </ul>
  );
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
      <p className="text-paragraph-sm text-foreground-muted">
        {decision.outcome === 'APPLIED'
          ? t('decision.applied', { interpretation: decision.interpretation })
          : t(`decision.noMatch.${decision.no_match_reason ?? 'REQUIRES_SELLER'}`)}
      </p>
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
}

/*
 * Everything the customer wrote after a send, one block per batch the AI read, with what the AI
 * made of it and what the seller then did. A failed batch says why, and offers a retry only when
 * one can help.
 */
export function RfqConversation({
  conversation,
  quoteId,
  branchId,
  onChanged,
}: RfqConversationProps) {
  const t = useTranslations('rfqs.detail.conversation');
  const errorMessage = useApiErrorMessage('rfqs.detail.conversation.errors');
  const [retryingId, setRetryingId] = useState<string | null>(null);

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

  return (
    <Card>
      <CardHeader className="flex-row items-center gap-x-2">
        <MessagesSquareIcon aria-hidden="true" className="size-4 text-foreground-muted" />
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <ol className="flex flex-col gap-y-4">
          {conversation.batches.map((batch, index) => (
            <li key={batch.id} className="flex flex-col gap-y-3">
              {index > 0 ? <Separator /> : null}
              <MessageList messages={batch.messages} />
              <BatchOutcome
                batch={batch}
                retrying={retryingId === batch.id}
                onRetry={() => void handleRetry(batch.id)}
              />
            </li>
          ))}
          {conversation.pending.length > 0 ? (
            <li className="flex flex-col gap-y-3">
              {conversation.batches.length > 0 ? <Separator /> : null}
              <MessageList messages={conversation.pending} />
              <p className="text-paragraph-sm text-foreground-muted">{t('pending')}</p>
            </li>
          ) : null}
        </ol>
      </CardContent>
    </Card>
  );
}
