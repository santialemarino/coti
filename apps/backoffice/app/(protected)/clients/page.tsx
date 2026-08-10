import { UsersIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { SectionPlaceholder } from '@/app/(protected)/_components/section-placeholder';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('clients');

export default async function ClientsPage() {
  const t = await getTranslations('clients');
  const tCommon = await getTranslations('common');

  return (
    <main className="flex flex-col px-6 py-10 gap-y-6">
      <SectionPlaceholder
        icon={UsersIcon}
        title={t('placeholderTitle')}
        description={t('placeholderDescription')}
        backLabel={tCommon('actions.back')}
      />
    </main>
  );
}
