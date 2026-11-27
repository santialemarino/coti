import 'server-only';

import { cache } from 'react';

import { apiRequest } from '@/lib/api/client';

// --- Raw types (API JSON shape, snake_case) ---

interface OnboardingRaw {
  flow_version: number;
  status: OnboardingStatus;
  current_step: OnboardingStepKey;
  steps: Partial<Record<OnboardingStepKey, OnboardingStepStatus>>;
  checklist: { step: OnboardingChecklistStep; done: boolean }[];
  checklist_hidden_at: string | null;
  completed_at: string | null;
}

interface ReadinessRaw {
  account_ready: boolean;
  branches: {
    id: string;
    name: string;
    products: number;
    missing_prices: number;
    has_staff: boolean;
    has_channel: boolean;
  }[];
}

export interface OperationalReadiness {
  accountReady: boolean;
  branches: {
    id: string;
    name: string;
    products: number;
    missingPrices: number;
    hasStaff: boolean;
    hasChannel: boolean;
  }[];
}

// --- Frontend types (camelCase) ---

export type OnboardingStatus = 'IN_PROGRESS' | 'COMPLETED' | 'DISMISSED';
export type OnboardingStepStatus = 'COMPLETED' | 'SKIPPED';
export type OnboardingStepKey =
  | 'WELCOME'
  | 'BRAND'
  | 'PORTAL'
  | 'FIRST_BRANCH'
  | 'CATALOG_UPLOAD'
  | 'CATALOG_REVIEW'
  | 'TEAM'
  | 'COMPLETE';
export type OnboardingChecklistStep = Extract<
  OnboardingStepKey,
  'BRAND' | 'PORTAL' | 'CATALOG_UPLOAD' | 'TEAM'
>;

export interface OnboardingChecklistItem {
  step: OnboardingChecklistStep;
  done: boolean;
}

export interface Onboarding {
  flowVersion: number;
  status: OnboardingStatus;
  currentStep: OnboardingStepKey;
  steps: Partial<Record<OnboardingStepKey, OnboardingStepStatus>>;
  checklist: OnboardingChecklistItem[];
  checklistHiddenAt: string | null;
  completedAt: string | null;
}

// --- Mappers ---

function mapOnboarding(raw: OnboardingRaw): Onboarding {
  return {
    flowVersion: raw.flow_version,
    status: raw.status,
    currentStep: raw.current_step,
    steps: raw.steps,
    checklist: raw.checklist,
    checklistHiddenAt: raw.checklist_hidden_at,
    completedAt: raw.completed_at,
  };
}

// --- API functions ---

export const getOnboarding = cache(async (): Promise<Onboarding> => {
  return mapOnboarding(
    await apiRequest<OnboardingRaw>({ path: '/v1/onboarding', branchScoped: false }),
  );
});

export async function getOperationalReadiness(): Promise<OperationalReadiness> {
  const raw = await apiRequest<ReadinessRaw>({ path: '/v1/onboarding/readiness' });
  return {
    accountReady: raw.account_ready,
    branches: raw.branches.map((branch) => ({
      id: branch.id,
      name: branch.name,
      products: branch.products,
      missingPrices: branch.missing_prices,
      hasStaff: branch.has_staff,
      hasChannel: branch.has_channel,
    })),
  };
}
