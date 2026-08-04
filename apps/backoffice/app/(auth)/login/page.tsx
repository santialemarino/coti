import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { InlineLink } from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { LoginForm } from '@/app/(auth)/login/_components/login-form';
import { NEXT_PARAM, ROUTES, safeNextPath } from '@/config/routes';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('login');

interface LoginPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const t = await getTranslations('auth.login');
  const params = await searchParams;
  const next = safeNextPath(typeof params[NEXT_PARAM] === 'string' ? params[NEXT_PARAM] : null);

  return (
    <AuthCard
      title={t('title')}
      footer={
        <InlineLink asChild tone="muted">
          <Link href={ROUTES.forgotPassword}>{t('forgotPassword')}</Link>
        </InlineLink>
      }
    >
      <LoginForm next={next} />
    </AuthCard>
  );
}
