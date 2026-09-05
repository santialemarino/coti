import { ChartColumnIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { SectionPlaceholder } from '@/app/(protected)/_components/section-placeholder';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('reports');

export default async function ReportsPage() {
  const t = await getTranslations('reports');
  const tCommon = await getTranslations('common');

  return (
    <main className="flex flex-col px-6 py-10 gap-y-6">
      <SectionPlaceholder
        icon={ChartColumnIcon}
        title={t('placeholderTitle')}
        description={t('placeholderDescription')}
        backLabel={tCommon('actions.back')}
      />
    </main>
  );
}
