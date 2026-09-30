import { describe, expect, it } from 'vitest';

import type { Onboarding } from '@/lib/api/onboarding';
import { hasPendingChecklist, showsChecklistOnHome } from '@/lib/utils/onboarding-checklist';

function onboarding(overrides: Partial<Onboarding> = {}): Onboarding {
  return {
    flowVersion: 1,
    status: 'DISMISSED',
    currentStep: 'BRAND',
    steps: {},
    checklist: [
      { step: 'BRAND', done: true },
      { step: 'CATALOG_UPLOAD', done: false },
      { step: 'TEAM', done: true },
    ],
    checklistHiddenAt: null,
    completedAt: null,
    ...overrides,
  };
}

const ALL_DONE = onboarding().checklist.map((item) => ({ ...item, done: true }));

describe('hasPendingChecklist', () => {
  it('is pending once the wizard is closed and a step is left', () => {
    expect(hasPendingChecklist(onboarding())).toBe(true);
    expect(hasPendingChecklist(onboarding({ status: 'COMPLETED' }))).toBe(true);
  });

  it('stays out of the way while the wizard is open', () => {
    expect(hasPendingChecklist(onboarding({ status: 'IN_PROGRESS' }))).toBe(false);
  });

  it('has nothing to say once every step is done', () => {
    expect(hasPendingChecklist(onboarding({ checklist: ALL_DONE }))).toBe(false);
  });
});

describe('showsChecklistOnHome', () => {
  it('shows the card while something is pending and it was not hidden', () => {
    expect(showsChecklistOnHome(onboarding())).toBe(true);
  });

  it('keeps a hidden card off the home screen', () => {
    expect(showsChecklistOnHome(onboarding({ checklistHiddenAt: '2026-09-30T12:00:00Z' }))).toBe(
      false,
    );
  });

  it('never shows a card with nothing pending', () => {
    expect(showsChecklistOnHome(onboarding({ checklist: ALL_DONE }))).toBe(false);
  });
});
