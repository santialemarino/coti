import { useTranslations } from 'next-intl';

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@repo/ui/components';
import { ReportSection } from '@/app/(protected)/reports/_components/report-section';
import type { SellerReport } from '@/lib/api/reports';
import type { Formatters } from '@/lib/i18n/create-formatters';

// Order volume per branch, for a report that spans several.
export function ReportBranches({
  branches,
  fmt,
}: {
  branches: SellerReport['branches'];
  fmt: Formatters;
}) {
  const t = useTranslations('reports.branches');

  return (
    <ReportSection flush title={t('title')} empty={branches.length === 0 ? t('empty') : undefined}>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead kind="text">{t('name')}</TableHead>
            <TableHead kind="count">{t('rfqs')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {branches.map((branch) => (
            <TableRow key={branch.branchId}>
              <TableCell>{branch.branchName}</TableCell>
              <TableCell>{fmt.value(branch.rfqs)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </ReportSection>
  );
}
