import { ClockIcon, ListChecksIcon, TrendingUpIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { LandingSection } from '@/app/(public)/_components/landing-section';

// Three and no more: past three, a list of benefits stops helping anyone decide.
const BENEFITS = [
  { key: 'speed', icon: ClockIcon },
  { key: 'accuracy', icon: ListChecksIcon },
  { key: 'ticket', icon: TrendingUpIcon },
] as const;

export async function LandingBenefits() {
  const t = await getTranslations('landing.benefits');

  return (
    <LandingSection id="benefits" eyebrow={t('eyebrow')} title={t('title')}>
      <ul className="grid gap-4 md:grid-cols-3">
        {BENEFITS.map(({ key, icon: Icon }) => (
          <li
            key={key}
            className="flex flex-col p-6 gap-y-4 bg-card border border-border rounded-1.5xl shadow-e2"
          >
            <span className="grid size-11 place-items-center bg-accent rounded-xl">
              <Icon aria-hidden="true" className="size-5 text-primary" />
            </span>
            <div className="flex flex-col gap-y-2">
              <h3 className="text-heading-5 text-foreground">{t(`items.${key}.title`)}</h3>
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
