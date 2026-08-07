import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('@/components/branded-screen', () => ({
  BrandedScreen: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
/*
 * Only the cookie read is stubbed, which is what pins the page off `getSession`: reaching for it
 * would resolve to undefined here and every test below would die. That matters — `getSession`
 * rethrows anything that is not a 401/403, so an unreachable API would replace "this page does not
 * exist" with an error screen over an outage that has nothing to do with it.
 */
vi.mock('@/lib/auth/session', () => ({ getAccessToken: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { getAccessToken } = await import('@/lib/auth/session');
const { getTranslations } = await import('next-intl/server');
const { default: NotFound } = await import('@/app/not-found');

const copy = messages.notFound;

// Resolves against the real catalog, so a renamed or missing key renders nothing and fails here.
function translator(namespace: string) {
  return (key: string) =>
    `${namespace}.${key}`
      .split('.')
      .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
}

async function renderPage() {
  return render(await NotFound());
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('NotFound', () => {
  /*
   * The CTA is the whole point of reading the session here: sending a signed-in caller to the login
   * screen is a dead end, and offering the home page to someone with no session bounces them
   * straight back to login with a `next` they never asked for.
   */
  it('offers a signed-in caller the way home', async () => {
    vi.mocked(getAccessToken).mockResolvedValue('a-token');
    const view = await renderPage();

    const link = view.getByRole('link', { name: copy.goHome });
    expect(link.getAttribute('href')).toBe(ROUTES.home);
    expect(view.queryByRole('link', { name: copy.goToLogin })).toBeNull();
  });

  it('offers a signed-out caller the login screen', async () => {
    vi.mocked(getAccessToken).mockResolvedValue(undefined);
    const view = await renderPage();

    const link = view.getByRole('link', { name: copy.goToLogin });
    expect(link.getAttribute('href')).toBe(ROUTES.login);
    expect(view.queryByRole('link', { name: copy.goHome })).toBeNull();
  });

  it('says what happened in Spanish, not in Next.js English', async () => {
    vi.mocked(getAccessToken).mockResolvedValue(undefined);
    const view = await renderPage();

    expect(view.getByText(copy.title)).toBeTruthy();
    expect(view.getByText(copy.description)).toBeTruthy();
  });
});
