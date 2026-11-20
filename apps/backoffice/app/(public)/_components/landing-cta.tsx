import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Button } from '@repo/ui/components';
import { demoContactHref } from '@/app/(public)/_components/demo-link';
import { revealItem } from '@/app/(public)/_components/reveal-item';
import { ROUTES } from '@/config/routes';

interface LandingCtaProps {
  signupEnabled: boolean;
  // Where the card falls in its section's entrance.
  revealIndex: number;
}

// The closing call to action, beside the plans. Asking for Coti is the main way in while signup is closed.
export async function LandingCta({ signupEnabled, revealIndex }: LandingCtaProps) {
  const t = await getTranslations('landing.cta');
  const tDemo = await getTranslations('landing.demo');
  const demoHref = demoContactHref({ message: tDemo('message'), subject: tDemo('subject') });

  return (
    <div
      {...revealItem(
        revealIndex,
        'flex flex-col items-start p-8 gap-y-5 bg-linear-to-br from-brand-800 to-brand-600 rounded-2xl text-primary-foreground sm:p-12 lg:flex-row lg:items-center lg:justify-between lg:gap-x-10',
      )}
    >
      <div className="flex flex-col max-w-xl gap-y-2">
        <h3 className="text-heading-3">{t('title')}</h3>
        <p className="text-paragraph text-brand-50">{t('description')}</p>
      </div>
      {signupEnabled || demoHref ? (
        <div className="flex flex-col shrink-0 items-start gap-y-2">
          <div className="flex flex-wrap gap-3">
            {signupEnabled ? (
              <Button asChild variant="secondary" size="lg">
                <Link href={ROUTES.signup}>{t('action')}</Link>
              </Button>
            ) : null}
            {demoHref ? (
              <Button
                asChild
                variant={signupEnabled ? 'ghost' : 'secondary'}
                size="lg"
                className={
                  signupEnabled
                    ? 'text-primary-foreground focus-visible:border-primary-foreground focus-visible:ring-primary-foreground/60'
                    : undefined
                }
              >
                <a href={demoHref} target="_blank" rel="noopener noreferrer">
                  {t('demo')}
                </a>
              </Button>
            ) : null}
          </div>
          {demoHref ? <p className="text-paragraph-sm text-brand-100">{t('demoHint')}</p> : null}
        </div>
      ) : null}
    </div>
  );
}
