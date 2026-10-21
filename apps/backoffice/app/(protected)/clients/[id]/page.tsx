import Link from 'next/link';
import { notFound } from 'next/navigation';
import { MailIcon, PhoneIcon, UserRoundIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  InlineLink,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ClientFiscalForm } from '@/app/(protected)/clients/[id]/_components/client-fiscal-form';
import { ClientTagEditor } from '@/app/(protected)/clients/[id]/_components/client-tag-editor';
import { BackLink } from '@/components/back-link';
import { ROUTES } from '@/config/routes';
import { getAccount } from '@/lib/api/account';
import {
  clientDisplayName,
  type ClientProfile,
  type ClientTag,
  type SaleInvoiceStatus,
} from '@/lib/api/client-profiles';
import { getClient, getClientFiscal, getClientTags } from '@/lib/api/clients';
import { ApiError, errorCodeOf } from '@/lib/api/errors';
import type { ClientFiscal } from '@/lib/api/invoicing';
import { formatRfqReference } from '@/lib/api/rfqs';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

const INVOICE_STATUS_TONE: Record<
  SaleInvoiceStatus,
  'success' | 'neutral' | 'outline' | 'warning'
> = {
  INVOICED: 'success',
  PENDING: 'neutral',
  WAIVED: 'outline',
  UNINVOICED: 'warning',
};

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientProfilePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations('clients');
  const fmt = await getFormatters();
  let profile: ClientProfile;
  let availableTags: ClientTag[];
  let fiscal: ClientFiscal | null;
  // A client's fiscal data only serves invoicing with Coti, so it is not even read while that is off.
  const account = await getAccount();
  try {
    [profile, availableTags, fiscal] = await Promise.all([
      getClient(id),
      getClientTags(),
      account.invoicingEnabled ? getClientFiscal(id) : null,
    ]);
  } catch (error) {
    if (error instanceof ApiError && errorCodeOf(error) === 'NOT_FOUND') notFound();
    throw error;
  }
  const name = clientDisplayName(profile.client, t('unnamed'));

  return (
    <PageShell>
      <div className="flex flex-col gap-y-2">
        <BackLink href={ROUTES.clients} label={t('backToList')} />
        <PageHeader
          title={name}
          description={t(fiscal ? 'profile.description' : 'profile.descriptionWithoutFiscal')}
          actions={<Badge tone="success">{t('salesCount', { total: profile.sales.length })}</Badge>}
        />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <Card>
          <CardHeader className="flex-row items-center gap-x-2">
            <UserRoundIcon aria-hidden="true" className="size-4 text-foreground-muted" />
            <CardTitle>{t('profile.contactTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-y-4">
            <div className="flex items-start gap-x-3">
              <PhoneIcon aria-hidden="true" className="mt-0.5 size-4 text-foreground-subtle" />
              <div className="flex flex-col gap-y-0.5">
                <span className="text-paragraph-xs text-foreground-muted">
                  {t('profile.phone')}
                </span>
                <span className="text-paragraph-sm text-foreground">
                  {profile.client.phone ?? t('profile.notRegistered')}
                </span>
              </div>
            </div>
            <div className="flex items-start gap-x-3">
              <MailIcon aria-hidden="true" className="mt-0.5 size-4 text-foreground-subtle" />
              <div className="flex min-w-0 flex-col gap-y-0.5">
                <span className="text-paragraph-xs text-foreground-muted">
                  {t('profile.email')}
                </span>
                <span className="break-all text-paragraph-sm text-foreground">
                  {profile.client.email ?? t('profile.notRegistered')}
                </span>
              </div>
            </div>
          </CardContent>
        </Card>

        <ClientTagEditor
          clientId={profile.client.id}
          initialTags={profile.tags}
          initialAvailable={availableTags}
        />
      </div>

      {fiscal ? <ClientFiscalForm key={id} client={fiscal} /> : null}

      <Card className="gap-y-0 overflow-hidden py-0">
        <CardHeader className="py-6">
          <CardTitle className="text-heading-3">{t('profile.salesTitle')}</CardTitle>
        </CardHeader>
        <Table>
          <TableCaption className="sr-only">{t('profile.salesCaption')}</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead kind="short">{t('profile.table.quote')}</TableHead>
              <TableHead kind="text">{t('profile.table.branch')}</TableHead>
              <TableHead kind="date">{t('profile.table.date')}</TableHead>
              <TableHead kind="money">{t('profile.table.total')}</TableHead>
              {account.invoicingEnabled ? (
                <TableHead kind="status">{t('profile.table.invoice')}</TableHead>
              ) : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {profile.sales.length === 0 ? (
              <TableEmptyRow
                colSpan={account.invoicingEnabled ? 5 : 4}
                icon={UserRoundIcon}
                title={t('profile.emptySales.title')}
                description={t('profile.emptySales.description')}
              />
            ) : (
              profile.sales.map((sale) => (
                /* The reference is the row's one link, stretched over the row, so the whole row
                   opens the order and is still a single stop for the keyboard. */
                <TableRow
                  key={sale.quoteId}
                  interactive
                  className="group/sale has-[a:focus-visible]:ring-3 has-[a:focus-visible]:ring-ring/45 relative"
                >
                  <TableCell kind="short">
                    <InlineLink
                      asChild
                      className="group-hover/sale:decoration-primary after:absolute after:inset-0"
                    >
                      <Link href={ROUTES.rfqsDetail(sale.rfqId)}>
                        {formatRfqReference(sale.quoteNumber) ?? t('profile.numberPending')}
                      </Link>
                    </InlineLink>
                  </TableCell>
                  <TableCell>{sale.branchName}</TableCell>
                  <TableCell kind="date">{fmt.dateNumeric(sale.acceptedAt)}</TableCell>
                  <TableCell kind="money" className="text-paragraph-sm-medium">
                    {fmt.currency(sale.total)}
                  </TableCell>
                  {account.invoicingEnabled ? (
                    <TableCell kind="status">
                      <Badge tone={INVOICE_STATUS_TONE[sale.invoiceStatus]}>
                        {t(`profile.invoiceStatuses.${sale.invoiceStatus}`)}
                      </Badge>
                    </TableCell>
                  ) : null}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>
    </PageShell>
  );
}
