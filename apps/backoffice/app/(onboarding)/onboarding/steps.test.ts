import { describe, expect, it } from 'vitest';

import { ONBOARDING_STEPS, resumeStep } from '@/app/(onboarding)/onboarding/steps';

describe('onboarding step registry', () => {
  it('keeps every visible screen in one unique ordered registry', () => {
    expect(ONBOARDING_STEPS).toEqual([
      'WELCOME',
      'BRAND',
      'FIRST_BRANCH',
      'CATALOG_UPLOAD',
      'CATALOG_REVIEW',
      'TEAM',
      'COMPLETE',
    ]);
    expect(new Set(ONBOARDING_STEPS).size).toBe(ONBOARDING_STEPS.length);
  });

  it('resumes a transient catalog review at the upload that can recreate it', () => {
    expect(resumeStep('CATALOG_REVIEW')).toBe('CATALOG_UPLOAD');
    expect(resumeStep('COMPLETE')).toBe('TEAM');
    expect(resumeStep('TEAM')).toBe('TEAM');
  });
});
