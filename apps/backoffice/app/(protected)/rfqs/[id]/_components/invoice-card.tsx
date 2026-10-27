'use client';

import { useCallback, useEffect, useRef, useState, useTransition } from 'react';
import Link from 'next/link';
import { ExternalLinkIcon, ReceiptTextIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
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
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { IssueInvoiceDialog } from '@/app/(protected)/rfqs/_components/issue-invoice-dialog';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { SignupLinkDialog } from '@/app/(protected)/rfqs/[id]/_components/signup-link-dialog';
import { InvoiceRecoveryButton } from '@/components/invoice-recovery-button';
import { SetupNotice } from '@/components/setup-notice';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { ApiError, errorCodeOf } from '@/lib/api/errors';
import {
  formatCuit,
  formatDni,
  formatInvoiceNumber,
  isCredited,
  type Invoice,
  type InvoiceAmounts,
  type InvoicePreview,
  type InvoiceReceiver,
} from '@/lib/api/invoicing';
import { getInvoicePreview, issueCreditNote } from '@/lib/api/invoicing-client';
import { setInvoiceWaived } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { isSetupIssue } from '@/lib/utils/setup-issues';

const RECEIVER_ISSUES = new Set([
  'RECEIVER_CUIT_REQUIRED',
  'RECEIVER_ID_REQUIRED',
  'RECEIVER_ADDRESS_REQUIRED',
]);
// The gaps a CUIT closes, and only the customer loads one, by signing up.
const SIGNUP_ISSUES = new Set(['RECEIVER_CUIT_REQUIRED', 'RECEIVER_ID_REQUIRED']);
const NO_AMOUNT = '0.00';
const DOC_NUMBER_FORMATS: Record<string, (digits: string) => string> = {
  CUIT: formatCuit,
  DNI: formatDni,
};

interface InvoiceCardProps {
  quoteId: string;
  branchId: string;
  /* The sale's client: null when none is associated, undefined while that is still unknown. */
  clientId: string | null | undefined;
  // An archived sale shows its invoice and draft but offers no issue or recovery until unarchived.
  readOnly?: boolean;
  // The order reference the confirmations name.
  reference: string;
  // «No requiere factura», with who marked it; null while the sale waits for its invoice.
  waiver: { at: string; by: string | null } | null;
  // Runs after the waiver is set or undone, so the screen re-reads the sale.
  onWaiverChanged: () => Promise<void>;
  // Bumped when something outside the card changed the invoice, so it reads it again.
  revision?: number;
}

/*
 * The invoice of an accepted sale: the one ARCA authorized, or what would be issued now and every
 * gap that stops it. Issuing is always a person's confirmed decision — it cannot be undone, only
 * cancelled by a credit note, after which the sale can be invoiced again.
 */
export function InvoiceCard({
  quoteId,
  branchId,
  clientId,
  readOnly = false,
  reference,
  waiver,
  onWaiverChanged,
  revision = 0,
}: InvoiceCardProps) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card');
  const tInvoicing = useTranslations('invoicing');
  const message = useApiErrorMessage('invoicing.card');
  const creditNoteMessage = useApiErrorMessage('invoicing.card.creditNote');
  const { isAdmin } = useRfqList();
  const [preview, setPreview] = useState<InvoicePreview | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  // ARCA's reasons from the attempt just made, until a reload carries them on the invoice itself.
  const [rejection, setRejection] = useState<string[]>([]);
  const [confirming, setConfirming] = useState(false);
  // ARCA's reasons for refusing the credit note just asked for, until a reload carries them.
  const [creditNoteRejection, setCreditNoteRejection] = useState<string[]>([]);
  const [confirmingCreditNote, setConfirmingCreditNote] = useState(false);
  const [creditNoting, startCreditNote] = useTransition();
  const [waiving, startWaive] = useTransition();

  // Only the newest read may land: a slower, older one would put back a receiver or a button
  // the seller has already moved past.
  const latestRead = useRef(0);
  const headingRef = useRef<HTMLDivElement>(null);
  const issueButtonRef = useRef<HTMLButtonElement>(null);
  const creditNoteButtonRef = useRef<HTMLButtonElement>(null);

  // A refresh that fails keeps the preview on screen; only the first read has nothing to show.
  const load = useCallback(
    async (initial: boolean) => {
      const read = ++latestRead.current;
      try {
        const result = await getInvoicePreview(quoteId, branchId);
        if (read !== latestRead.current) return;
        setPreview(result);
        setLoadError(null);
      } catch (cause) {
        if (read !== latestRead.current) return;
        if (initial) setLoadError(message(errorCodeOf(cause)));
        else toast.error(message(errorCodeOf(cause)));
      }
    },
    [branchId, message, quoteId],
  );
  const reload = useCallback(() => load(false), [load]);

  // The client is part of what the invoice says, so a change of client is a new preview.
  useEffect(() => {
    void load(true);
  }, [clientId, load, revision]);

  function markIssued(invoice: Invoice) {
    latestRead.current++;
    setRejection([]);
    setPreview((current) => (current ? { ...current, invoice, issues: [] } : current));
    // The button that opened the dialog is gone with the draft; focus lands on what replaced it.
    requestAnimationFrame(() => headingRef.current?.focus());
  }

  async function issueFailed(cause: unknown) {
    const code = errorCodeOf(cause);
    setRejection(code === 'INVOICE_REJECTED' && cause instanceof ApiError ? cause.issues : []);
    await reload();
  }

  function creditNote(invoice: Invoice) {
    setCreditNoteRejection([]);
    startCreditNote(async () => {
      try {
        const note = await issueCreditNote(invoice.id, branchId);
        setConfirmingCreditNote(false);
        toast.success(
          t('creditNote.issued', {
            type: note.type,
            number: invoiceNumberOf(note),
            invoice: invoiceNumberOf(invoice),
          }),
        );
        await reload();
        // The issued invoice gave way to a new draft; focus lands on the card that replaced it.
        requestAnimationFrame(() => headingRef.current?.focus());
      } catch (cause) {
        setConfirmingCreditNote(false);
        const code = errorCodeOf(cause);
        if (code === 'INVOICE_REJECTED' && cause instanceof ApiError) {
          setCreditNoteRejection(cause.issues);
        }
        toast.error(creditNoteMessage(code));
        await reload();
      }
    });
  }

  function setWaived(waived: boolean) {
    startWaive(async () => {
      try {
        await setInvoiceWaived(quoteId, waived, branchId);
        toast.success(t(waived ? 'waiver.waived' : 'waiver.restored', { reference }));
        await onWaiverChanged();
        requestAnimationFrame(() => headingRef.current?.focus());
      } catch (cause) {
        toast.error(message(errorCodeOf(cause)));
      }
    });
  }

  // A closed dialog hands focus back to the button that opened it, or to the card once that is gone.
  function returnFocus(event: Event, opener: React.RefObject<HTMLButtonElement | null>) {
    event.preventDefault();
    (opener.current?.isConnected ? opener.current : headingRef.current)?.focus();
  }

  const issued = preview?.invoice?.status === 'ISSUED' ? preview.invoice : null;
  // The dialog outlives the invoice it cancels, so its exit still plays once the card reloads.
  const creditNoteTarget = useHeldWhileClosed(issued, confirmingCreditNote);
  // An invoice keeps the environment it was made in; a draft follows the server's.
  const testInvoice = (issued ?? preview?.invoice ?? preview)?.environment === 'HOMOLOGATION';

  return (
    <Card>
      <CardHeader>
        <div ref={headingRef} tabIndex={-1} className="flex items-center gap-x-2 outline-none">
          <ReceiptTextIcon aria-hidden="true" className="size-4 text-foreground-muted" />
          <CardTitle>
            {issued
              ? t('issuedTitle', { type: issued.type, number: invoiceNumberOf(issued) })
              : preview && !waiver
                ? t('draftTitle', { type: preview.type })
                : t('title')}
          </CardTitle>
          {testInvoice ? <Badge tone="outline">{tInvoicing('testBadge')}</Badge> : null}
        </div>
        {preview && !issued && !waiver ? <CardDescription>{t('draftHint')}</CardDescription> : null}
        {testInvoice ? <CardDescription>{t('testNote')}</CardDescription> : null}
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
          <div className="flex flex-col gap-y-4">
            {preview.creditedInvoice ? (
              <CreditedInvoice invoice={preview.creditedInvoice} branchId={branchId} />
            ) : null}
            <IssuedInvoice
              invoice={issued}
              branchId={branchId}
              rejection={creditNoteRejection}
              onRecovered={() => void reload()}
              // A credit note goes to the invoice's environment, which only a server set to it reaches.
              creditable={issued.environment === preview.environment}
              onCreditNote={() => setConfirmingCreditNote(true)}
              creditNoting={creditNoting}
              creditNoteButtonRef={creditNoteButtonRef}
            />
          </div>
        ) : waiver && preview.invoice?.status !== 'PENDING' ? (
          <Callout
            tone="info"
            title={t('waiver.title')}
            action={
              readOnly ? undefined : (
                <PendingButton
                  variant="outline"
                  size="sm"
                  pending={waiving}
                  pendingLabel={t('waiver.undoing')}
                  onClick={() => setWaived(false)}
                >
                  {t('waiver.undo')}
                </PendingButton>
              )
            }
          >
            {t('waiver.body', { name: waiver.by ?? '', date: fmt.date(waiver.at) })}
          </Callout>
        ) : (
          <div className="flex flex-col gap-y-4">
            {preview.creditedInvoice ? (
              <CreditedInvoice invoice={preview.creditedInvoice} branchId={branchId} />
            ) : null}
            {preview.otherEnvironmentInvoice ? (
              <OtherEnvironmentInvoice
                invoice={preview.otherEnvironmentInvoice}
                branchId={branchId}
              />
            ) : null}
            {preview.invoice?.status === 'PENDING' ? (
              <Callout tone="info" title={t('pending.title')}>
                {t('pending.body')}
                {readOnly ? null : (
                  <div className="mt-3">
                    <InvoiceRecoveryButton
                      invoiceId={preview.invoice.id}
                      branchId={branchId}
                      onRecovered={() => void reload()}
                    />
                  </div>
                )}
              </Callout>
            ) : null}
            <RejectionNotice
              reasons={preview.invoice?.status === 'REJECTED' ? preview.invoice.issues : rejection}
            />
            {preview.issues.map((issue) =>
              isSetupIssue(issue) ? (
                <SetupNotice key={issue} issue={issue} isAdmin={isAdmin} tone="info" />
              ) : (
                <Callout
                  key={issue}
                  tone="info"
                  title={t(`issues.${issue}.title`)}
                  action={
                    SIGNUP_ISSUES.has(issue) && preview.customerSignupUrl ? (
                      <SignupLinkDialog url={preview.customerSignupUrl} />
                    ) : undefined
                  }
                >
                  {t(`issues.${issue}.body`)}
                  {RECEIVER_ISSUES.has(issue) && clientId === null ? ` ${t('noClient')}` : null}
                  {SIGNUP_ISSUES.has(issue) && !preview.customerSignupUrl
                    ? ` ${t('signup.noSite')}`
                    : null}
                </Callout>
              ),
            )}
            <IssuerFacts
              cuit={preview.issuerCuit}
              pointOfSale={preview.pointOfSale}
              editable={isAdmin}
            />
            <ReceiverFacts receiver={preview.receiver} />
            {clientId ? (
              <div>
                <Button asChild variant="outline">
                  <Link href={ROUTES.clientDetail(clientId)}>{t('editClient')}</Link>
                </Button>
              </div>
            ) : null}
            <AmountsTable amounts={preview.amounts} currency={preview.currency} />

            {!readOnly && preview.invoice?.status !== 'PENDING' ? (
              <div className="flex flex-wrap justify-end gap-2">
                <PendingButton
                  variant="outline"
                  pending={waiving}
                  pendingLabel={t('waiver.pending')}
                  onClick={() => setWaived(true)}
                >
                  {t('waiver.action')}
                </PendingButton>
                {preview.issues.length === 0 ? (
                  <Button
                    ref={issueButtonRef}
                    disabled={waiving}
                    onClick={() => setConfirming(true)}
                  >
                    {t('issue', { type: preview.type })}
                  </Button>
                ) : null}
              </div>
            ) : null}

            <IssueInvoiceDialog
              open={confirming}
              onOpenChange={setConfirming}
              quoteId={quoteId}
              branchId={branchId}
              preview={preview}
              onIssued={markIssued}
              onFailed={issueFailed}
              onCloseAutoFocus={(event) => returnFocus(event, issueButtonRef)}
            />
          </div>
        )}
        {creditNoteTarget ? (
          <ConfirmDialog
            open={confirmingCreditNote}
            onOpenChange={(open) => !creditNoting && setConfirmingCreditNote(open)}
            entity={creditNoteTarget}
            tone="danger"
            title={t('creditNote.confirm.title', { type: creditNoteTarget.type })}
            description={(shown) =>
              t('creditNote.confirm.description', {
                type: shown.type,
                number: invoiceNumberOf(shown),
                receiver: shown.receiver.name || t('finalConsumer'),
                total: fmt.currency(shown.amounts.total, shown.currency),
              })
            }
            onConfirm={() => creditNote(creditNoteTarget)}
            onCloseAutoFocus={(event) => returnFocus(event, creditNoteButtonRef)}
            pending={creditNoting}
            labels={{
              confirm: t('creditNote.confirm.confirm'),
              pending: t('creditNote.confirm.confirming'),
              cancel: t('creditNote.confirm.cancel'),
            }}
          />
        ) : null}
      </CardContent>
    </Card>
  );
}

