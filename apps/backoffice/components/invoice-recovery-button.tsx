'use client';

import { useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { PendingButton } from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { ApiError, codeForStatus, errorCodeOf, knownErrorCode } from '@/lib/api/errors';

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
  const t = useTranslations('invoicing.history');
  const message = useApiErrorMessage('invoicing.card');
  const router = useRouter();
  const [pending, startTransition] = useTransition();

  function recover() {
    startTransition(async () => {
      try {
        const response = await fetch(`/api/invoices/${encodeURIComponent(invoiceId)}/recover`, {
          method: 'POST',
          headers: { 'X-Branch-Id': branchId },
        });
        if (!response.ok) {
          const result = (await response.json()) as { code?: string };
          throw new ApiError(
            knownErrorCode(result.code) ?? codeForStatus(response.status),
            response.status,
          );
        }
        toast.success(t('recovered'));
        onRecovered?.();
        router.refresh();
      } catch (error) {
        toast.error(message(errorCodeOf(error)));
      }
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
