import { getTranslations } from 'next-intl/server';

import { Brand } from '@/components/brand';

export async function CustomerFooter() {
  const t = await getTranslations('customerOrders');
  return (
    <footer className="flex items-center justify-center px-4 py-6 gap-x-2 text-paragraph-xs text-foreground-subtle">
      <span>{t('poweredBy')}</span>
      <Brand size="sm" label="Coti" />
    </footer>
  );
}
