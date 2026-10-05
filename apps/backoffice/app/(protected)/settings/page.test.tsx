import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';

class RedirectError extends Error {
  constructor(readonly path: string) {
    super(`NEXT_REDIRECT ${path}`);
  }
}

vi.mock('next/navigation', () => ({
  redirect: vi.fn((path: string) => {
    throw new RedirectError(path);
  }),
}));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn(async () => (key: string) => key) }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));

const { getSession } = await import('@/lib/auth/session');
const { default: SettingsIndexPage } = await import('@/app/(protected)/settings/page');

beforeEach(() => vi.clearAllMocks());

describe('SettingsIndexPage', () => {
  it('gives an administrator the empty pane beside the sections', async () => {
    vi.mocked(getSession).mockResolvedValue({ role: 'ADMIN' } as never);

    render(await SettingsIndexPage());

    expect(screen.getByText('indexHint')).toBeTruthy();
  });

  it('sends a seller, who has no sections, to their one settings page', async () => {
    vi.mocked(getSession).mockResolvedValue({ role: 'SELLER' } as never);

    await expect(SettingsIndexPage()).rejects.toMatchObject({ path: ROUTES.changePassword });
  });
});
