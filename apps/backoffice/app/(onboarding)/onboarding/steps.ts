import type { OnboardingStepKey } from '@/lib/api/onboarding';

export const ONBOARDING_STEPS = [
  'WELCOME',
  'BRAND',
  'FIRST_BRANCH',
  'CATALOG_UPLOAD',
  'CATALOG_REVIEW',
  'TEAM',
  'COMPLETE',
] as const satisfies readonly OnboardingStepKey[];

export type ProgressStepKey = (typeof ONBOARDING_STEPS)[number];

export function resumeStep(step: OnboardingStepKey): OnboardingStepKey {
  if (step === 'CATALOG_REVIEW') return 'CATALOG_UPLOAD';
  if (step === 'COMPLETE') return 'TEAM';
  return step;
}
