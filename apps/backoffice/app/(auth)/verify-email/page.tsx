import Link from 'next/link';
import { CircleXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, Hint, InlineLink, StatusScreen } from '@repo/ui/components';
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

  // Arriving with no token is the same dead end as a spent one: explain, then offer a new link.
  if (!token) {
    return (
      <Card className="gap-y-6">
        <StatusScreen
          icon={CircleXIcon}
          tone="danger"
          title={t('title')}
          description={t('errors.invalidLink')}
        />
        <div className="flex flex-col px-6 gap-y-4">
          <Hint>{t('resendHint')}</Hint>
          <ResendVerificationForm />
          <InlineLink asChild tone="muted" className="self-center">
            <Link href={ROUTES.login}>{t('backToLogin')}</Link>
          </InlineLink>
        </div>
      </Card>
    );
  }

  return <ConfirmEmailForm token={token} />;
}
