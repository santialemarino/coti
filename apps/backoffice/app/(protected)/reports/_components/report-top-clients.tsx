import { useTranslations } from 'next-intl';

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@repo/ui/components';
import { ReportSection } from '@/app/(protected)/reports/_components/report-section';
import type { SellerReport } from '@/lib/api/reports';
import type { Formatters } from '@/lib/i18n/create-formatters';

// The clients who accepted the most in the period.
export function ReportTopClients({
  clients,
  fmt,
  className,
}: {
  clients: SellerReport['topClients'];
  fmt: Formatters;
  className?: string;
}) {
  const t = useTranslations('reports.clients');

  return (
    <ReportSection
      flush
      title={t('title')}
      empty={clients.length === 0 ? t('empty') : undefined}
      className={className}
    >
      <Table figures="end">
        <TableHeader>
          <TableRow>
            <TableHead kind="text">{t('name')}</TableHead>
            <TableHead kind="money">{t('acceptedAmount')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {clients.map((client, index) => (
            <TableRow key={client.clientId ?? `label-${index}`}>
              <TableCell>{client.name || t('unnamed')}</TableCell>
              <TableCell>{fmt.currency(client.acceptedAmount)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </ReportSection>
  );
}
