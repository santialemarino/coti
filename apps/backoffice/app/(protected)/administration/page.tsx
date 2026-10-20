import Link from 'next/link';
import { notFound } from 'next/navigation';
import { ReceiptTextIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
  Badge,
  Button,
  Card,
  EmptyState,
  InlineLink,
  MetaList,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { InvoicePagination } from '@/app/(protected)/administration/_components/invoice-pagination';
import { InvoiceRecoveryButton } from '@/components/invoice-recovery-button';
import { ROUTES } from '@/config/routes';
import { getAccount, showsInvoiceHistory } from '@/lib/api/account';
import { getBranches } from '@/lib/api/branches';
import { getInvoices } from '@/lib/api/invoices';
import { formatInvoiceNumber, isCredited, type Invoice } from '@/lib/api/invoicing';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

// The API pages the history up to here; anything past it reads as the first page.
const PAGE_MAX = 100000;

const STATUS_TONE: Record<Invoice['status'] | 'CREDITED', 'success' | 'danger' | 'neutral'> = {
  ISSUED: 'success',
  REJECTED: 'danger',
  PENDING: 'neutral',
  CREDITED: 'neutral',
};

export const generateMetadata = () => generatePageMetadata('administration');

export default async function AdministrationPage({
  searchParams,
}: {
  searchParams: Promise<{ page?: string }>;
}) {
  if (!showsInvoiceHistory(await getAccount())) notFound();
  const session = await getSession();
  const fmt = await getFormatters();
  const t = await getTranslations('invoicing.history');
  const tInvoicing = await getTranslations('invoicing');
  const branches = await getBranches();
  const branchId = await getEffectiveBranchId(branches);
  const requested = Number((await searchParams).page ?? '1');
  const page =
    Number.isInteger(requested) && requested > 0 && requested <= PAGE_MAX ? requested : 1;
  const result = await getInvoices(page);
  const pageCount = Math.max(1, Math.ceil(result.total / result.pageSize));
  // With every branch in view, each row says which one it belongs to.
  const showBranch = !branchId;
  const branchNames = new Map(branches.map((branch) => [branch.id, branch.name]));

  return (
    <PageShell>
      <PageHeader
        title={t('title')}
        description={t(showBranch ? 'descriptionAll' : 'description')}
        actions={
          <>
            {session?.role === ADMIN_ROLE ? (
              <Button asChild variant="outline">
                <Link href={ROUTES.invoicingSettings}>{t('settings')}</Link>
              </Button>
            ) : null}
            <Button asChild>
              <Link href={ROUTES.rfqs}>{t('orders')}</Link>
            </Button>
          </>
        }
      />
      {result.items.length === 0 ? (
        <EmptyState icon={ReceiptTextIcon} title={t(showBranch ? 'emptyAll' : 'empty')} />
      ) : (
        <Card className="py-0 gap-y-0 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead kind="date">{t('date')}</TableHead>
                <TableHead kind="short">{t('number')}</TableHead>
                <TableHead kind="text">{t('receiver')}</TableHead>
                {showBranch ? <TableHead kind="text">{t('branch')}</TableHead> : null}
                <TableHead kind="status">{t('status')}</TableHead>
                <TableHead kind="money">{t('amount')}</TableHead>
                <TableHead kind="short">{t('cae')}</TableHead>
                <TableHead kind="actionsWide">{t('actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {result.items.map((invoice) => {
                // An issued invoice a credit note cancelled reads as cancelled, not as authorized.
                const status = isCredited(invoice) ? 'CREDITED' : invoice.status;
                const credited = invoice.creditedInvoice;
                return (
                  <TableRow key={invoice.id}>
                    <TableCell kind="date">{fmt.dateNumeric(invoice.issuedOn)}</TableCell>
                    <TableCell kind="short" className="tabular-nums">
                      <span className="flex flex-col items-start gap-y-0.5">
                        <span>
                          {invoice.kind === 'CREDIT_NOTE'
                            ? t('creditNoteType', { type: invoice.type })
                            : invoice.type}
                          {invoice.number === null
                            ? null
                            : ` ${formatInvoiceNumber(invoice.pointOfSale, invoice.number)}`}
                        </span>
                        {credited?.number != null ? (
                          <InlineLink
                            tone="muted"
                            href={ROUTES.invoicePDF(credited.id, invoice.branchId)}
                            className="text-paragraph-xs"
                          >
                            {t('credits', {
                              type: credited.type,
                              number: formatInvoiceNumber(credited.pointOfSale, credited.number),
                            })}
                          </InlineLink>
                        ) : null}
                      </span>
                    </TableCell>
                    <TableCell>{invoice.receiver.name || t('finalConsumer')}</TableCell>
                    {showBranch ? (
                      <TableCell>{branchNames.get(invoice.branchId) ?? '—'}</TableCell>
                    ) : null}
                    <TableCell kind="status">
                      <span className="flex flex-wrap gap-1">
                        <Badge tone={STATUS_TONE[status]}>{t(`statuses.${status}`)}</Badge>
                        {invoice.environment === 'HOMOLOGATION' ? (
                          <Badge tone="outline">{tInvoicing('testBadge')}</Badge>
                        ) : null}
                      </span>
                    </TableCell>
                    <TableCell kind="money">
                      {fmt.currency(invoice.amounts.total, invoice.currency)}
                    </TableCell>
                    <TableCell kind="short" className="tabular-nums">
                      {invoice.cae ?? '—'}
                    </TableCell>
                    <TableCell kind="actionsWide">
                      <InvoiceAction invoice={invoice} downloadLabel={t('download')} />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}
      {pageCount > 1 ? (
        <div className="flex justify-center">
          <InvoicePagination page={page} pageCount={pageCount} />
        </div>
      ) : null}
    </PageShell>
  );
}

function InvoiceAction({ invoice, downloadLabel }: { invoice: Invoice; downloadLabel: string }) {
  if (invoice.status === 'ISSUED') {
    return (
      <Button asChild variant="outline" size="sm">
        <a href={ROUTES.invoicePDF(invoice.id, invoice.branchId)}>{downloadLabel}</a>
      </Button>
    );
  }
  if (invoice.status === 'PENDING') {
    return <InvoiceRecoveryButton invoiceId={invoice.id} branchId={invoice.branchId} />;
  }
  return (
    <MetaList
      items={invoice.issues}
      className="justify-center text-paragraph-xs text-foreground-muted"
    />
  );
}
