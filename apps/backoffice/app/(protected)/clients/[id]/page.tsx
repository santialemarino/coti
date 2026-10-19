import Link from 'next/link';
import { notFound } from 'next/navigation';
import { CalendarDaysIcon, MailIcon, PhoneIcon, UserRoundIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import {
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
import { cn } from '@repo/ui/lib';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { PageShell } from '@/app/(protected)/_components/page-shell';
import { ClientTagEditor } from '@/app/(protected)/clients/[id]/_components/client-tag-editor';
import { RfqStatusBadge } from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import { BackLink } from '@/components/back-link';
import { ROUTES } from '@/config/routes';
import { clientDisplayName, type ClientProfile, type ClientTag } from '@/lib/api/client-profiles';
import { getClient, getClientTags } from '@/lib/api/clients';
import { ApiError, errorCodeOf } from '@/lib/api/errors';
import { formatRfqReference } from '@/lib/api/rfqs';
import { hasQuoteTotal } from '@/lib/constants/rfq-status';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

/*
 * One headline number of the profile's activity roll-up. The counts come straight from the summary
 * the backend derives from the history below, so a tile can never disagree with the table.
 */
function SummaryStat({
  label,
  value,
  tone = 'default',
}: {
  label: string;
  value: number;
  tone?: 'default' | 'success' | 'warning' | 'danger';
}) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-y-1">
        <span className="text-paragraph-xs text-foreground-muted">{label}</span>
        <span
          className={cn(
            'text-heading-3',
            tone === 'success' && 'text-success-foreground',
            tone === 'warning' && 'text-warning-foreground',
            tone === 'danger' && 'text-danger-foreground',
          )}
        >
          {value}
        </span>
      </CardContent>
    </Card>
  );
}

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
    <PageShell>
      <div className="flex flex-col gap-y-2">
        <BackLink href={ROUTES.clients} label={t('backToList')} />
        <PageHeader title={name} description={t('profile.description')} />
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <SummaryStat label={t('profile.summary.total')} value={profile.summary.totalQuotes} />
        <SummaryStat
          label={t('profile.summary.accepted')}
          value={profile.summary.acceptedQuotes}
          tone="success"
        />
        <SummaryStat
          label={t('profile.summary.pending')}
          value={profile.summary.pendingQuotes}
          tone="warning"
        />
        <SummaryStat
          label={t('profile.summary.rejected')}
          value={profile.summary.rejectedQuotes}
          tone="danger"
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
            <div className="flex items-start gap-x-3">
              <CalendarDaysIcon
                aria-hidden="true"
                className="mt-0.5 size-4 text-foreground-subtle"
              />
              <div className="flex flex-col gap-y-0.5">
                <span className="text-paragraph-xs text-foreground-muted">
                  {t('profile.since')}
                </span>
                <span className="text-paragraph-sm text-foreground">
                  {fmt.dateNumeric(profile.client.createdAt)}
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
          <CardTitle className="text-heading-3">{t('profile.historyTitle')}</CardTitle>
        </CardHeader>
        <Table>
          <TableCaption className="sr-only">{t('profile.historyCaption')}</TableCaption>
          <TableHeader>
            <TableRow>
              <TableHead kind="short">{t('profile.table.quote')}</TableHead>
              <TableHead kind="text">{t('profile.table.branch')}</TableHead>
              <TableHead kind="status">{t('profile.table.status')}</TableHead>
              <TableHead kind="date">{t('profile.table.date')}</TableHead>
              <TableHead kind="money">{t('profile.table.total')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {profile.quotes.length === 0 ? (
              <TableEmptyRow
                colSpan={5}
                icon={UserRoundIcon}
                title={t('profile.emptyHistory.title')}
                description={t('profile.emptyHistory.description')}
              />
            ) : (
              profile.quotes.map((quote) => (
                <TableRow key={quote.quoteId}>
                  <TableCell kind="short">
                    <InlineLink asChild>
                      <Link href={ROUTES.rfqsDetail(quote.rfqId)}>
                        {formatRfqReference(quote.quoteNumber) ?? t('profile.numberPending')}
                      </Link>
                    </InlineLink>
                  </TableCell>
                  <TableCell>{quote.branchName}</TableCell>
                  <TableCell kind="status">
                    <RfqStatusBadge status={quote.status} archived={quote.archived} size="sm" />
                  </TableCell>
                  <TableCell kind="date">{fmt.dateNumeric(quote.updatedAt)}</TableCell>
                  <TableCell kind="money" className="text-paragraph-sm-medium">
                    {hasQuoteTotal(quote.status) && quote.total != null ? (
                      fmt.currency(quote.total)
                    ) : (
                      <span className="text-foreground-subtle" aria-hidden="true">
                        —
                      </span>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>
    </PageShell>
  );
}
