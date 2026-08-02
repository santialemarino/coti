import { getTranslations } from 'next-intl/server';

import { PriceImport } from '@/app/(protected)/settings/prices/_components/price-import';
import { getBranches } from '@/lib/api/branches';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('priceSettings');

export default async function PriceSettingsPage() {
  const t = await getTranslations('priceImport');
  const branches = await getBranches();
  return (
    <main className="flex min-h-screen flex-col px-6 py-10 gap-y-8 bg-background lg:px-12">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-3xl font-bold">{t('title')}</h1>
        <p className="text-muted-foreground">{t('description')}</p>
      </header>
      <PriceImport branches={branches} />
    </main>
  );
}
