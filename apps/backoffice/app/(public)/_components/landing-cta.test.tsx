import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));
vi.mock('@/app/(public)/_components/demo-link', () => ({ demoContactHref: vi.fn(() => null) }));

const { getTranslations } = await import('next-intl/server');
const { demoContactHref } = await import('@/app/(public)/_components/demo-link');
const { LandingCta } = await import('@/app/(public)/_components/landing-cta');

const copy = messages.landing.cta;
const DEMO_HREF = 'mailto:ventas@coti.test?subject=Coti';

// Resolves against the real catalog, so a renamed or missing key renders nothing and fails here.
function translator(namespace: string) {
  return (key: string) =>
    `${namespace}.${key}`
      .split('.')
      .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(demoContactHref).mockReturnValue(null);
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('LandingCta', () => {
  // Closed, asking for Coti is the way in, worded as the owner chose and with what happens next.
  it('offers only the request for Coti while signup is closed', async () => {
    vi.mocked(demoContactHref).mockReturnValue(DEMO_HREF);
    const view = render(await LandingCta({ signupEnabled: false, revealIndex: 0 }));

    expect(view.getAllByRole('link').map((link) => link.getAttribute('href'))).toEqual([DEMO_HREF]);
    expect(view.getByRole('link', { name: 'Quiero Coti en mi corralón' })).toBeTruthy();
    expect(view.getByText('Te contactamos y lo dejamos andando con tu catálogo.')).toBeTruthy();
    expect(view.queryByRole('link', { name: copy.action })).toBeNull();
  });

  it('offers signup beside the request for Coti while signup is open', async () => {
    vi.mocked(demoContactHref).mockReturnValue(DEMO_HREF);
    const view = render(await LandingCta({ signupEnabled: true, revealIndex: 0 }));

    expect(view.getAllByRole('link').map((link) => link.getAttribute('href'))).toEqual([
      ROUTES.signup,
      DEMO_HREF,
    ]);
    expect(view.getByRole('link', { name: copy.action })).toBeTruthy();
  });

  // The hero's demo button and this one are the same request, so they read the same.
  it('words the request for Coti as the hero does', () => {
    expect(copy.demo).toBe(messages.landing.hero.demo);
  });

  it('offers no action at all while signup is closed and no contact is set', async () => {
    const view = render(await LandingCta({ signupEnabled: false, revealIndex: 0 }));

    expect(view.queryAllByRole('link')).toHaveLength(0);
    expect(view.queryByText(copy.demoHint)).toBeNull();
  });
});
