'use client';

import { useEffect, useRef, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  PendingButton,
  Spinner,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { formatInvoiceNumber, type Invoice, type InvoicePreview } from '@/lib/api/invoicing';
import { getInvoicePreview, issueInvoice } from '@/lib/api/invoicing-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { isSetupIssue } from '@/lib/utils/setup-issues';

type PreviewState =
  | { kind: 'loading' }
  | { kind: 'failed' }
  | { kind: 'loaded'; preview: InvoicePreview };

interface IssueInvoiceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  quoteId: string;
  branchId: string;
  // The draft already on screen; without one the dialog reads it each time it opens.
  preview?: InvoicePreview;
  onIssued: (invoice: Invoice) => void | Promise<void>;
  // Runs after a refused issue has been reported, so the caller can read the sale again.
  onFailed?: (cause: unknown) => void | Promise<void>;
  onCloseAutoFocus?: (event: Event) => void;
}

/*
 * The confirmation every «Facturar» goes through: the invoice card, a queue card and a table row.
 * Issuing cannot be undone, only cancelled by a credit note, so it always names the type, the
 * receiver and the total, and it refuses to offer an invoice the draft says cannot be issued yet.
 */
export function IssueInvoiceDialog({
  open,
  onOpenChange,
  quoteId,
  branchId,
  preview: givenPreview,
  onIssued,
  onFailed,
  onCloseAutoFocus,
}: IssueInvoiceDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card');
  const tSetup = useTranslations('common.setup');
  const message = useApiErrorMessage('invoicing.card');
  const [read, setRead] = useState<PreviewState>({ kind: 'loading' });
  const [issuing, startIssue] = useTransition();
  const latestRead = useRef(0);

  const state: PreviewState = givenPreview ? { kind: 'loaded', preview: givenPreview } : read;
  const preview = state.kind === 'loaded' ? state.preview : null;
  const live = preview?.invoice?.status === 'ISSUED' || preview?.invoice?.status === 'PENDING';
  const ready = preview !== null && !live && preview.issues.length === 0;

  useEffect(() => {
    if (!open || givenPreview) return;
    const ticket = ++latestRead.current;
    setRead({ kind: 'loading' });
    getInvoicePreview(quoteId, branchId)
      .then((result) => {
        if (ticket === latestRead.current) setRead({ kind: 'loaded', preview: result });
      })
      .catch(() => {
        if (ticket === latestRead.current) setRead({ kind: 'failed' });
      });
  }, [branchId, givenPreview, open, quoteId]);

  function confirm(confirmed: InvoicePreview) {
    startIssue(async () => {
      try {
        const invoice = await issueInvoice(quoteId, branchId, confirmed);
        onOpenChange(false);
        toast.success(
          t('issued', {
            type: invoice.type,
            number: formatInvoiceNumber(invoice.pointOfSale, invoice.number ?? 0),
          }),
        );
        await onIssued(invoice);
      } catch (cause) {
        onOpenChange(false);
        toast.error(message(errorCodeOf(cause)));
        await onFailed?.(cause);
      }
    });
  }

  function blockedReasons(issues: string[]): string {
    return fmt.list(
      issues.map((issue) =>
        isSetupIssue(issue) ? tSetup(`${issue}.title`) : t(`issues.${issue}.title`),
      ),
    );
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !issuing && onOpenChange(next)}>
      <DialogContent
        className="sm:max-w-md"
        closeOnClickOutside={!issuing}
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogHeader>
          <DialogTitle>
            {preview ? t('confirm.title', { type: preview.type }) : t('confirm.titlePending')}
          </DialogTitle>
          <DialogDescription>
            {state.kind === 'loading' ? (
              <span className="flex items-center gap-x-2">
                <Spinner size="sm" />
                {t('loading')}
              </span>
            ) : state.kind === 'failed' || !preview ? (
              t('confirm.unavailable')
            ) : live ? (
              t('confirm.alreadyIssued')
            ) : preview.issues.length > 0 ? (
              t('confirm.blocked', { reasons: blockedReasons(preview.issues) })
            ) : (
              t('confirm.description', {
                type: preview.type,
                receiver: preview.receiver.name || t('finalConsumer'),
                total: fmt.currency(preview.amounts.total, preview.currency),
              })
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" disabled={issuing} onClick={() => onOpenChange(false)}>
            {t('confirm.cancel')}
          </Button>
          {ready || state.kind === 'loading' ? (
            <PendingButton
              pending={issuing}
              pendingLabel={t('confirm.confirming')}
              disabled={!ready}
              onClick={() => preview && confirm(preview)}
            >
              {t('confirm.confirm')}
            </PendingButton>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
