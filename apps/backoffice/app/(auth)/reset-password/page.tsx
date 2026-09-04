import Link from 'next/link';
import { CircleXIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Card, InlineLink, StatusScreen } from '@repo/ui/components';
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

  // A missing token is the same dead end as an expired one, so it gets the same screen.
  if (!token) {
    return (
      <Card>
        <StatusScreen
          icon={CircleXIcon}
          tone="danger"
          title={t('title')}
          description={t('errors.invalidLink')}
        >
          <InlineLink asChild>
            <Link href={ROUTES.forgotPassword}>{t('requestAnother')}</Link>
          </InlineLink>
        </StatusScreen>
      </Card>
    );
  }

  return <ResetPasswordForm token={token} />;
}
