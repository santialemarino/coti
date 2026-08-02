import { getTranslations } from 'next-intl/server';

import { ForgotPasswordForm } from '@/app/(auth)/forgot-password/_components/forgot-password-form';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('forgotPassword');

export default async function ForgotPasswordPage() {
  const t = await getTranslations('auth.forgotPassword');

  return (
    <>
      <h1 className="text-3xl font-bold">{t('title')}</h1>
      <p className="text-sm text-muted-foreground">{t('description')}</p>
      <ForgotPasswordForm />
    </>
  );
}
