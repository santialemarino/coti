import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Callout, InlineLink } from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { LoginForm } from '@/app/(auth)/login/_components/login-form';
import { LOCKED_REASON, NEXT_PARAM, REASON_PARAM, ROUTES, safeNextPath } from '@/config/routes';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('login');

interface LoginPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const t = await getTranslations('auth.login');
  const params = await searchParams;
  const next = safeNextPath(typeof params[NEXT_PARAM] === 'string' ? params[NEXT_PARAM] : null);
  const locked = params[REASON_PARAM] === LOCKED_REASON;

  return (
    <AuthCard
      title={t('title')}
      footer={
        <div className="flex flex-col items-center gap-y-2">
          <InlineLink asChild tone="muted">
            <Link href={ROUTES.forgotPassword}>{t('forgotPassword')}</Link>
          </InlineLink>
          <p className="text-paragraph-sm text-foreground-muted">
            {t('noAccount')}{' '}
            <InlineLink asChild>
              <Link href={ROUTES.signup}>{t('signup')}</Link>
            </InlineLink>
          </p>
        </div>
      }
    >
      <div className="flex flex-col gap-y-5">
        {locked ? <Callout tone="warning">{t('locked')}</Callout> : null}
        <LoginForm next={next} />
      </div>
    </AuthCard>
  );
}
