'use client';

import { useState } from 'react';
import { CheckCircle2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Button, ConfirmDialog } from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { acceptSentVersion } from '@/lib/api/rfqs-client';

interface AcceptSentVersionButtonProps {
  quoteId: string;
  branchId: string;
  reference: string;
  // The number of the version the customer received.
  sentVersion: number;
  onChanged: () => Promise<void>;
}

/*
 * A change-requested quote whose customer took what they were sent after all. Accepting it throws
 * the open draft away, so it asks first.
 */
export function AcceptSentVersionButton({
  quoteId,
  branchId,
  reference,
  sentVersion,
  onChanged,
}: AcceptSentVersionButtonProps) {
  const t = useTranslations('rfqs.detail.callouts.acceptSentVersion');
  const tCommon = useTranslations('common');
  const errorMessage = useApiErrorMessage('rfqs.detail.lifecycle');
  const [open, setOpen] = useState(false);
  const [accepting, setAccepting] = useState(false);

  async function handleAccept() {
    setAccepting(true);
    try {
      await acceptSentVersion(quoteId, branchId);
      toast.success(t('success', { reference }));
      setOpen(false);
      await onChanged();
    } catch (error) {
      toast.error(errorMessage(errorCodeOf(error)));
      setOpen(false);
    } finally {
      setAccepting(false);
    }
  }

  return (
    <>
      <Button type="button" variant="outline" onClick={() => setOpen(true)}>
        <CheckCircle2Icon />
        {t('button')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={(next) => !accepting && setOpen(next)}
        entity={sentVersion}
        title={t('title')}
        description={(version) => t('description', { version })}
        onConfirm={handleAccept}
        pending={accepting}
        tone="danger"
        labels={{ confirm: t('confirm'), pending: t('pending'), cancel: tCommon('actions.cancel') }}
      />
    </>
  );
}
