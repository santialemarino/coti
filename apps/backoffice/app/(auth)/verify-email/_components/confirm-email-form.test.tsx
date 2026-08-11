import { render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/app/(auth)/verify-email/actions', () => ({ confirmEmail: vi.fn() }));
vi.mock('@/app/(auth)/verify-email/_components/resend-verification-form', () => ({
  ResendVerificationForm: vi.fn(() => null),
}));

const { ConfirmEmailForm } =
  await import('@/app/(auth)/verify-email/_components/confirm-email-form');
const { ResendVerificationForm } =
  await import('@/app/(auth)/verify-email/_components/resend-verification-form');
const { confirmEmail } = await import('@/app/(auth)/verify-email/actions');
const messages = (await import('@/translations/es.json')).default;

const copy = messages.auth.verifyEmail;
const EMAIL = 'ana@corralonsanmartin.test';

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderForm(address?: string) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ConfirmEmailForm token="abc123" address={address} />
    </NextIntlClientProvider>,
  );
}

function linkHrefs(view: RenderResult): string[] {
  return [...view.baseElement.querySelectorAll('a')].map((a) => a.getAttribute('href') ?? '');
}

beforeEach(() => {
  vi.clearAllMocks();
});

/*
 * Every branch of this screen has to answer the same question — is there a session, and whose
 * address is it — and the address is what answers both. A branch that forgets sends the caller
 * somewhere the gate bounces, or asks them to type what the app already knows.
 */
describe('with no session', () => {
  it('sends a confirmed caller to log in, since home would only bounce', async () => {
    vi.mocked(confirmEmail).mockResolvedValue({ done: true });
    const view = renderForm();

    view.baseElement.querySelector('form')?.requestSubmit?.();
    await waitFor(() => expect(view.getByText(copy.done)).toBeTruthy());

    expect(linkHrefs(view)).toContain('/login');
  });

  it('asks for an address on a burned link, having none of its own', () => {
    const view = renderForm();

    expect(view.getByText(copy.prompt)).toBeTruthy();
    expect(linkHrefs(view)).toContain('/login');
  });
});

describe('with the caller address known', () => {
  /*
   * A scanner or a corporate link checker burns the token before the person clicks, so this is
   * the most common failure here — and the one place the address is most obviously already known.
   */
  it('resends to that address on a burned link instead of asking for one', async () => {
    vi.mocked(confirmEmail).mockResolvedValue({ error: 'INVALID_LINK' });
    const view = renderForm(EMAIL);

    view.baseElement.querySelector('form')?.requestSubmit?.();

    await waitFor(() => expect(ResendVerificationForm).toHaveBeenCalled());
    expect(ResendVerificationForm).toHaveBeenCalledWith(
      expect.objectContaining({ address: EMAIL }),
      undefined,
    );
  });

  it('goes home rather than to the login screen', () => {
    const view = renderForm(EMAIL);

    expect(linkHrefs(view)).toContain('/');
    expect(linkHrefs(view)).not.toContain('/login');
  });
});
