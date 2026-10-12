import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { BranchTable } from '@/app/(protected)/settings/branches/_components/branch-table';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getAccountBranches } from '@/lib/api/branches';
import { getInvoicingSettings } from '@/lib/api/invoicing-settings';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('branchSettings');

export default async function BranchSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('branches');
  // Closed branches included, which is what lets one be reopened. The switcher reads the other
  // list, so it can never offer a branch the API would refuse.
  const [branches, setup, invoicing] = await Promise.all([
    getAccountBranches(),
    getARCASetup(),
    getInvoicingSettings(),
  ]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <BranchTable
        branches={branches}
        setup={setup}
        fiscalBranches={invoicing.branches}
        companyTaxId={invoicing.taxId}
      />
    </main>
  );
}
