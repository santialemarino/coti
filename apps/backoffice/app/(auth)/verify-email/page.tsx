import Link from 'next/link';
import { CircleXIcon, MailCheckIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, Hint, InlineLink, StatusScreen } from '@repo/ui/components';
import { ConfirmEmailForm } from '@/app/(auth)/verify-email/_components/confirm-email-form';
import { ResendVerificationForm } from '@/app/(auth)/verify-email/_components/resend-verification-form';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { apiErrorMessage } from '@/lib/i18n/api-error';
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
  const tRoot = await getTranslations();
  const params = await searchParams;
  const token = typeof params[TOKEN_PARAM] === 'string' ? params[TOKEN_PARAM] : '';

  if (!token) {
    /*
     * Signed in with no token means they just registered: signup opens a session and sends them
     * here, so this is the notice that the mail is on its way rather than a broken link. The
     * resend form is offered either way — a mail that never arrives looks the same from here.
     */
    const registered = (await getSession()) !== null;

    return (
      <Card className="gap-y-6">
        <StatusScreen
          icon={registered ? MailCheckIcon : CircleXIcon}
          tone={registered ? 'info' : 'danger'}
          title={t('title')}
          description={
            registered ? t('sent') : apiErrorMessage(tRoot, 'auth.verifyEmail', 'INVALID_LINK')
          }
        />
        <div className="flex flex-col px-6 gap-y-4">
          <Hint>{t('resend.hint')}</Hint>
          <ResendVerificationForm />
          <InlineLink asChild tone="muted" className="self-center">
            <Link href={registered ? ROUTES.home : ROUTES.login}>
              {registered ? t('continue') : t('backToLogin')}
            </Link>
          </InlineLink>
        </div>
      </Card>
    );
  }

  return <ConfirmEmailForm token={token} />;
}
