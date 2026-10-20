'use client';

import { useEffect, useId, useRef, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
  PendingButton,
  Spinner,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { formatInvoiceNumber, type InvoicePreview } from '@/lib/api/invoicing';
import { getInvoicePreview, issueInvoice } from '@/lib/api/invoicing-client';
import { setQuotePaid } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { isSetupIssue } from '@/lib/utils/setup-issues';

type PreviewState =
  | { kind: 'loading' }
  | { kind: 'failed' }
  | { kind: 'loaded'; preview: InvoicePreview };

interface MarkPaidDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  quoteId: string;
  branchId: string;
  reference: string;
  // Whether the account invoices with Coti; off, the dialog never offers the invoice.
  invoicingEnabled: boolean;
  // The account default for «Facturar también».
  invoiceOnPayment: boolean;
  // Runs once the paid mark is saved, whether or not the invoice went out.
  onDone: () => Promise<void>;
}

/*
 * Marks an accepted sale paid and, in the same confirmation, offers its invoice with the preview the
 * invoice card would issue. The paid mark is saved first: an invoice ARCA refuses leaves it in place.
 */
export function MarkPaidDialog({
  open,
  onOpenChange,
  quoteId,
  branchId,
  reference,
  invoicingEnabled,
  invoiceOnPayment,
  onDone,
}: MarkPaidDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.closure');
  const tCard = useTranslations('invoicing.card');
  const tSetup = useTranslations('common.setup');
  const message = useApiErrorMessage('rfqs.detail.closure');
  const [state, setState] = useState<PreviewState>({ kind: 'loading' });
  const [invoice, setInvoice] = useState(false);
  const [saving, startSave] = useTransition();
  const latestRead = useRef(0);
  const checkboxId = useId();

  const preview = state.kind === 'loaded' ? state.preview : null;
  // A sale with an invoice issued or in flight has nothing to offer; neither has an account off.
  const offered =
    invoicingEnabled &&
    state.kind !== 'failed' &&
    preview?.invoice?.status !== 'ISSUED' &&
    preview?.invoice?.status !== 'PENDING';
  const blocked = preview !== null && preview.issues.length > 0;
  const invoicing = offered && preview !== null && !blocked && invoice;

  // Each opening reads the invoice afresh and starts from the account default.
  useEffect(() => {
    if (!open || !invoicingEnabled) return;
    const read = ++latestRead.current;
    setState({ kind: 'loading' });
    setInvoice(invoiceOnPayment);
    getInvoicePreview(quoteId, branchId)
      .then((result) => {
        if (read === latestRead.current) setState({ kind: 'loaded', preview: result });
      })
      .catch(() => {
        if (read === latestRead.current) setState({ kind: 'failed' });
      });
  }, [branchId, invoiceOnPayment, invoicingEnabled, open, quoteId]);

  function confirm() {
    startSave(async () => {
      try {
        await setQuotePaid(quoteId, true, branchId);
      } catch (cause) {
        toast.error(message(errorCodeOf(cause)));
        return;
      }
      if (invoicing && preview) {
        try {
          const issued = await issueInvoice(quoteId, branchId, preview);
          toast.success(
            t('markedPaidAndInvoiced', {
              reference,
              type: issued.type,
              number: formatInvoiceNumber(issued.pointOfSale, issued.number ?? 0),
            }),
          );
        } catch (cause) {
          toast.error(
            t(errorCodeOf(cause) === 'INVOICE_REJECTED' ? 'invoiceRejected' : 'invoiceFailed'),
          );
        }
      } else {
        toast.success(t('markedPaid', { reference }));
      }
      onOpenChange(false);
      await onDone();
    });
  }

  function blockedReasons(issues: string[]): string {
    return fmt.list(
      issues.map((issue) =>
        isSetupIssue(issue) ? tSetup(`${issue}.title`) : tCard(`issues.${issue}.title`),
      ),
    );
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !saving && onOpenChange(next)}>
      <DialogContent className="sm:max-w-md" closeOnClickOutside={!saving}>
        <DialogHeader>
          <DialogTitle>{t('dialog.title', { reference })}</DialogTitle>
          <DialogDescription>{t('dialog.description')}</DialogDescription>
        </DialogHeader>

        {!invoicingEnabled ? null : state.kind === 'loading' ? (
          <div className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted">
            <Spinner size="sm" />
            {t('dialog.invoiceLoading')}
          </div>
        ) : state.kind === 'failed' ? (
          <p className="text-paragraph-sm text-foreground-muted">
            {t('dialog.invoiceUnavailable')}
          </p>
        ) : offered && preview ? (
          <div className="flex items-start gap-x-3">
            <Checkbox
              id={checkboxId}
              className="mt-0.5"
              checked={!blocked && invoice}
              disabled={blocked || saving}
              onCheckedChange={(checked) => setInvoice(checked === true)}
              aria-describedby={`${checkboxId}-hint`}
            />
            <div className="flex flex-col gap-y-1">
              <Label htmlFor={checkboxId}>{t('dialog.invoice')}</Label>
              <p id={`${checkboxId}-hint`} className="text-paragraph-xs text-foreground-muted">
                {blocked
                  ? t('dialog.invoiceBlocked', { reasons: blockedReasons(preview.issues) })
                  : t('dialog.invoiceHint', {
                      type: preview.type,
                      receiver: preview.receiver.name || tCard('finalConsumer'),
                      total: fmt.currency(preview.amounts.total, preview.currency),
                    })}
              </p>
            </div>
          </div>
        ) : null}

        <DialogFooter>
          <Button variant="outline" disabled={saving} onClick={() => onOpenChange(false)}>
            {t('dialog.cancel')}
          </Button>
          <PendingButton
            pending={saving}
            pendingLabel={t('dialog.pending')}
            disabled={invoicingEnabled && state.kind === 'loading'}
            onClick={confirm}
          >
            {invoicing ? t('dialog.confirmAndInvoice') : t('dialog.confirm')}
          </PendingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
