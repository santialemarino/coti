import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { ProductPreview } from '@/app/(public)/_components/product-preview';
import { revealItem } from '@/app/(public)/_components/reveal-item';
import { ROUTES } from '@/config/routes';

interface LandingHeroProps {
  signedIn: boolean;
}

// The first screen answers what Coti does and for whom, with the way in visible before any scroll. A
// seller already has the way back to the queue in the header, so the hero offers them nothing twice.
export async function LandingHero({ signedIn }: LandingHeroProps) {
  const t = await getTranslations('landing.hero');

  return (
    <section aria-labelledby="hero-title" className="bg-linear-to-b from-accent to-body-background">
      <div className="grid w-full max-w-6xl mx-auto px-4 pt-12 pb-16 gap-12 sm:px-6 lg:grid-cols-[1.1fr_1fr] lg:items-center lg:pt-20 lg:pb-24">
        <div className="flex flex-col items-start gap-y-6">
          <p {...revealItem(0, 'text-paragraph-sm-semibold text-primary')}>{t('slogan')}</p>
          <h1
            id="hero-title"
            {...revealItem(1, 'text-heading-2 text-foreground lg:text-heading-1')}
          >
            {t('title')}
          </h1>
          <p {...revealItem(2, 'max-w-xl text-paragraph text-foreground-muted')}>
            {t('description')}
          </p>
          {signedIn ? null : (
            <>
              <div {...revealItem(3, 'flex flex-wrap gap-3')}>
                <Button asChild size="lg">
                  <Link href={ROUTES.signup}>{t('signup')}</Link>
                </Button>
                <Button asChild variant="outline" size="lg">
                  <Link href={ROUTES.login}>{t('login')}</Link>
                </Button>
              </div>
              <p {...revealItem(4, 'text-paragraph-sm text-foreground-subtle')}>{t('note')}</p>
            </>
          )}
        </div>
        <div {...revealItem(2)}>
          <ProductPreview />
        </div>
      </div>
    </section>
  );
}
