'use client';

import { useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { AnimatedHeight, Callout, CopyButton, PendingButton, Textarea } from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { draftQuoteFollowup } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

interface FollowupCalloutProps {
  quoteId: string | null;
  branchId: string;
  // Who the quote is waiting on: the client once it is sent, the seller before.
  origin: 'customer' | 'seller';
  // When the quote went quiet; null when there is no quote row to read it from.
  quietSince: string | null;
}

// «Requiere seguimiento», and for a client who went quiet a message the seller can edit and send.
export function FollowupCallout({ quoteId, branchId, origin, quietSince }: FollowupCalloutProps) {
  const t = useTranslations('rfqs.detail.callouts.followup');
  const fmt = useFormatters();
  const message = useApiErrorMessage('rfqs.detail.callouts.followup.draft');
  const [draft, setDraft] = useState<string | null>(null);
  const [pending, startDraft] = useTransition();

  const canDraft = origin === 'customer' && quoteId !== null;

  function handleDraft() {
    if (!quoteId) return;
    startDraft(async () => {
      try {
        const result = await draftQuoteFollowup(quoteId, branchId);
        setDraft(result.message);
      } catch (error) {
        toast.error(message(errorCodeOf(error)));
      }
    });
  }

  return (
    <Callout
      tone="followup"
      title={t('title')}
      action={
        canDraft ? (
          <PendingButton
            variant="outline"
            size="sm"
            pending={pending}
            pendingLabel={t('draft.pending')}
            onClick={handleDraft}
          >
            {draft === null ? t('draft.action') : t('draft.again')}
          </PendingButton>
        ) : null
      }
    >
      {quietSince ? t(origin, { date: fmt.date(quietSince) }) : t(`${origin}Undated`)}
      <AnimatedHeight trigger={draft === null ? 'closed' : 'open'}>
        {draft !== null ? (
          <div className="flex flex-col pt-2 gap-y-2">
            <Textarea
              aria-label={t('draft.label')}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
            />
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <CopyButton
                value={draft}
                labels={{ copy: t('draft.copy'), copied: t('draft.copied') }}
                onCopyError={() => toast.error(t('draft.copyFailed'))}
              />
              <span className="text-paragraph-xs text-foreground-muted">{t('draft.hint')}</span>
            </div>
          </div>
        ) : null}
      </AnimatedHeight>
    </Callout>
  );
}
