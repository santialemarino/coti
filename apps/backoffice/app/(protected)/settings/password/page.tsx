import { getTranslations } from 'next-intl/server';

import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ChangePasswordForm } from '@/app/(protected)/settings/password/_components/change-password-form';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('changePassword');

export default async function ChangePasswordPage() {
  const t = await getTranslations('auth.changePassword');

  return (
    <main className="flex flex-col max-w-xl gap-y-8">
      <PageHeader title={t('title')} />
      <ChangePasswordForm />
    </main>
  );
}
