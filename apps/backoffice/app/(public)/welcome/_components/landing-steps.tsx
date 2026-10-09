import { getTranslations } from 'next-intl/server';

import { LandingSection } from '@/app/(public)/welcome/_components/landing-section';

// Keyed by id, not position, so reordering the copy can never renumber a step under another's text.
const STEPS = ['arrives', 'drafts', 'review', 'answer'] as const;

export async function LandingSteps() {
  const t = await getTranslations('landing.steps');

  return (
    <LandingSection
      id="steps"
      eyebrow={t('eyebrow')}
      title={t('title')}
      description={t('description')}
      className="bg-card"
    >
      <ol className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
        {STEPS.map((step, index) => (
          <li key={step} className="flex flex-col gap-y-3">
            <span className="grid size-10 place-items-center bg-primary rounded-full text-paragraph-sm-semibold text-primary-foreground tabular-nums">
              {index + 1}
            </span>
            <h3 className="text-heading-6 text-foreground">{t(`items.${step}.title`)}</h3>
            <p className="text-paragraph-sm text-foreground-muted">
              {t(`items.${step}.description`)}
            </p>
          </li>
        ))}
      </ol>
    </LandingSection>
  );
}
