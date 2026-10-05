import { describe, expect, it, vi } from 'vitest';

import { settingsNavItems } from '@/app/(protected)/settings/_components/settings-nav-items';
import { ROUTES } from '@/config/routes';
import type { Branch } from '@/lib/api/branches';
import type { Onboarding } from '@/lib/api/onboarding';

const translate = (key: string) => key;
const MAILED = [{ id: 'b1', email: 'centro@corralon.test', isActive: true }] as Branch[];
const FINISHED = { status: 'COMPLETED', checklist: [] } as unknown as Onboarding;

function items(overrides: Partial<Parameters<typeof settingsNavItems>[0]> = {}) {
  return settingsNavItems({
    t: translate,
    tSetup: translate,
    onboarding: FINISHED,
    branches: MAILED,
    ...overrides,
  });
}

describe('settingsNavItems', () => {
  // The dot is the trail from the shell to the fix, and it must not cry wolf.
  it('marks Sucursales only while an open branch has no mailbox', () => {
    const branchesItem = (list: ReturnType<typeof items>) =>
      list.find((item) => item.href === ROUTES.branchSettings);

    expect(branchesItem(items())?.attention).toBeUndefined();
    const missing = [...MAILED, { id: 'b2', email: null, isActive: true }] as Branch[];
    expect(branchesItem(items({ branches: missing }))?.attention).toBe('attention');
  });

  it('starts with account and keeps email and password out of the section list', () => {
    const hrefs = items().map((item) => item.href);

    expect(hrefs[0]).toBe(ROUTES.accountSettings);
    expect(hrefs).not.toContain(ROUTES.emailSettings);
    expect(hrefs).not.toContain(ROUTES.changePassword);
  });

  it('lists the initial setup with its progress while a step is pending', () => {
    const t = vi.fn(translate);
    const onboarding = {
      status: 'COMPLETED',
      checklistHiddenAt: '2026-09-30T12:00:00Z',
      checklist: [
        { step: 'BRAND', done: true },
        { step: 'CATALOG_UPLOAD', done: false },
        { step: 'TEAM', done: true },
      ],
    } as unknown as Onboarding;

    const hrefs = items({ t, onboarding }).map((item) => item.href);

    expect(hrefs).toContain(ROUTES.onboardingSettings);
    expect(t).toHaveBeenCalledWith('nav.onboarding', { done: 2, total: 3 });
  });

  it('drops the initial setup once every step is done', () => {
    const onboarding = {
      status: 'DISMISSED',
      checklist: [{ step: 'BRAND', done: true }],
    } as unknown as Onboarding;

    expect(items({ onboarding }).map((item) => item.href)).not.toContain(ROUTES.onboardingSettings);
  });
});
