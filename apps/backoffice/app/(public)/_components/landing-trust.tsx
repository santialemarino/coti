import { CalculatorIcon, LockKeyholeIcon, UserCheckIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { LandingSection } from '@/app/(public)/_components/landing-section';

// The objections a supplier raises about letting a model near a quote, each answered by a rule the
// product enforces.
const POINTS = [
  { key: 'prices', icon: CalculatorIcon },
  { key: 'approval', icon: UserCheckIcon },
  { key: 'data', icon: LockKeyholeIcon },
] as const;

export async function LandingTrust() {
  const t = await getTranslations('landing.trust');

  return (
    <LandingSection
      id="trust"
      eyebrow={t('eyebrow')}
      title={t('title')}
      description={t('description')}
      className="bg-card"
    >
      <ul className="grid gap-6 md:grid-cols-3">
        {POINTS.map(({ key, icon: Icon }) => (
          <li key={key} className="flex items-start gap-x-4">
            <span className="grid size-10 shrink-0 place-items-center bg-accent rounded-full">
              <Icon aria-hidden="true" className="size-5 text-primary" />
            </span>
            <div className="flex flex-col gap-y-1">
              <h3 className="text-heading-6 text-foreground">{t(`items.${key}.title`)}</h3>
              <p className="text-paragraph-sm text-foreground-muted">
                {t(`items.${key}.description`)}
              </p>
            </div>
          </li>
        ))}
      </ul>
    </LandingSection>
  );
}
