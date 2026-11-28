'use client';

import { useTransition } from 'react';
import { MessageCircleIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Callout, PendingButton } from '@repo/ui/components';
import {
  customerReplyKind,
  type CustomerReplyKind,
} from '@/app/(protected)/rfqs/_components/customer-reply';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import type { CustomerReply } from '@/lib/api/rfqs';
import { readCustomerReply, transitionQuote } from '@/lib/api/rfqs-client';

const CALLOUT_TONES = {
  ACCEPTS: 'success',
  REJECTS: 'warning',
  QUESTION: 'info',
  WROTE: 'info',
} as const satisfies Record<CustomerReplyKind, string>;

interface CustomerReplyCalloutProps {
  quoteId: string;
  branchId: string;
  reference: string;
  reply: Pick<CustomerReply, 'reason' | 'replyAcceptance' | 'createdAt'>;
  onChanged: () => Promise<void>;
}

/*
 * A reply that left a sent quote as it was. An apparent acceptance offers «Marcar aceptada», an
 * apparent rejection a one-click «Rechazar» — the AI never rejects on its own — and every kind can be
 * marked read, which keeps it out of «El cliente escribió» until the customer writes again.
 */
export function CustomerReplyCallout({
  quoteId,
  branchId,
  reference,
  reply,
  onChanged,
}: CustomerReplyCalloutProps) {
  const t = useTranslations('rfqs.detail.callouts.customerReply');
  const errorMessage = useApiErrorMessage('rfqs.detail.lifecycle');
  const [accepting, startAccept] = useTransition();
  const [rejecting, startReject] = useTransition();
  const [reading, startRead] = useTransition();

  const kind = customerReplyKind(reply);
  const busy = accepting || rejecting || reading;

  function run(action: () => Promise<unknown>, success: string) {
    return async () => {
      try {
        await action();
        toast.success(success);
        await onChanged();
      } catch (error) {
        toast.error(errorMessage(errorCodeOf(error)));
      }
    };
  }

  return (
    <Callout
      tone={CALLOUT_TONES[kind]}
      icon={MessageCircleIcon}
      title={t(`${kind}.title`)}
      action={
        <div className="flex flex-wrap gap-2">
          <PendingButton
            type="button"
            variant="outline"
            size="sm"
            disabled={busy && !reading}
            pending={reading}
            pendingLabel={t('reading')}
            onClick={() =>
              startRead(
                run(
                  () => readCustomerReply(quoteId, branchId, reply.createdAt),
                  t('readSuccess', { reference }),
                ),
              )
            }
          >
            {t('read')}
          </PendingButton>
          {kind === 'ACCEPTS' ? (
            <PendingButton
              type="button"
              size="sm"
              disabled={busy && !accepting}
              pending={accepting}
              pendingLabel={t('accepting')}
              onClick={() =>
                startAccept(
                  run(
                    () => transitionQuote(quoteId, branchId, 'ACCEPTED'),
                    t('acceptSuccess', { reference }),
                  ),
                )
              }
            >
              {t('accept')}
            </PendingButton>
          ) : null}
          {kind === 'REJECTS' ? (
            <PendingButton
              type="button"
              variant="destructive"
              size="sm"
              disabled={busy && !rejecting}
              pending={rejecting}
              pendingLabel={t('rejecting')}
              onClick={() =>
                startReject(
                  run(
                    () => transitionQuote(quoteId, branchId, 'REJECTED'),
                    t('rejectSuccess', { reference }),
                  ),
                )
              }
            >
              {t('reject')}
            </PendingButton>
          ) : null}
        </div>
      }
    >
      {t(`${kind}.description`)}
    </Callout>
  );
}
