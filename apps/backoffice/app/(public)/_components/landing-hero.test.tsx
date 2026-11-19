import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('@/app/(public)/_components/product-preview', () => ({
  ProductPreview: () => null,
}));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));
vi.mock('@/app/(public)/_components/demo-link', () => ({ demoContactHref: vi.fn(() => null) }));

const { getTranslations } = await import('next-intl/server');
const { demoContactHref } = await import('@/app/(public)/_components/demo-link');
const { LandingHero } = await import('@/app/(public)/_components/landing-hero');

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

  // Sales are direct demos, so once Coti's contact is configured the second way in is asking for one.
  it('offers a demo instead of login once a demo contact is configured', async () => {
    vi.mocked(demoContactHref).mockReturnValue('https://wa.me/5491155500199?text=hola');
    const view = render(await LandingHero({ signedIn: false }));
    const links = view.getAllByRole('link');

    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      ROUTES.signup,
      'https://wa.me/5491155500199?text=hola',
    ]);
    expect(view.getByRole('link', { name: copy.demo })).toBeTruthy();
  });

  // The header already carries "Ir a Pedidos"; the hero repeating it is noise.
  it('offers a seller no second way to the queue', async () => {
    const view = render(await LandingHero({ signedIn: true }));

    expect(view.queryAllByRole('link')).toHaveLength(0);
  });
});
