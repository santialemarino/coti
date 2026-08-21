import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { ChangeEmailForm } from '@/components/change-email-form';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('emailSettings');

export default async function EmailSettingsPage() {
  const session = await getSession();
  if (!session) redirect(ROUTES.sessionEnded);
  const t = await getTranslations('auth.changeEmail');

  return (
    <main className="flex flex-col max-w-xl gap-y-6">
      <header className="flex flex-col gap-y-2">
        <h1 className="text-heading-2">{t('title')}</h1>
        <p className="text-paragraph text-foreground-muted">
          {t('description', { email: session.email })}
        </p>
      </header>
      <ChangeEmailForm />
    </main>
  );
}
