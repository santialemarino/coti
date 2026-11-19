import { getTranslations } from 'next-intl/server';

import { LandingSection } from '@/app/(public)/_components/landing-section';
import { revealItem } from '@/app/(public)/_components/reveal-item';

// The shape of the corralón Coti is built for, so a visitor can tell at a glance whether it is them.
const FACTS = ['sellers', 'quotes', 'catalog', 'branches'] as const;

export async function LandingFit() {
  const t = await getTranslations('landing.fit');

  return (
    <LandingSection
      id="fit"
      eyebrow={t('eyebrow')}
      title={t('title')}
      description={t('description')}
    >
      <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {FACTS.map((key, index) => (
          <div
            key={key}
            {...revealItem(
              index + 1,
              'flex flex-col p-6 gap-y-1 bg-card border border-border rounded-1.5xl shadow-e2',
            )}
          >
            <dt className="order-2 text-paragraph-sm text-foreground-muted">
              {t(`items.${key}.label`)}
            </dt>
            <dd className="order-1 text-heading-4 text-primary">{t(`items.${key}.value`)}</dd>
          </div>
        ))}
      </dl>
    </LandingSection>
  );
}
