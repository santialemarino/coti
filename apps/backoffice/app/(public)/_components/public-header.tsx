import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { Brand } from '@/components/brand';
import { ROUTES } from '@/config/routes';

interface PublicHeaderProps {
  signedIn: boolean;
}

export async function PublicHeader({ signedIn }: PublicHeaderProps) {
  const t = await getTranslations('landing.header');
  const tCommon = await getTranslations('common');

  return (
    <header className="sticky top-0 z-40 border-b border-border bg-body-background/85 backdrop-blur">
      <div className="flex w-full max-w-6xl h-16 items-center justify-between mx-auto px-4 gap-x-4 sm:px-6">
        <Link
          href={ROUTES.landing}
          className="rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/45"
        >
          <Brand label={tCommon('appName')} />
        </Link>
        <nav aria-label={t('nav')} className="flex items-center gap-x-2">
          {signedIn ? (
            <Button asChild>
              <Link href={ROUTES.home}>{t('goToOrders')}</Link>
            </Button>
          ) : (
            <>
              <Button asChild variant="ghost">
                <Link href={ROUTES.login}>{t('login')}</Link>
              </Button>
              <Button asChild>
                <Link href={ROUTES.signup}>{t('signup')}</Link>
              </Button>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
