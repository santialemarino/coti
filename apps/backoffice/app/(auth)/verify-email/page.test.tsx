import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/app/(auth)/verify-email/_components/confirm-email-form', () => ({
  ConfirmEmailForm: vi.fn(() => null),
}));
vi.mock('@/app/(auth)/verify-email/_components/resend-verification-form', () => ({
  ResendVerificationForm: vi.fn(() => null),
}));
vi.mock('@/components/change-email-form', () => ({ ChangeEmailForm: vi.fn(() => null) }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));
vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { ConfirmEmailForm } =
  await import('@/app/(auth)/verify-email/_components/confirm-email-form');
const { ResendVerificationForm } =
  await import('@/app/(auth)/verify-email/_components/resend-verification-form');
const { ChangeEmailForm } = await import('@/components/change-email-form');
const { getSession } = await import('@/lib/auth/session');
const { getTranslations } = await import('next-intl/server');
const { default: VerifyEmailPage } = await import('@/app/(auth)/verify-email/page');

const EMAIL = 'ana@corralonsanmartin.test';

function session(emailVerified: boolean) {
  return {
    userId: 'u1',
    accountId: 'a1',
    name: 'Ana Gómez',
    email: EMAIL,
    emailVerified,
    role: 'ADMIN',
  };
}

async function renderPage(params: Record<string, string | string[] | undefined>) {
  return render(await VerifyEmailPage({ searchParams: Promise.resolve(params) }));
}

/*
 * Keys rather than Spanish, with any interpolated value appended — the assertions are about which
 * screen rendered and what reached it, not about wording that a copy edit may legitimately change.
 * `has` answers false so an error code resolves through the shared catalog, as it does in the app.
 */
const translator = Object.assign(
  (key: string, values?: Record<string, unknown>) =>
    values ? `${key} ${Object.values(values).join(' ')}` : key,
  { has: () => false },
);

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockResolvedValue(translator as never);
});

/*
 * Four ways to arrive here and they are not the same screen. The branching is the whole point of
 * the page, so each case is pinned by what it hands its children rather than by its wording.
 */
describe('with a token', () => {
  it('hands the token to the confirm form, and says there is a session', async () => {
    vi.mocked(getSession).mockResolvedValue(session(false));

    await renderPage({ token: 'abc123' });

    expect(ConfirmEmailForm).toHaveBeenCalledWith(
      expect.objectContaining({ token: 'abc123', address: EMAIL }),
      undefined,
    );
  });

  // Opening the mailed link in the browser the mail client owns is the common case, and it
  // usually holds no session — the success CTA has to lead somewhere reachable.
  it('names no address when there is no session', async () => {
    vi.mocked(getSession).mockResolvedValue(null);

    await renderPage({ token: 'abc123' });

    expect(ConfirmEmailForm).toHaveBeenCalledWith(
      expect.objectContaining({ address: undefined }),
      undefined,
    );
  });

  /*
   * getSession rethrows anything that is not a refusal, so an unreachable API used to take the
   * confirm button down with it — on the one path where the token, not the session, is the point.
   */
  it('still renders the confirm button when the session cannot be resolved', async () => {
    vi.mocked(getSession).mockRejectedValue(new Error('UNREACHABLE'));

    await renderPage({ token: 'abc123' });

    expect(ConfirmEmailForm).toHaveBeenCalledWith(
      expect.objectContaining({ token: 'abc123', address: undefined }),
      undefined,
    );
  });

  // Two of the same parameter is not a token; it must not be handed on as one.
  it('treats a repeated token parameter as no token', async () => {
    vi.mocked(getSession).mockResolvedValue(null);

    await renderPage({ token: ['a', 'b'] });

    expect(ConfirmEmailForm).not.toHaveBeenCalled();
  });
});

describe('with no token', () => {
  it('resends to the caller address when the session says who they are', async () => {
    vi.mocked(getSession).mockResolvedValue(session(false));

    const view = await renderPage({});

    expect(ResendVerificationForm).toHaveBeenCalledWith(
      expect.objectContaining({ address: EMAIL }),
      undefined,
    );
    expect(view.baseElement.textContent).toContain(EMAIL);
  });

  it('asks for an address when there is no session to name one', async () => {
    vi.mocked(getSession).mockResolvedValue(null);

    await renderPage({});

    expect(ResendVerificationForm).toHaveBeenCalledWith(
      expect.objectContaining({ address: undefined }),
      undefined,
    );
  });

  /*
   * Coming back to this screen after confirming used to be told a mail was on its way, which was
   * never true and left the only way out looking like a wait.
   */
  it('tells an already-confirmed caller there is nothing to do, and offers no resend', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true));

    await renderPage({});

    expect(ResendVerificationForm).not.toHaveBeenCalled();
    expect(ConfirmEmailForm).not.toHaveBeenCalled();
  });

  /*
   * The correction is the other half of the escape hatch, and it needs the session: the route it
   * posts to authenticates, so offering the form to a caller who followed a broken link would be
   * a button that cannot work.
   */
  it('offers the correction to a caller who has a session and no confirmation', async () => {
    vi.mocked(getSession).mockResolvedValue(session(false));

    await renderPage({});

    expect(ChangeEmailForm).toHaveBeenCalled();
  });

  it('offers no correction without a session', async () => {
    vi.mocked(getSession).mockResolvedValue(null);

    await renderPage({});

    expect(ChangeEmailForm).not.toHaveBeenCalled();
  });

  it('offers no correction once the address is confirmed', async () => {
    vi.mocked(getSession).mockResolvedValue(session(true));

    await renderPage({});

    expect(ChangeEmailForm).not.toHaveBeenCalled();
  });
});
