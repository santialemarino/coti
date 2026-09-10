import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { ChangeEmailForm } from '@/components/change-email-form';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('emailSettings');

export default async function EmailSettingsPage() {
  if (!(await getSession())) redirect(ROUTES.sessionEnded);
  const t = await getTranslations('auth.changeEmail');

  return (
    <main className="flex flex-col max-w-xl gap-y-6">
      <h1 className="text-heading-2">{t('title')}</h1>
      <ChangeEmailForm />
    </main>
  );
}
