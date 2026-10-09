import { ChartColumnIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { PageShell } from '@/app/(protected)/_components/page-shell';
import { SectionPlaceholder } from '@/app/(protected)/_components/section-placeholder';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('reports');

export default async function ReportsPage() {
  const t = await getTranslations('reports');
  const tCommon = await getTranslations('common');

  return (
    <PageShell>
      <SectionPlaceholder
        icon={ChartColumnIcon}
        title={t('placeholderTitle')}
        description={t('placeholderDescription')}
        backLabel={tCommon('actions.back')}
      />
    </PageShell>
  );
}
