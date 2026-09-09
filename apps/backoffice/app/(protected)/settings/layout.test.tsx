import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { SettingsNavItem } from '@/app/(protected)/settings/_components/settings-nav';
import { ROUTES } from '@/config/routes';

vi.mock('@/app/(protected)/settings/_components/settings-nav', () => ({
  SettingsNav: vi.fn(() => null),
}));
vi.mock('@/lib/api/onboarding', () => ({ getOnboarding: vi.fn() }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { SettingsNav } = await import('@/app/(protected)/settings/_components/settings-nav');
const { getOnboarding } = await import('@/lib/api/onboarding');
const { getSession } = await import('@/lib/auth/session');
const { getTranslations } = await import('next-intl/server');
const { default: SettingsLayout } = await import('@/app/(protected)/settings/layout');

function renderedItems(): SettingsNavItem[] | undefined {
  return vi.mocked(SettingsNav).mock.calls[0]?.[0].items;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getSession).mockResolvedValue({
    userId: 'u1',
    accountId: 'a1',
    name: 'Ana Gómez',
    email: 'ana@corralon.test',
    emailVerified: true,
    role: 'ADMIN',
  });
  vi.mocked(getOnboarding).mockResolvedValue({ status: 'COMPLETED' } as never);
  vi.mocked(getTranslations).mockResolvedValue(((key: string) => key) as never);
});

describe('SettingsLayout', () => {
  it('starts with account and keeps email and password out of the section list', async () => {
    render(await SettingsLayout({ children: null }));

    const hrefs = renderedItems()?.map((item) => item.href);
    expect(hrefs?.[0]).toBe(ROUTES.accountSettings);
    expect(hrefs).not.toContain(ROUTES.emailSettings);
    expect(hrefs).not.toContain(ROUTES.changePassword);
  });
});
