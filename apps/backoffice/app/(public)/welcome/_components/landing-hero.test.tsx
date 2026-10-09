import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('@/app/(public)/welcome/_components/product-preview', () => ({
  ProductPreview: () => null,
}));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { getTranslations } = await import('next-intl/server');
const { LandingHero } = await import('@/app/(public)/welcome/_components/landing-hero');

const copy = messages.landing.hero;

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

describe('LandingHero', () => {
  it('names what Coti does in the one top-level heading', async () => {
    const view = render(await LandingHero({ signedIn: false }));

    expect(view.getByRole('heading', { level: 1, name: copy.title })).toBeTruthy();
  });

  it('puts signup first and login second for a visitor', async () => {
    const view = render(await LandingHero({ signedIn: false }));
    const links = view.getAllByRole('link');

    expect(links.map((link) => link.getAttribute('href'))).toEqual([ROUTES.signup, ROUTES.login]);
  });

  it('sends a seller to the queue instead', async () => {
    const view = render(await LandingHero({ signedIn: true }));

    expect(view.getByRole('link', { name: copy.goToOrders }).getAttribute('href')).toBe(
      ROUTES.home,
    );
    expect(view.queryByRole('link', { name: copy.signup })).toBeNull();
  });
});
