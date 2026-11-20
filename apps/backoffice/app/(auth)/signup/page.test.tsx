import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const signupSwitch = vi.hoisted(() => ({ enabled: true }));

vi.mock('@/lib/config', () => ({
  get SIGNUP_ENABLED() {
    return signupSwitch.enabled;
  },
}));
vi.mock('next/navigation', () => ({
  redirect: vi.fn((to: string) => {
    throw new Error(`redirect:${to}`);
  }),
}));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));
vi.mock('@/lib/api/signup-invites', () => ({ signupInviteRefusal: vi.fn(async () => null) }));
vi.mock('@/app/(auth)/signup/_components/signup-form', () => ({
  SignupForm: ({ invite }: { invite?: string }) => <p>wizard:{invite ?? 'none'}</p>,
}));

const { getTranslations } = await import('next-intl/server');
const { redirect } = await import('next/navigation');
const { signupInviteRefusal } = await import('@/lib/api/signup-invites');
const { default: SignupPage } = await import('@/app/(auth)/signup/page');

function lookup(path: string): unknown {
  return path
    .split('.')
    .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
}

// The root reader the error resolver needs, over the real catalog.
function translator(namespace?: string) {
  const prefix = namespace ? `${namespace}.` : '';
  const read = (key: string) => String(lookup(prefix + key) ?? prefix + key);
  read.has = (key: string) => typeof lookup(prefix + key) === 'string';
  return read;
}

function open(params: Record<string, string> = {}) {
  return SignupPage({ searchParams: Promise.resolve(params) });
}

beforeEach(() => {
  vi.clearAllMocks();
  signupSwitch.enabled = true;
  vi.mocked(getTranslations).mockImplementation((async (namespace?: string) =>
    translator(namespace)) as never);
});

describe('SignupPage', () => {
  it('opens the wizard to anyone while signup is open', async () => {
    const view = render(await open());

    expect(view.getByText('wizard:none')).toBeTruthy();
    expect(signupInviteRefusal).not.toHaveBeenCalled();
  });

  // Closed, the plans and the request for Coti are the way in.
  it('sends a visitor without an invite to the plans while signup is closed', async () => {
    signupSwitch.enabled = false;

    await expect(open()).rejects.toThrow(`redirect:${ROUTES.landingSection('probar')}`);
    expect(redirect).toHaveBeenCalledWith('/#probar');
  });

  it('opens the wizard carrying a live invite while signup is closed', async () => {
    signupSwitch.enabled = false;
    const view = render(await open({ invite: 'raw-token' }));

    expect(signupInviteRefusal).toHaveBeenCalledWith('raw-token');
    expect(view.getByText('wizard:raw-token')).toBeTruthy();
  });

  it.each([
    ['SIGNUP_INVITE_EXPIRED', messages.errors.SIGNUP_INVITE_EXPIRED, '/#probar'],
    ['SIGNUP_INVITE_INVALID', messages.errors.SIGNUP_INVITE_INVALID, '/#probar'],
    ['SIGNUP_INVITE_USED', messages.errors.SIGNUP_INVITE_USED, ROUTES.login],
  ] as const)('says why a %s invite cannot open the wizard', async (code, sentence, next) => {
    signupSwitch.enabled = false;
    vi.mocked(signupInviteRefusal).mockResolvedValue(code);
    const view = render(await open({ invite: 'raw-token' }));

    expect(view.getByText(sentence)).toBeTruthy();
    expect(view.queryByText(/wizard:/)).toBeNull();
    expect(view.getByRole('link').getAttribute('href')).toBe(next);
  });
});
