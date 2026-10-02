import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ChangeEmailForm } from '@/components/change-email-form';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('emailSettings');

export default async function EmailSettingsPage() {
  if (!(await getSession())) redirect(ROUTES.sessionEnded);
  const t = await getTranslations('auth.changeEmail');

  return (
    <main className="flex flex-col max-w-xl gap-y-8">
      <PageHeader title={t('title')} />
      <ChangeEmailForm />
    </main>
  );
}
