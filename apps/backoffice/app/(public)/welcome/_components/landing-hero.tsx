import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { ProductPreview } from '@/app/(public)/welcome/_components/product-preview';
import { ROUTES } from '@/config/routes';

interface LandingHeroProps {
  signedIn: boolean;
}

// The first screen answers what Coti does and for whom, with the way in visible before any scroll.
export async function LandingHero({ signedIn }: LandingHeroProps) {
  const t = await getTranslations('landing.hero');

  return (
    <section aria-labelledby="hero-title" className="bg-linear-to-b from-accent to-body-background">
      <div className="grid w-full max-w-6xl mx-auto px-4 pt-12 pb-16 gap-12 sm:px-6 lg:grid-cols-[1.1fr_1fr] lg:items-center lg:pt-20 lg:pb-24">
        <div className="flex flex-col items-start gap-y-6 animate-rise-in">
          <p className="text-paragraph-sm-semibold text-primary">{t('slogan')}</p>
          <h1 id="hero-title" className="text-heading-2 text-foreground lg:text-heading-1">
            {t('title')}
          </h1>
          <p className="max-w-xl text-paragraph text-foreground-muted">{t('description')}</p>
          <div className="flex flex-wrap gap-3">
            {signedIn ? (
              <Button asChild size="lg">
                <Link href={ROUTES.home}>{t('goToOrders')}</Link>
              </Button>
            ) : (
              <>
                <Button asChild size="lg">
                  <Link href={ROUTES.signup}>{t('signup')}</Link>
                </Button>
                <Button asChild variant="outline" size="lg">
                  <Link href={ROUTES.login}>{t('login')}</Link>
                </Button>
              </>
            )}
          </div>
          <p className="text-paragraph-sm text-foreground-subtle">{t('note')}</p>
        </div>
        {/* A position in the same entrance, not a reusable delay. */}
        <ProductPreview className="animate-rise-in [animation-delay:120ms]" />
      </div>
    </section>
  );
}
