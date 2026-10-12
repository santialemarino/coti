import Link from 'next/link';
import { ReceiptTextIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
  Badge,
  Button,
  Card,
  CardContent,
  EmptyState,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { InvoiceRecoveryButton } from '@/components/invoice-recovery-button';
import { ROUTES } from '@/config/routes';
import { getBranches } from '@/lib/api/branches';
import { getInvoices } from '@/lib/api/invoices';
import { formatInvoiceNumber } from '@/lib/api/invoicing';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { getSession } from '@/lib/auth/session';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('administration');

export default async function AdministrationPage({
  searchParams,
}: {
  searchParams: Promise<{ page?: string }>;
}) {
  const t = await getTranslations('invoicing.history');
  const fmt = await getFormatters();
  const session = await getSession();
  const branchId = await getEffectiveBranchId(await getBranches());
  const requested = Number((await searchParams).page ?? '1');
  const page = Number.isInteger(requested) && requested > 0 && requested <= 100000 ? requested : 1;
  const result = branchId
    ? await getInvoices(branchId, page)
    : { items: [], total: 0, page, pageSize: 25 };
  const pages = Math.max(1, Math.ceil(result.total / result.pageSize));
  return (
    <PageShell>
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <>
            {session?.role === 'ADMIN' ? (
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
      {!branchId ? (
        <p className="text-paragraph-md text-foreground-muted">{t('selectBranch')}</p>
      ) : result.items.length === 0 ? (
        <EmptyState icon={ReceiptTextIcon} title={t('empty')} />
      ) : (
        <Card>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead kind="text">{t('date')}</TableHead>
                  <TableHead kind="text">{t('number')}</TableHead>
                  <TableHead kind="text">{t('receiver')}</TableHead>
                  <TableHead kind="text">{t('status')}</TableHead>
                  <TableHead kind="money">{t('amount')}</TableHead>
                  <TableHead kind="text">{t('cae')}</TableHead>
                  <TableHead kind="text">{t('actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {result.items.map((invoice) => (
                  <TableRow key={invoice.id}>
                    <TableCell>{fmt.date(invoice.issuedOn)}</TableCell>
                    <TableCell>
                      {invoice.type}{' '}
                      {invoice.number === null
                        ? '—'
                        : formatInvoiceNumber(invoice.pointOfSale, invoice.number)}
                    </TableCell>
                    <TableCell>{invoice.receiver.name || t('finalConsumer')}</TableCell>
                    <TableCell>
                      <Badge
                        tone={
                          invoice.status === 'ISSUED'
                            ? 'success'
                            : invoice.status === 'REJECTED'
                              ? 'danger'
                              : 'neutral'
                        }
                      >
                        {t(`statuses.${invoice.status}`)}
                      </Badge>
                    </TableCell>
                    <TableCell kind="money">
                      {fmt.currency(invoice.amounts.total, invoice.currency)}
                    </TableCell>
                    <TableCell className="tabular-nums">{invoice.cae ?? '—'}</TableCell>
                    <TableCell>
                      {invoice.status === 'ISSUED' ? (
                        <Button asChild variant="outline" size="sm">
                          <a href={ROUTES.invoicePDF(invoice.id, branchId)}>{t('download')}</a>
                        </Button>
                      ) : invoice.status === 'PENDING' ? (
                        <InvoiceRecoveryButton invoiceId={invoice.id} branchId={branchId} />
                      ) : (
                        invoice.issues.join(' · ')
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
      {pages > 1 ? (
        <div className="flex items-center justify-between gap-x-4">
          <Button
            asChild
            variant="outline"
            aria-disabled={page <= 1}
            tabIndex={page <= 1 ? -1 : undefined}
          >
            <Link href={`${ROUTES.administration}?page=${Math.max(1, page - 1)}`}>
              {t('previous')}
            </Link>
          </Button>
          <p className="text-paragraph-sm text-foreground-muted">{t('page', { page, pages })}</p>
          <Button
            asChild
            variant="outline"
            aria-disabled={page >= pages}
            tabIndex={page >= pages ? -1 : undefined}
          >
            <Link href={`${ROUTES.administration}?page=${Math.min(pages, page + 1)}`}>
              {t('next')}
            </Link>
          </Button>
        </div>
      ) : null}
    </PageShell>
  );
}
