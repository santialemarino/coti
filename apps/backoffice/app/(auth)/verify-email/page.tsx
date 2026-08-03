import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { ConfirmEmailForm } from '@/app/(auth)/verify-email/_components/confirm-email-form';
import { ResendVerificationForm } from '@/app/(auth)/verify-email/_components/resend-verification-form';
import { ROUTES } from '@/config/routes';
import { generatePageMetadata } from '@/lib/utils/page';

// The route the API mails, so its shape is a contract: WEB_BACKOFFICE_URL plus
// /verify-email?token=…
const TOKEN_PARAM = 'token';

export const generateMetadata = () => generatePageMetadata('verifyEmail');

interface VerifyEmailPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function VerifyEmailPage({ searchParams }: VerifyEmailPageProps) {
  const t = await getTranslations('auth.verifyEmail');
  const params = await searchParams;
  const token = typeof params[TOKEN_PARAM] === 'string' ? params[TOKEN_PARAM] : '';

  if (!token) {
    return (
      <>
        <h1 className="text-3xl font-bold">{t('title')}</h1>
        <p className="text-sm text-destructive">{t('errors.invalidLink')}</p>
        <p className="text-sm text-muted-foreground">{t('resendHint')}</p>
        <ResendVerificationForm />
        <Button asChild variant="outline">
          <Link href={ROUTES.login}>{t('backToLogin')}</Link>
        </Button>
      </>
    );
  }

  return (
    <>
      <h1 className="text-3xl font-bold">{t('title')}</h1>
      <ConfirmEmailForm token={token} />
    </>
  );
}
