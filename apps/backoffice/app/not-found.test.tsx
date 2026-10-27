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
vi.mock('@/lib/auth/session', () => ({ isAuthenticated: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { isAuthenticated } = await import('@/lib/auth/session');
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
  // A seller is sent back to work; the public site is the second way out, not the first.
  it('offers a signed-in caller the queue and the public site', async () => {
    vi.mocked(isAuthenticated).mockResolvedValue(true);
    const view = await renderPage();

    expect(view.getByRole('link', { name: copy.goToOrders }).getAttribute('href')).toBe(
      ROUTES.home,
    );
    expect(view.getByRole('link', { name: copy.goToSite }).getAttribute('href')).toBe(
      ROUTES.landing,
    );
  });

  // Someone with no session reaches this directly, and the root is where the landing lives.
  it('offers a signed-out caller the way back to the landing, not a login screen', async () => {
    vi.mocked(isAuthenticated).mockResolvedValue(false);
    const view = await renderPage();

    expect(view.getByRole('link', { name: copy.backHome }).getAttribute('href')).toBe(
      ROUTES.landing,
    );
    expect(view.getAllByRole('link')).toHaveLength(1);
  });

  it('says what happened in Spanish, not in Next.js English', async () => {
    vi.mocked(isAuthenticated).mockResolvedValue(false);
    const view = await renderPage();

    expect(view.getByText(copy.title)).toBeTruthy();
    expect(view.getByText(copy.description)).toBeTruthy();
  });
});
