import { CheckIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { LandingCta } from '@/app/(public)/_components/landing-cta';
import { LandingSection } from '@/app/(public)/_components/landing-section';
import { revealItem } from '@/app/(public)/_components/reveal-item';
import { EXTRA_QUOTE_PRICE, PLANS } from '@/lib/constants/plans';
import { getFormatters } from '@/lib/i18n/formatters-server';

interface LandingPricingProps {
  signedIn: boolean;
  signupEnabled: boolean;
}

/*
 * The plans and the way in, in one band: the header's «Probar Coti» lands here. A seller already
 * has an account, so they see the plans without the call to action.
 */
export async function LandingPricing({ signedIn, signupEnabled }: LandingPricingProps) {
  const fmt = await getFormatters();
  const t = await getTranslations('landing.pricing');
  const price = (amount: string) => fmt.currency(amount, 'ARS', { wholeUnits: true });

  return (
    <LandingSection
      id="probar"
      eyebrow={t('eyebrow')}
      title={t('title')}
      description={t('description')}
    >
      <div className="flex flex-col gap-y-6">
        <ul className="grid gap-4 md:grid-cols-3">
          {PLANS.map((plan, index) => {
            const features = [
              t(plan.extraBranch ? 'sharedQuotes' : 'quotes', {
                count: fmt.value(plan.quotesPerMonth),
              }),
              t('branches', { count: plan.branches }),
              ...(plan.extraBranch
                ? [
                    t('extraBranch', {
                      price: price(plan.extraBranch.monthlyPrice),
                      count: fmt.value(plan.extraBranch.quotesPerMonth),
                    }),
                  ]
                : []),
            ];
            return (
              <li
                key={plan.key}
                {...revealItem(
                  index + 1,
                  'flex flex-col p-6 gap-y-5 bg-card border border-border rounded-1.5xl shadow-e2',
                )}
              >
                <div className="flex flex-col gap-y-1">
                  <h3 className="text-heading-5 text-foreground">{t(`plans.${plan.key}.name`)}</h3>
                  <p className="text-paragraph-sm text-foreground-muted">
                    {t(`plans.${plan.key}.summary`)}
                  </p>
                </div>
                <p className="flex flex-wrap items-baseline gap-x-2">
                  <span className="text-heading-3 text-foreground tabular-nums">
                    {price(plan.monthlyPrice)}
                  </span>
                  <span className="text-paragraph-sm text-foreground-muted">{t('perMonth')}</span>
                </p>
                <ul className="flex flex-col gap-y-2">
                  {features.map((feature) => (
                    <li key={feature} className="flex gap-x-2">
                      <CheckIcon
                        aria-hidden="true"
                        className="shrink-0 size-4 mt-0.5 text-primary"
                      />
                      <span className="text-paragraph-sm text-foreground">{feature}</span>
                    </li>
                  ))}
                </ul>
              </li>
            );
          })}
        </ul>
        <p {...revealItem(PLANS.length + 1, 'text-paragraph-sm text-foreground-muted')}>
          {t('extraQuote', { price: price(EXTRA_QUOTE_PRICE) })} {t('onboarding')}
        </p>
      </div>
      {signedIn ? null : (
        <LandingCta signupEnabled={signupEnabled} revealIndex={PLANS.length + 2} />
      )}
    </LandingSection>
  );
}
