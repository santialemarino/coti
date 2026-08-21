import Link from 'next/link';
import { CircleCheckIcon, CircleXIcon, MailCheckIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, Hint, InlineLink, Separator, StatusScreen } from '@repo/ui/components';
import { ConfirmEmailForm } from '@/app/(auth)/verify-email/_components/confirm-email-form';
import { ResendVerificationForm } from '@/app/(auth)/verify-email/_components/resend-verification-form';
import { ChangeEmailForm } from '@/components/change-email-form';
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

  /*
   * With a token the session only picks a label, so it must not be able to take the confirm
   * button down with it: getSession rethrows anything that is not a refusal, and an API that is
   * merely unreachable would otherwise turn a live link into an error screen. Everything below
   * genuinely needs the answer, so there the throw stands.
   */
  if (token) {
    const owner = await getSession().catch(() => null);
    return <ConfirmEmailForm token={token} address={owner?.email} />;
  }

  const session = await getSession();

  /*
   * Nothing left to do, and saying so beats repeating that a mail is on its way — which is what
   * this screen said to anyone who came back to it after confirming.
   */
  if (session?.emailVerified) {
    return (
      <Card>
        <StatusScreen
          icon={CircleCheckIcon}
          tone="success"
          title={t('alreadyTitle')}
          description={t('already')}
        >
          <InlineLink asChild>
            <Link href={ROUTES.home}>{t('continue')}</Link>
          </InlineLink>
        </StatusScreen>
      </Card>
    );
  }

  /*
   * A session with an unconfirmed address means they just registered: signup opens one and sends
   * them here, so this is the notice that the mail is on its way rather than a broken link.
   * Without a session there is nothing to name and nothing to resend to but a typed address.
   */
  const registered = session !== null;

  return (
    <Card className="gap-y-6">
      <StatusScreen
        icon={registered ? MailCheckIcon : CircleXIcon}
        tone={registered ? 'info' : 'danger'}
        title={t('title')}
        description={
          registered
            ? t('sentTo', { email: session.email })
            : apiErrorMessage(tRoot, 'auth.verifyEmail', 'INVALID_LINK')
        }
      />
      <div className="flex flex-col px-6 gap-y-4">
        <Hint>{registered ? t('resend.hintKnown') : t('resend.hint')}</Hint>
        <ResendVerificationForm address={registered ? session.email : undefined} />
        {/*
         * The other half of this screen, and it needs the session: correcting a typo would
         * otherwise go through user administration, which stays closed until the address the
         * caller cannot read is confirmed.
         */}
        {registered && (
          <>
            <Separator />
            <p className="text-paragraph-sm-medium text-foreground">{t('change.title')}</p>
            <Hint>{t('change.hint')}</Hint>
            <ChangeEmailForm variant="outline" />
          </>
        )}
        <InlineLink asChild tone="muted" className="self-center">
          <Link href={registered ? ROUTES.home : ROUTES.login}>
            {registered ? t('continue') : t('backToLogin')}
          </Link>
        </InlineLink>
      </div>
    </Card>
  );
}
