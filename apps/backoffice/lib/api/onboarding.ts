import 'server-only';

import { cache } from 'react';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

interface OnboardingRaw {
  flow_version: number;
  status: OnboardingStatus;
  current_step: OnboardingStepKey;
  steps: Partial<Record<OnboardingStepKey, OnboardingStepStatus>>;
  completed_at: string | null;
}

// --- Frontend types (camelCase) ---

export type OnboardingStatus = 'IN_PROGRESS' | 'COMPLETED' | 'DISMISSED';
export type OnboardingStepStatus = 'COMPLETED' | 'SKIPPED';
export type OnboardingStepKey =
  | 'WELCOME'
  | 'BRAND'
  | 'FIRST_BRANCH'
  | 'CATALOG_UPLOAD'
  | 'CATALOG_REVIEW'
  | 'TEAM'
  | 'COMPLETE';

export interface Onboarding {
  flowVersion: number;
  status: OnboardingStatus;
  currentStep: OnboardingStepKey;
  steps: Partial<Record<OnboardingStepKey, OnboardingStepStatus>>;
  completedAt: string | null;
}

// --- Mappers ---

function mapOnboarding(raw: OnboardingRaw): Onboarding {
  return {
    flowVersion: raw.flow_version,
    status: raw.status,
    currentStep: raw.current_step,
    steps: raw.steps,
    completedAt: raw.completed_at,
  };
}

// --- API functions ---

export const getOnboarding = cache(async (): Promise<Onboarding> => {
  return mapOnboarding(
    await apiRequest<OnboardingRaw>({ path: '/v1/onboarding', branchScoped: false }),
  );
});