function invoiceNumberOf(invoice: Invoice): string {
  return formatInvoiceNumber(invoice.pointOfSale, invoice.number ?? 0);
}

function IssuedInvoice({
  invoice,
  branchId,
  rejection,
  onRecovered,
  creditable,
  onCreditNote,
  creditNoting,
  creditNoteButtonRef,
}: {
  invoice: Invoice;
  branchId: string;
  /* ARCA's reasons for refusing the credit note just asked for. */
  rejection: string[];
  onRecovered: () => void;
  creditable: boolean;
  onCreditNote: () => void;
  creditNoting: boolean;
  creditNoteButtonRef: React.RefObject<HTMLButtonElement | null>;
}) {
  const t = useTranslations('invoicing.card');
  const note = invoice.creditNote;

  return (
    <div className="flex flex-col gap-y-4">
      {note?.status === 'PENDING' ? (
        <Callout tone="info" title={t('creditNote.pending.title')}>
          {t('creditNote.pending.body')}
          <div className="mt-3">
            <InvoiceRecoveryButton
              invoiceId={note.id}
              branchId={branchId}
              onRecovered={onRecovered}
            />
          </div>
        </Callout>
      ) : null}
      <RejectionNotice
        reasons={note?.status === 'REJECTED' ? note.issues : rejection}
        namespace="invoicing.card.creditNote.rejected"
      />
      <AuthorizationFacts voucher={invoice} />
      <IssuerFacts cuit={invoice.issuerCuit} pointOfSale={invoice.pointOfSale} />
      <ReceiverFacts receiver={invoice.receiver} />
      <AmountsTable amounts={invoice.amounts} currency={invoice.currency} />
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <Button asChild variant="outline">
          <a href={ROUTES.invoicePDF(invoice.id, branchId)}>{t('download')}</a>
        </Button>
        {creditable && note?.status !== 'PENDING' ? (
          <PendingButton
            ref={creditNoteButtonRef}
            variant="outline"
            pending={creditNoting}
            pendingLabel={t('creditNote.issuing')}
            onClick={onCreditNote}
          >
            {t('creditNote.issue')}
          </PendingButton>
        ) : null}
      </div>
      {invoice.qrUrl ? (
        <InlineLink href={invoice.qrUrl} target="_blank" rel="noopener noreferrer">
          {t('verify')}
          <ExternalLinkIcon aria-hidden="true" />
        </InlineLink>
      ) : null}
    </div>
  );
}

