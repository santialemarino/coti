'use client';

import { useRef, useState, useTransition } from 'react';
import { WalletIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  PendingButton,
} from '@repo/ui/components';
import { MarkPaidDialog } from '@/app/(protected)/rfqs/_components/mark-paid-dialog';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { setQuotePaid } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

interface SaleClosureCardProps {
  quoteId: string;
  branchId: string;
  reference: string;
  paidAt: string | null;
  // The name of whoever marked it paid.
  paidBy: string | null;
  saleComplete: boolean;
  invoicingEnabled: boolean;
  invoiceOnPayment: boolean;
  // «No requiere factura» is set, so marking it paid offers no invoice.
  invoiceWaived: boolean;
  // An archived sale shows its closure but offers no change until unarchived.
  readOnly?: boolean;
  onChanged: () => Promise<void>;
}

/*
 * How far an accepted sale is closed. «Cobrado» is a fact the seller marks and can take back; it
 * never moves the order's status.
 */
export function SaleClosureCard({
  quoteId,
  branchId,
  reference,
  paidAt,
  paidBy,
  saleComplete,
  invoicingEnabled,
  invoiceOnPayment,
  invoiceWaived,
  readOnly = false,
  onChanged,
}: SaleClosureCardProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.closure');
  const message = useApiErrorMessage('rfqs.detail.closure');
  const [marking, setMarking] = useState(false);
  const [unmarking, startUnmark] = useTransition();
  const markButtonRef = useRef<HTMLButtonElement>(null);

  function unmark() {
    startUnmark(async () => {
      try {
        await setQuotePaid(quoteId, false, branchId);
        toast.success(t('unmarkedPaid', { reference }));
        await onChanged();
        requestAnimationFrame(() => markButtonRef.current?.focus());
      } catch (cause) {
        toast.error(message(errorCodeOf(cause)));
      }
    });
  }

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <div className="flex items-center gap-x-2">
          <WalletIcon aria-hidden="true" className="size-4 text-foreground-muted" />
          <CardTitle>{t('title')}</CardTitle>
        </div>
        {saleComplete ? (
          <Badge tone="success" dot>
            {t('complete')}
          </Badge>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
        <p className="text-paragraph-sm text-foreground">
          {paidAt ? t('paid', { name: paidBy ?? '', date: fmt.date(paidAt) }) : t('unpaid')}
        </p>
        {readOnly ? null : paidAt ? (
          <PendingButton
            variant="outline"
            size="sm"
            pending={unmarking}
            pendingLabel={t('unmarking')}
            onClick={unmark}
          >
            {t('unmarkPaid')}
          </PendingButton>
        ) : (
          <Button ref={markButtonRef} size="sm" onClick={() => setMarking(true)}>
            {t('markPaid')}
          </Button>
        )}
      </CardContent>
      <MarkPaidDialog
        open={marking}
        onOpenChange={setMarking}
        quoteId={quoteId}
        branchId={branchId}
        reference={reference}
        invoicingEnabled={invoicingEnabled}
        invoiceOnPayment={invoiceOnPayment}
        invoiceWaived={invoiceWaived}
        onDone={onChanged}
      />
    </Card>
  );
}
