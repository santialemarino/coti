'use server';

import { revalidatePath } from 'next/cache';

import type { OnboardingActionResult } from '@/app/(onboarding)/onboarding/actions';
import { ROUTES } from '@/config/routes';
import { apiRequest } from '@/lib/api/client';
import { errorCodeOf } from '@/lib/api/errors';

export async function setChecklistHidden(hidden: boolean): Promise<OnboardingActionResult> {
  try {
    await apiRequest({
      path: `/v1/onboarding/checklist/${hidden ? 'hide' : 'show'}`,
      method: 'POST',
      branchScoped: false,
    });
  } catch (error) {
    return { error: errorCodeOf(error) };
  }
  revalidatePath(ROUTES.home);
  revalidatePath(ROUTES.onboardingSettings);
  return { ok: true };
}
