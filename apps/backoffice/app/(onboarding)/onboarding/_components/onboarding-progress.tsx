'use client';

import { useTranslations } from 'next-intl';

import { ONBOARDING_STEPS } from '@/app/(onboarding)/onboarding/steps';
import type { OnboardingStepKey, OnboardingStepStatus } from '@/lib/api/onboarding';

interface OnboardingProgressProps {
  current: OnboardingStepKey;
  resolved: Partial<Record<OnboardingStepKey, OnboardingStepStatus>>;
}

export function OnboardingProgress({ current, resolved }: OnboardingProgressProps) {
  const t = useTranslations('onboarding.progress');
  const currentIndex = ONBOARDING_STEPS.indexOf(current as (typeof ONBOARDING_STEPS)[number]);
  const visibleIndex = Math.max(0, currentIndex);

  return (
    <div aria-label={t('label')}>
      <ol className="grid grid-cols-7 gap-x-2">
        {ONBOARDING_STEPS.map((step, index) => {
          const active = index === visibleIndex;
          const state = active ? 'current' : index < visibleIndex ? 'completed' : 'pending';
          const status = state === 'completed' ? resolved[step] : undefined;
          return (
            <li key={step}>
              <span
                aria-current={active ? 'step' : undefined}
                data-state={state}
                className={
                  state === 'current'
                    ? 'block h-1.5 w-full bg-primary border border-primary rounded-full transition-[background-color,border-color,box-shadow] duration-300 ease-in-out-soft'
                    : state === 'completed'
                      ? 'block h-1.5 w-full bg-primary-active border border-primary-active rounded-full transition-[background-color,border-color,box-shadow] duration-300 ease-in-out-soft'
                      : 'block h-1.5 w-full bg-background border border-border rounded-full shadow-e1 transition-[background-color,border-color,box-shadow] duration-300 ease-in-out-soft'
                }
              />
              <span className="sr-only">
                {t(`steps.${step}`)}:{' '}
                {state === 'current'
                  ? t('current')
                  : state === 'completed'
                    ? t(`statuses.${status ?? 'COMPLETED'}`)
                    : t('pending')}
              </span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
