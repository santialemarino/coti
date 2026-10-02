import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('@/app/(auth)/login/_components/login-form', () => ({ LoginForm: vi.fn(() => null) }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { getTranslations } = await import('next-intl/server');
const { default: LoginPage } = await import('@/app/(auth)/login/page');

const copy = messages.auth.login;

// Resolves against the real catalog, so a renamed or missing key renders nothing and fails here.
function translator(namespace: string) {
  return (key: string) =>
    `${namespace}.${key}`
      .split('.')
      .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
}

async function renderPage(params: Record<string, string> = {}) {
  return render(await LoginPage({ searchParams: Promise.resolve(params) }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('LoginPage', () => {
  /*
   * A CTA is a prompt plus a short link, never one long link: the whole sentence inside the anchor
   * is what a screen reader announces as the link's name.
   */
  it('offers the signup CTA as a prompt with a short link', async () => {
    const view = await renderPage();
    const link = view.getByRole('link', { name: copy.signup });

    expect(link.textContent).toBe(copy.signup);
    expect(link.getAttribute('href')).toBe(ROUTES.signup);
    expect(link.closest('p')?.textContent).toBe(`${copy.noAccount} ${copy.signup}`);
  });

  it('offers password recovery as its own link', async () => {
    const view = await renderPage();
    const link = view.getByRole('link', { name: copy.forgotPassword });

    expect(link.getAttribute('href')).toBe(ROUTES.forgotPassword);
  });

  // A session ended by a lockout arrives here with the reason, and the screen says it.
  it('says the account is locked when the session ended for that', async () => {
    const view = await renderPage({ reason: 'locked' });

    expect(view.getByText(copy.locked)).toBeTruthy();
  });

  it('says nothing about a lock otherwise', async () => {
    const view = await renderPage({ reason: 'other' });

    expect(view.queryByText(copy.locked)).toBeNull();
  });
});
