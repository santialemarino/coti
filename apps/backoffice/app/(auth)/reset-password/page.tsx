import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { ResetPasswordForm } from '@/app/(auth)/reset-password/_components/reset-password-form';
import { ROUTES } from '@/config/routes';
import { generatePageMetadata } from '@/lib/utils/page';

// The route the API mails, so its shape is a contract: WEB_BACKOFFICE_URL plus
// /reset-password?token=…
const TOKEN_PARAM = 'token';

export const generateMetadata = () => generatePageMetadata('resetPassword');

interface ResetPasswordPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function ResetPasswordPage({ searchParams }: ResetPasswordPageProps) {
  const t = await getTranslations('auth.resetPassword');
  const params = await searchParams;
  const token = typeof params[TOKEN_PARAM] === 'string' ? params[TOKEN_PARAM] : '';

  if (!token) {
    return (
      <>
        <h1 className="text-3xl font-bold">{t('title')}</h1>
        <p className="text-sm text-muted-foreground">{t('errors.invalidLink')}</p>
        <Button asChild variant="outline">
          <Link href={ROUTES.forgotPassword}>{t('requestAnother')}</Link>
        </Button>
      </>
    );
  }

  return (
    <>
      <h1 className="text-3xl font-bold">{t('title')}</h1>
      <ResetPasswordForm token={token} />
    </>
  );
}
