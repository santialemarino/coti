import { getTranslations } from 'next-intl/server';

import { ChangePasswordForm } from '@/app/(protected)/settings/password/_components/change-password-form';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('changePassword');

export default async function ChangePasswordPage() {
  const t = await getTranslations('auth.changePassword');

  return (
    <main className="flex flex-col px-6 py-10 gap-y-6">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-3xl font-bold">{t('title')}</h1>
        <p className="text-muted-foreground">{t('description')}</p>
      </header>
      <ChangePasswordForm />
    </main>
  );
}