// The date and ARCA's authorization of an issued voucher.
function AuthorizationFacts({ voucher }: { voucher: Invoice }) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card');

  return (
    <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-3">
      <Fact label={t('issuedOn')}>{fmt.date(voucher.issuedOn)}</Fact>
      <Fact label={t('cae')}>
        <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="whitespace-nowrap tabular-nums">{voucher.cae}</span>
          {voucher.cae ? (
            <CopyButton
              value={voucher.cae}
              labels={{ copy: t('caeCopy'), copied: t('caeCopied') }}
              onCopyError={() => toast.error(t('caeCopyFailed'))}
            />
          ) : null}
        </span>
      </Fact>
      <Fact label={t('caeExpires')}>
        {voucher.caeExpiresOn ? fmt.date(voucher.caeExpiresOn) : null}
      </Fact>
    </dl>
  );
}

// An invoice an issued credit note cancelled: history, with the note that cancelled it.
function CreditedInvoice({ invoice, branchId }: { invoice: Invoice; branchId: string }) {
  const t = useTranslations('invoicing.card.credited');
  const tInvoicing = useTranslations('invoicing');
  const note = invoice.creditNote;
  if (!note || !isCredited(invoice)) return null;

  return (
    <Callout
      tone="info"
      title={t('title', {
        type: invoice.type,
        number: invoiceNumberOf(invoice),
        noteType: note.type,
        noteNumber: invoiceNumberOf(note),
      })}
    >
      <div className="flex flex-col items-start gap-y-3">
        {note.environment === 'HOMOLOGATION' ? (
          <Badge tone="outline">{tInvoicing('testBadge')}</Badge>
        ) : null}
        <p>{t('body')}</p>
        <AuthorizationFacts voucher={note} />
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <Button asChild variant="outline" size="sm">
            <a href={ROUTES.invoicePDF(note.id, branchId)}>{t('downloadNote')}</a>
          </Button>
          <Button asChild variant="outline" size="sm">
            <a href={ROUTES.invoicePDF(invoice.id, branchId)}>{t('downloadInvoice')}</a>
          </Button>
        </div>
        {note.qrUrl ? (
          <InlineLink href={note.qrUrl} target="_blank" rel="noopener noreferrer">
            {t('verify')}
            <ExternalLinkIcon aria-hidden="true" />
          </InlineLink>
        ) : null}
      </div>
    </Callout>
  );
}

