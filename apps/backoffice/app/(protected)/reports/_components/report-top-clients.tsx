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
      {Array.from(new Set(clients.map((client) => client.currency))).map((currency) => (
        <div key={currency} className="py-3">
          <h3 className="px-5 pb-2 text-paragraph-sm-semibold text-foreground-muted">{currency}</h3>
          <Table figures="end">
            <TableHeader>
              <TableRow>
                <TableHead kind="text">{t('name')}</TableHead>
                <TableHead kind="money">{t('acceptedAmount')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {clients
                .filter((client) => client.currency === currency)
                .map((client, index) => (
                  <TableRow key={client.clientId ?? `label-${index}`}>
                    <TableCell>{client.name || t('unnamed')}</TableCell>
                    <TableCell kind="money">
                      {currency} {fmt.currency(client.acceptedAmount, currency)}
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        </div>
      ))}
    </ReportSection>
  );
}
