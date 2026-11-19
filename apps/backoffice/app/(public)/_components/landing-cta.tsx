import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { demoContactHref } from '@/app/(public)/_components/demo-link';
import { Reveal } from '@/app/(public)/_components/reveal';
import { revealItem } from '@/app/(public)/_components/reveal-item';
import { ROUTES } from '@/config/routes';

export async function LandingCta() {
  const t = await getTranslations('landing.cta');
  const tDemo = await getTranslations('landing.demo');
  const demoHref = demoContactHref({ message: tDemo('message'), subject: tDemo('subject') });

  return (
    <section aria-labelledby="cta-title" className="py-16 lg:py-24">
      <Reveal className="flex flex-col w-full max-w-6xl mx-auto px-4 sm:px-6">
        <div
          {...revealItem(
            0,
            'flex flex-col items-start p-8 gap-y-5 bg-linear-to-br from-brand-800 to-brand-600 rounded-2xl text-primary-foreground sm:p-12 lg:flex-row lg:items-center lg:justify-between lg:gap-x-10',
          )}
        >
          <div className="flex flex-col max-w-xl gap-y-2">
            <h2 id="cta-title" className="text-heading-3">
              {t('title')}
            </h2>
            <p className="text-paragraph text-brand-50">{t('description')}</p>
          </div>
          <div className="flex flex-wrap shrink-0 gap-3">
            <Button asChild variant="secondary" size="lg">
              <Link href={ROUTES.signup}>{t('action')}</Link>
            </Button>
            {demoHref ? (
              <Button asChild variant="ghost" size="lg" className="text-primary-foreground">
                <a href={demoHref} target="_blank" rel="noopener noreferrer">
                  {t('demo')}
                </a>
              </Button>
            ) : null}
          </div>
        </div>
      </Reveal>
    </section>
  );
}