// An invoice the sale has in the other ARCA environment: shown as history, it holds nothing here.
function OtherEnvironmentInvoice({ invoice, branchId }: { invoice: Invoice; branchId: string }) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.card.otherInvoice');
  const tInvoicing = useTranslations('invoicing');

  return (
    <Callout
      tone="info"
      title={t('title', { type: invoice.type, number: invoiceNumberOf(invoice) })}
      action={
        <Button asChild variant="outline" size="sm">
          <a href={ROUTES.invoicePDF(invoice.id, branchId)}>{t('download')}</a>
        </Button>
      }
    >
      <span className="flex flex-col items-start gap-y-1.5">
        {invoice.environment === 'HOMOLOGATION' ? (
          <Badge tone="outline">{tInvoicing('testBadge')}</Badge>
        ) : null}
        {t(`${invoice.environment}.body`, { date: fmt.date(invoice.issuedOn) })}
      </span>
    </Callout>
  );
}

function RejectionNotice({
  reasons,
  namespace = 'invoicing.card.rejected',
}: {
  reasons: string[];
  namespace?: 'invoicing.card.rejected' | 'invoicing.card.creditNote.rejected';
}) {
  const t = useTranslations(namespace);
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
          number: (DOC_NUMBER_FORMATS[receiver.docType] ?? String)(receiver.docNumber),
        });

  return (
    <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-3">
      <Fact label={t('receiver')}>{receiver.name || t('finalConsumer')}</Fact>
      <Fact label={t('documentLabel')}>{document}</Fact>
      <Fact label={t('condition')}>{tConditions(receiver.ivaCondition)}</Fact>
      <Fact label={t('receiverAddress')}>{receiver.address || '—'}</Fact>
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

function IssuerFacts({
  cuit,
  pointOfSale,
  editable = false,
}: {
  cuit: string;
  pointOfSale: number | null;
  editable?: boolean;
}) {
  const t = useTranslations('invoicing.card.issuer');
  return (
    <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-2">
      <Fact label={t('cuit')}>
        <div className="flex flex-col items-start gap-y-1">
          <span>{cuit ? formatCuit(cuit) : '—'}</span>
          {editable ? (
            <InlineLink asChild>
              <Link href={ROUTES.arcaSettings}>{t('account')}</Link>
            </InlineLink>
          ) : null}
        </div>
      </Fact>
      <Fact label={t('pointOfSale')}>
        <div className="flex flex-col items-start gap-y-1">
          <span>{pointOfSale ?? '—'}</span>
          {editable ? (
            <InlineLink asChild>
              <Link href={ROUTES.invoicingSection('points-of-sale')}>{t('branch')}</Link>
            </InlineLink>
          ) : null}
        </div>
      </Fact>
    </dl>
  );
}
