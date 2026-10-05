'use client';

import { useCallback, useEffect, useState, useTransition } from 'react';
import { ExternalLinkIcon, ReceiptTextIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  CopyButton,
  InlineLink,
  PendingButton,
  Spinner,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { SetupNotice } from '@/components/setup-notice';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { ApiError, errorCodeOf } from '@/lib/api/errors';
import {
  formatCuit,
  formatInvoiceNumber,
  type Invoice,
  type InvoiceAmounts,
  type InvoicePreview,
  type InvoiceReceiver,
} from '@/lib/api/invoicing';
import { getInvoicePreview, issueInvoice } from '@/lib/api/invoicing-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { isSetupIssue } from '@/lib/utils/setup-issues';
import { ClientFiscalForm } from './client-fiscal-form';

const RECEIVER_ISSUES = new Set(['RECEIVER_CUIT_REQUIRED', 'RECEIVER_ID_REQUIRED']);
const NO_AMOUNT = '0.00';

interface InvoiceCardProps {
  quoteId: string;
  branchId: string;
  /* The sale's client: null when none is associated, undefined while that is still unknown. */
  clientId: string | null | undefined;
}

/*
 * The invoice of an accepted sale: the one ARCA authorized, or what would be issued now and every
 * gap that stops it. Issuing is always a person's confirmed decision — it cannot be undone.
 */
export function InvoiceCard({ quoteId, branchId, clientId }: InvoiceCardProps) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card');
  const message = useApiErrorMessage('invoicing.card');
  const { isAdmin } = useRfqList();
  const [preview, setPreview] = useState<InvoicePreview | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  // ARCA's reasons from the attempt just made, until a reload carries them on the invoice itself.
  const [rejection, setRejection] = useState<string[]>([]);
  const [confirming, setConfirming] = useState(false);
  // A buyer with nothing missing may still be a registered one owed an A, so the data stays reachable.
  const [editingFiscal, setEditingFiscal] = useState(false);
  const [issuing, startIssue] = useTransition();

  // A refresh that fails keeps the preview on screen; only the first read has nothing to show.
  const reload = useCallback(async () => {
    try {
      setPreview(await getInvoicePreview(quoteId, branchId));
    } catch (cause) {
      toast.error(message(errorCodeOf(cause)));
    }
  }, [branchId, message, quoteId]);

  // The client is part of what the invoice says, so a change of client is a new preview.
  useEffect(() => {
    let active = true;
    getInvoicePreview(quoteId, branchId)
      .then((result) => {
        if (!active) return;
        setPreview(result);
        setLoadError(null);
      })
      .catch((cause) => {
        if (active) setLoadError(message(errorCodeOf(cause)));
      });
    return () => {
      active = false;
    };
  }, [branchId, clientId, message, quoteId]);

  function issue() {
    setRejection([]);
    startIssue(async () => {
      try {
        const invoice = await issueInvoice(quoteId, branchId);
        setConfirming(false);
        toast.success(t('issued', { type: invoice.type, number: invoiceNumberOf(invoice) }));
        setPreview((current) => (current ? { ...current, invoice, issues: [] } : current));
      } catch (cause) {
        setConfirming(false);
        const code = errorCodeOf(cause);
        if (code === 'INVOICE_REJECTED' && cause instanceof ApiError) setRejection(cause.issues);
        toast.error(message(code));
        await reload();
      }
    });
  }

  const issued = preview?.invoice?.status === 'ISSUED' ? preview.invoice : null;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-x-2">
          <ReceiptTextIcon aria-hidden="true" className="size-4 text-foreground-muted" />
          <CardTitle>
            {issued
              ? t('issuedTitle', { type: issued.type, number: invoiceNumberOf(issued) })
              : preview
                ? t('draftTitle', { type: preview.type })
                : t('title')}
          </CardTitle>
        </div>
        {preview && !issued ? <CardDescription>{t('draftHint')}</CardDescription> : null}
      </CardHeader>
      <CardContent>
        {loadError ? (
          <Callout tone="danger">{loadError}</Callout>
        ) : !preview ? (
          <div className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted">
            <Spinner size="sm" />
            {t('loading')}
          </div>
        ) : issued ? (
          <IssuedInvoice invoice={issued} />
        ) : (
          <div className="flex flex-col gap-y-4">
            {preview.invoice?.status === 'PENDING' ? (
              <Callout tone="info" title={t('pending.title')}>
                {t('pending.body')}
              </Callout>
            ) : null}
            <RejectionNotice
              reasons={preview.invoice?.status === 'REJECTED' ? preview.invoice.issues : rejection}
            />
            {preview.issues.map((issue) =>
              isSetupIssue(issue) ? (
                <SetupNotice key={issue} issue={issue} isAdmin={isAdmin} />
              ) : (
                <Callout key={issue} tone="warning" title={t(`issues.${issue}.title`)}>
                  {t(`issues.${issue}.body`)}
                  {RECEIVER_ISSUES.has(issue) && clientId === null ? ` ${t('noClient')}` : null}
                </Callout>
              ),
            )}
            {clientId &&
            (editingFiscal || preview.issues.some((issue) => RECEIVER_ISSUES.has(issue))) ? (
              <ClientFiscalForm
                key={clientId}
                clientId={clientId}
                onSaved={() => {
                  setEditingFiscal(false);
                  void reload();
                }}
              />
            ) : null}

            <ReceiverFacts receiver={preview.receiver} />
            {clientId &&
            !editingFiscal &&
            !preview.issues.some((issue) => RECEIVER_ISSUES.has(issue)) ? (
              <div>
                <Button variant="outline" onClick={() => setEditingFiscal(true)}>
                  {t('fiscal.edit')}
                </Button>
              </div>
            ) : null}
            <AmountsTable amounts={preview.amounts} currency={preview.currency} />

            {preview.issues.length === 0 ? (
              <div className="flex justify-end">
                <PendingButton
                  pending={issuing}
                  pendingLabel={t('issuing')}
                  onClick={() => setConfirming(true)}
                >
                  {t('issue', { type: preview.type })}
                </PendingButton>
              </div>
            ) : null}

            <ConfirmDialog
              open={confirming}
              onOpenChange={(open) => !issuing && setConfirming(open)}
              entity={preview}
              tone="default"
              title={t('confirm.title', { type: preview.type })}
              description={(shown) =>
                t('confirm.description', {
                  type: shown.type,
                  receiver: shown.receiver.name || t('finalConsumer'),
                  total: fmt.currency(shown.amounts.total, shown.currency),
                })
              }
              onConfirm={issue}
              pending={issuing}
              labels={{
                confirm: t('confirm.confirm'),
                pending: t('confirm.confirming'),
                cancel: t('confirm.cancel'),
              }}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function invoiceNumberOf(invoice: Invoice): string {
  return formatInvoiceNumber(invoice.pointOfSale, invoice.number ?? 0);
}

function IssuedInvoice({ invoice }: { invoice: Invoice }) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card');

  return (
    <div className="flex flex-col gap-y-4">
      <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-3">
        <Fact label={t('issuedOn')}>{fmt.date(invoice.issuedOn)}</Fact>
        <Fact label={t('cae')}>
          <span className="flex items-center gap-x-2">
            <span className="tabular-nums">{invoice.cae}</span>
            {invoice.cae ? (
              <CopyButton
                value={invoice.cae}
                labels={{ copy: t('caeCopy'), copied: t('caeCopied') }}
                onCopyError={() => toast.error(t('caeCopyFailed'))}
              />
            ) : null}
          </span>
        </Fact>
        <Fact label={t('caeExpires')}>
          {invoice.caeExpiresOn ? fmt.date(invoice.caeExpiresOn) : null}
        </Fact>
      </dl>
      <ReceiverFacts receiver={invoice.receiver} />
      <AmountsTable amounts={invoice.amounts} currency={invoice.currency} />
      {invoice.qrUrl ? (
        <InlineLink href={invoice.qrUrl} target="_blank" rel="noopener noreferrer">
          {t('verify')}
          <ExternalLinkIcon aria-hidden="true" />
        </InlineLink>
      ) : null}
    </div>
  );
}

function RejectionNotice({ reasons }: { reasons: string[] }) {
  const t = useTranslations('invoicing.card.rejected');
  if (reasons.length === 0) return null;

  return (
    <Callout tone="danger" title={t('title')}>
      {/* ARCA's own wording, verbatim: it names the field it refused. */}
      <ul className="flex flex-col gap-y-1">
        {reasons.map((reason, index) => (
          <li key={index}>{reason}</li>
        ))}
      </ul>
      <p className="mt-1">{t('retry')}</p>
    </Callout>
  );
}

function ReceiverFacts({ receiver }: { receiver: InvoiceReceiver }) {
  const t = useTranslations('invoicing.card');
  const tDocs = useTranslations('invoicing.docTypes');
  const tConditions = useTranslations('invoicing.ivaConditions');
  const document =
    receiver.docType === 'NONE' || !receiver.docNumber
      ? tDocs('NONE')
      : t('document', {
          type: tDocs(receiver.docType),
          number: receiver.docType === 'CUIT' ? formatCuit(receiver.docNumber) : receiver.docNumber,
        });

  return (
    <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-3">
      <Fact label={t('receiver')}>{receiver.name || t('finalConsumer')}</Fact>
      <Fact label={t('documentLabel')}>{document}</Fact>
      <Fact label={t('condition')}>{tConditions(receiver.ivaCondition)}</Fact>
    </dl>
  );
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col min-w-0 gap-y-0.5">
      <dt className="text-paragraph-xs text-foreground-muted">{label}</dt>
      <dd className="text-foreground wrap-anywhere">{children}</dd>
    </div>
  );
}

function AmountsTable({ amounts, currency }: { amounts: InvoiceAmounts; currency?: string }) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card.amounts');
  const tRates = useTranslations('invoicing.vatRates');
  const rows = [
    { key: 'net', label: t('net'), amount: amounts.net },
    ...amounts.byRate.map((row) => ({
      key: row.rate,
      label: tRates(row.rate),
      amount: row.amount,
    })),
    ...(amounts.exempt !== NO_AMOUNT
      ? [{ key: 'exempt', label: t('exempt'), amount: amounts.exempt }]
      : []),
  ];

  return (
    <Table figures="end">
      <TableCaption className="sr-only">{t('caption')}</TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead kind="text">{t('concept')}</TableHead>
          <TableHead kind="money">{t('amount')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.key}>
            <TableCell>{row.label}</TableCell>
            <TableCell kind="money">{fmt.currency(row.amount, currency)}</TableCell>
          </TableRow>
        ))}
        <TableRow>
          <TableCell className="text-paragraph-sm-semibold">{t('total')}</TableCell>
          <TableCell kind="money" className="text-paragraph-sm-semibold">
            {fmt.currency(amounts.total, currency)}
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  );
}
