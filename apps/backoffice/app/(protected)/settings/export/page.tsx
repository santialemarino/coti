import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { DataExportForm } from '@/app/(protected)/settings/export/_components/data-export-form';
import { getAccount, showsInvoiceHistory } from '@/lib/api/account';
import { getBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('exportSettings');

export default async function ExportSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('dataExport');
  const [account, branches] = await Promise.all([getAccount(), getBranches()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <DataExportForm
        branches={branches.map(({ id, name, isActive }) => ({ id, name, isActive }))}
        offersInvoices={showsInvoiceHistory(account)}
      />
    </main>
  );
}
