'use client';

import { useTranslations } from 'next-intl';

import { useHeldWhileClosed } from '@repo/ui/hooks';
import { IssueInvoiceDialog } from '@/app/(protected)/rfqs/_components/issue-invoice-dialog';
import { MarkPaidDialog } from '@/app/(protected)/rfqs/_components/mark-paid-dialog';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import type { SaleNextStep } from '@/app/(protected)/rfqs/_components/sale-closure';
import { formatRfqReference, type RfqRecord } from '@/lib/api/rfqs';

export interface ClosingTarget {
  record: RfqRecord;
  action: SaleNextStep;
}

interface SaleClosureDialogsProps {
  target: ClosingTarget | null;
  onClose: () => void;
  // Runs once the closing write is over, whatever came of it, so the list reads the sale again.
  onDone: () => Promise<void>;
}

/*
 * The confirmations a list opens to close a sale. They live beside the list rather than inside a
 * card or a row: the write moves the order to another stack or out of the list, and the dialog has
 * to outlive the element that opened it to play its exit.
 */
export function SaleClosureDialogs({ target, onClose, onDone }: SaleClosureDialogsProps) {
  const t = useTranslations('rfqs');
  const { invoicingEnabled, invoiceOnPayment } = useRfqList();
  const shown = useHeldWhileClosed(target, target !== null);

  if (!shown) return null;
  const { record } = shown;
  const { quoteId } = record;
  if (quoteId === null) return null;
  const reference = formatRfqReference(record.quoteNumber) ?? t('list.numberPending');
  const close = (open: boolean) => {
    if (!open) onClose();
  };

  return (
    <>
      <MarkPaidDialog
        open={target?.action === 'MARK_PAID'}
        onOpenChange={close}
        quoteId={quoteId}
        branchId={record.branchId}
        reference={reference}
        invoicingEnabled={invoicingEnabled}
        invoiceOnPayment={invoiceOnPayment}
        invoiceWaived={record.invoiceWaivedAt !== null}
        onDone={onDone}
      />
      <IssueInvoiceDialog
        open={target?.action === 'INVOICE'}
        onOpenChange={close}
        quoteId={quoteId}
        branchId={record.branchId}
        onIssued={onDone}
        onFailed={onDone}
      />
    </>
  );
}
