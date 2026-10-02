import { StoreIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { EmptyState } from '@repo/ui/components';

// What a seller assigned to no branch sees instead of screens that would each fail on its own.
export function NoBranchScreen() {
  const t = useTranslations('home.noBranch');

  return (
    <main className="flex flex-1 items-center justify-center px-6 py-16 bg-body-background">
      <EmptyState icon={StoreIcon} title={t('title')} description={t('description')} />
    </main>
  );
}
