'use client';

import { useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { PendingButton } from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { recoverInvoice } from '@/lib/api/invoicing-client';

interface InvoiceRecoveryButtonProps {
  invoiceId: string;
  branchId: string;
  onRecovered?: () => void;
}

export function InvoiceRecoveryButton({
  invoiceId,
  branchId,
  onRecovered,
}: InvoiceRecoveryButtonProps) {
  const router = useRouter();
  const t = useTranslations('invoicing.history');
  const message = useApiErrorMessage('invoicing.card');
  const [pending, startTransition] = useTransition();

  function recover() {
    startTransition(async () => {
      try {
        await recoverInvoice(invoiceId, branchId);
      } catch (error) {
        toast.error(message(errorCodeOf(error)));
        return;
      }
      toast.success(t('recovered'));
      onRecovered?.();
      router.refresh();
    });
  }

  return (
    <PendingButton
      variant="outline"
      size="sm"
      pending={pending}
      pendingLabel={t('recovering')}
      onClick={recover}
    >
      {t('recover')}
    </PendingButton>
  );
}
