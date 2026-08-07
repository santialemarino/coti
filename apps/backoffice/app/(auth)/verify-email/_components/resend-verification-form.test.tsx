import { fireEvent, render, waitFor, type RenderResult } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ResendVerificationForm } from '@/app/(auth)/verify-email/_components/resend-verification-form';
import { RESEND_COOLDOWN_SECONDS } from '@/lib/config';
import messages from '@/translations/es.json';

vi.mock('@/app/(auth)/verify-email/actions', () => ({ resendVerification: vi.fn() }));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { resendVerification } = await import('@/app/(auth)/verify-email/actions');
const { toast } = await import('sonner');

const copy = messages.auth.verifyEmail;
const EMAIL = 'ana@corralonsanmartin.test';

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderForm() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ResendVerificationForm />
    </NextIntlClientProvider>,
  );
}

function submitButton(view: RenderResult): HTMLButtonElement {
  const button = view.baseElement.querySelector('button[type="submit"]');
  if (!(button instanceof HTMLButtonElement)) throw new Error('no submit button on screen');
  return button;
}

// Clicking a submit button does not reach a React form in jsdom, which implements no requestSubmit.
function submit(view: RenderResult) {
  const form = view.baseElement.querySelector('form');
  if (!(form instanceof HTMLFormElement)) throw new Error('no form on screen');
  fireEvent.submit(form);
}

// The countdown is the button's label, so the number on it is the state under test.
function secondsLeft(view: RenderResult): number {
  const found = submitButton(view).textContent?.match(/\d+/);
  if (!found) throw new Error(`no countdown on the button: ${submitButton(view).textContent}`);
  return Number(found[0]);
}

function fillEmail(view: RenderResult, value = EMAIL) {
  const input = view.baseElement.querySelector('input[name="email"]');
  if (!(input instanceof HTMLInputElement)) throw new Error('no email field on screen');
  fireEvent.change(input, { target: { value } });
}

async function send(view: RenderResult) {
  fillEmail(view);
  submit(view);
  await waitFor(() => expect(resendVerification).toHaveBeenCalled());
}

/*
 * `shouldAdvanceTime` is required for `waitFor` to resolve at all under fake timers — without it
 * nothing drives the poll and every wait here times out. The cost is that fake time then moves both
 * with real time and with each explicit advance, so the countdown lands on a number this test
 * cannot predict. It asserts the contract instead: shut with a number on it, the number falls, and
 * it opens once the cooldown has passed.
 */
beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => vi.useRealTimers());

describe('ResendVerificationForm', () => {
  /*
   * A mail that never arrives looks exactly like one that was never sent, so replacing the form
   * with a confirmation leaves the one caller who needs it most with no way to ask again.
   */
  it('keeps the form on screen after a send, and confirms with a toast', async () => {
    vi.mocked(resendVerification).mockResolvedValue({ sent: true });
    const view = renderForm();

    await send(view);

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith(copy.resend.sent));
    expect(view.baseElement.querySelector('input[name="email"]')).not.toBeNull();
    expect(view.queryByText(copy.resend.sent)).toBeNull();
  });

  it('shuts the button for the cooldown and counts it down in the label', async () => {
    vi.mocked(resendVerification).mockResolvedValue({ sent: true });
    const view = renderForm();

    await send(view);

    await waitFor(() => expect(submitButton(view)).toHaveProperty('disabled', true));
    expect(secondsLeft(view)).toBe(RESEND_COOLDOWN_SECONDS);

    await vi.advanceTimersByTimeAsync(2000);

    await waitFor(() => expect(secondsLeft(view)).toBeLessThan(RESEND_COOLDOWN_SECONDS));
    expect(secondsLeft(view)).toBeGreaterThan(0);
  });

  it('opens the button again once the cooldown runs out', async () => {
    vi.mocked(resendVerification).mockResolvedValue({ sent: true });
    const view = renderForm();

    await send(view);
    await waitFor(() => expect(submitButton(view)).toHaveProperty('disabled', true));

    await vi.advanceTimersByTimeAsync(RESEND_COOLDOWN_SECONDS * 1000);

    await waitFor(() => expect(submitButton(view)).toHaveProperty('disabled', false));
    expect(submitButton(view).textContent).toContain(copy.resend.submit);
  });

  /*
   * A shut button stops a click, not the Enter key that reaches the form behind it — and every
   * extra send spends an allowance the API answers 202 for either way, so nothing would say no.
   */
  it('refuses a second send while the cooldown is running', async () => {
    vi.mocked(resendVerification).mockResolvedValue({ sent: true });
    const view = renderForm();

    await send(view);
    await waitFor(() => expect(submitButton(view)).toHaveProperty('disabled', true));

    submit(view);
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(resendVerification).toHaveBeenCalledOnce();
  });

  it('puts a refused address on the field and starts no cooldown', async () => {
    vi.mocked(resendVerification).mockResolvedValue({ error: 'INVALID_BODY', field: 'email' });
    const view = renderForm();

    await send(view);

    await waitFor(() => expect(view.getByText(copy.resend.errors.INVALID_BODY)).toBeTruthy());
    expect(submitButton(view)).toHaveProperty('disabled', false);
    expect(toast.success).not.toHaveBeenCalled();
  });
});
