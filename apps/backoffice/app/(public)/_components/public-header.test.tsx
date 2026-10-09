import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('@/components/brand', () => ({ Brand: () => null }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { getTranslations } = await import('next-intl/server');
const { PublicHeader } = await import('@/app/(public)/_components/public-header');

const copy = messages.landing.header;

// Resolves against the real catalog, so a renamed or missing key renders nothing and fails here.
function translator(namespace: string) {
  return (key: string) =>
    `${namespace}.${key}`
      .split('.')
      .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('PublicHeader', () => {
  it('offers a visitor the way in: log in or create an account', async () => {
    const view = render(await PublicHeader({ signedIn: false }));

    expect(view.getByRole('link', { name: copy.login }).getAttribute('href')).toBe(ROUTES.login);
    expect(view.getByRole('link', { name: copy.signup }).getAttribute('href')).toBe(ROUTES.signup);
    expect(view.queryByRole('link', { name: copy.goToOrders })).toBeNull();
  });

  // A login screen is a dead end for someone already past it.
  it('offers a seller the way back to the queue instead', async () => {
    const view = render(await PublicHeader({ signedIn: true }));

    expect(view.getByRole('link', { name: copy.goToOrders }).getAttribute('href')).toBe(
      ROUTES.home,
    );
    expect(view.queryByRole('link', { name: copy.login })).toBeNull();
    expect(view.queryByRole('link', { name: copy.signup })).toBeNull();
  });
});
