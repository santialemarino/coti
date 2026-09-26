import Link from 'next/link';
import { notFound } from 'next/navigation';
import { ArrowLeftIcon, MailIcon, PhoneIcon, UserRoundIcon } from 'lucide-react';
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
import { ClientTagEditor } from '@/app/(protected)/clients/[id]/_components/client-tag-editor';
import { clientDisplayName, type ClientProfile, type ClientTag } from '@/lib/api/client-profiles';
import { getClient, getClientTags } from '@/lib/api/clients';
import { ApiError, errorCodeOf } from '@/lib/api/errors';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientProfilePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const t = await getTranslations('clients');
  const fmt = await getFormatters();
  let profile: ClientProfile;
  let availableTags: ClientTag[];
  try {
    [profile, availableTags] = await Promise.all([getClient(id), getClientTags()]);
  } catch (error) {
    if (error instanceof ApiError && errorCodeOf(error) === 'NOT_FOUND') notFound();
    throw error;
  }
  const name = clientDisplayName(profile.client, t('unnamed'));

  return (
    <main className="flex flex-col gap-y-8">
      <div className="flex flex-col gap-y-4">
        <InlineLink asChild tone="muted">
          <Link href="/clients">
            <ArrowLeftIcon aria-hidden="true" />
            {t('backToList')}
          </Link>
        </InlineLink>
        <div className="flex flex-col sm:flex-row sm:items-end sm:justify-between gap-3">
          <div className="flex flex-col gap-y-1">
            <h1 className="text-heading-2">{name}</h1>
            <p className="text-paragraph text-foreground-muted">{t('profile.description')}</p>
          </div>
          <Badge tone="success">{t('salesCount', { total: profile.sales.length })}</Badge>
        </div>
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

      <Card className="gap-y-0 overflow-hidden py-0">
        <CardHeader className="py-6">
          <CardTitle className="text-heading-3">{t('profile.salesTitle')}</CardTitle>
        </CardHeader>
        <Table>
          <TableCaption className="sr-only">{t('profile.salesCaption')}</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead>{t('profile.table.quote')}</TableHead>
              <TableHead>{t('profile.table.branch')}</TableHead>
              <TableHead>{t('profile.table.date')}</TableHead>
              <TableHead className="text-right">{t('profile.table.total')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {profile.sales.length === 0 ? (
              <TableEmptyRow
                colSpan={4}
                icon={UserRoundIcon}
                title={t('profile.emptySales.title')}
                description={t('profile.emptySales.description')}
              />
            ) : (
              profile.sales.map((sale) => (
                <TableRow key={sale.quoteId}>
                  <TableCell>
                    <InlineLink asChild>
                      <Link href={`/rfqs/${sale.rfqId}`}>#{sale.quoteNumber}</Link>
                    </InlineLink>
                  </TableCell>
                  <TableCell>{sale.branchName}</TableCell>
                  <TableCell>{fmt.date(sale.acceptedAt)}</TableCell>
                  <TableCell className="text-right text-paragraph-sm-medium">
                    {fmt.currency(sale.total)}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>
    </main>
  );
}
