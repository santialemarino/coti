import { getTranslations } from 'next-intl/server';

import { LoginForm } from '@/app/(auth)/login/_components/login-form';
import { NEXT_PARAM, safeNextPath } from '@/config/routes';
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
    <>
      <h1 className="text-3xl font-bold">{t('title')}</h1>
      <LoginForm next={next} />
    </>
  );
}
