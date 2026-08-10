import { getTranslations } from 'next-intl/server';

import { AccountForm } from '@/app/(protected)/settings/account/_components/account-form';
import { getAccount } from '@/lib/api/account';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('accountSettings');

export default async function AccountSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('account');
  const account = await getAccount();

  return (
    <main className="flex flex-col gap-y-8">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{t('title')}</h1>
        <p className="text-paragraph text-foreground-muted">{t('description')}</p>
      </header>
      <AccountForm account={account} />
    </main>
  );
}
