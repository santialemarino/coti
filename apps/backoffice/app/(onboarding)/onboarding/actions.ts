'use server';

import { revalidatePath } from 'next/cache';

import {
  onboardingBrandSchema,
  type OnboardingBrandValues,
} from '@/app/(onboarding)/onboarding/form-schema';
import { branchSchema, type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { createUser } from '@/app/(protected)/settings/users/actions';
import type { UserValues } from '@/app/(protected)/settings/users/form-schema';
import { ROUTES } from '@/config/routes';
import { getAccount } from '@/lib/api/account';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf, type ApiErrorCode } from '@/lib/api/errors';
import type { OnboardingStepKey, OnboardingStepStatus } from '@/lib/api/onboarding';

export interface OnboardingActionResult {
  ok?: true;
  error?: ApiErrorCode;
}

export async function saveOnboardingStep(
  step: OnboardingStepKey,
  stepStatus: OnboardingStepStatus,
  currentStep: OnboardingStepKey,
): Promise<OnboardingActionResult> {
  return write('/v1/onboarding', 'PUT', {
    step,
    step_status: stepStatus,
    current_step: currentStep,
  });
}

export async function completeOnboarding(): Promise<OnboardingActionResult> {
  return write('/v1/onboarding/complete', 'POST');
}

export async function dismissOnboarding(): Promise<OnboardingActionResult> {
  return write('/v1/onboarding/dismiss', 'POST');
}

export async function resumeOnboarding(): Promise<OnboardingActionResult> {
  return write('/v1/onboarding/resume', 'POST');
}

export async function updateOnboardingBrand(
  values: OnboardingBrandValues,
): Promise<OnboardingActionResult> {
  const parsed = onboardingBrandSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };
  const account = await getAccount();
  return write('/v1/account', 'PUT', {
    name: account.name,
    legal_name: account.legalName ?? undefined,
    tax_id: account.taxId ?? undefined,
    brand_logo_url: account.brandLogoUrl ?? undefined,
    brand_color: parsed.data.brandColor ? `#${parsed.data.brandColor}` : undefined,
  });
}

export async function updateOnboardingBranch(
  branchId: string,
  values: BranchValues,
): Promise<OnboardingActionResult> {
  const parsed = branchSchema().safeParse(values);
  if (!parsed.success) return { error: 'INVALID_BODY' };
  return write(`/v1/branches/${branchId}`, 'PUT', {
    name: parsed.data.name,
    address: parsed.data.address || undefined,
    default_expiry_days: Number(parsed.data.defaultExpiryDays),
  });
}

export async function createOnboardingUser(values: UserValues): Promise<OnboardingActionResult> {
  return createUser(values);
}

async function write(
  path: string,
  method: 'POST' | 'PUT',
  body?: Record<string, unknown>,
): Promise<OnboardingActionResult> {
  try {
    await apiRequest({ path, method, body, branchScoped: false });
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.onboarding);
  return { ok: true };
}
