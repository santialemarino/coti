import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { ContactVerification } from '@/app/profile/_components/contact-verification';
import { confirmContactVerification, requestContactVerification } from '@/app/profile/actions';
import messages from '@/translations/es.json';

const mocks = vi.hoisted(() => ({ refresh: vi.fn(), success: vi.fn(), error: vi.fn() }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: mocks.refresh }) }));
vi.mock('sonner', () => ({ toast: { success: mocks.success, error: mocks.error } }));
vi.mock('@/app/profile/actions', () => ({
  requestContactVerification: vi.fn(),
  confirmContactVerification: vi.fn(),
}));
beforeEach(() => vi.clearAllMocks());

function show(kind: 'EMAIL' | 'WHATSAPP', onSubmit = vi.fn()) {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <form onSubmit={onSubmit}>
        <ContactVerification
          kind={kind}
          value={kind === 'EMAIL' ? 'test@example.test' : '+5491112345678'}
          verified={false}
        />
      </form>
    </NextIntlClientProvider>,
  );
  return onSubmit;
}

it('requests an email link without claiming that the contact is verified', async () => {
  vi.mocked(requestContactVerification).mockResolvedValue({ id: 'proof' });
  show('EMAIL');
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.verify }));
  await waitFor(() =>
    expect(mocks.success).toHaveBeenCalledWith(messages.customerProfile.verificationEmailSent),
  );
  expect(confirmContactVerification).not.toHaveBeenCalled();
  expect(mocks.refresh).not.toHaveBeenCalled();
});

it('confirms a WhatsApp code without submitting the surrounding profile form', async () => {
  vi.mocked(requestContactVerification).mockResolvedValue({ id: 'proof' });
  vi.mocked(confirmContactVerification).mockResolvedValue({});
  const outer = show('WHATSAPP');
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.verify }));
  const input = await screen.findByLabelText(new RegExp(messages.customerProfile.code));
  fireEvent.change(input, { target: { value: '123456' } });
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.confirmContact }));
  await waitFor(() => expect(confirmContactVerification).toHaveBeenCalledWith('proof', '123456'));
  expect(outer).not.toHaveBeenCalled();
  expect(mocks.refresh).toHaveBeenCalled();
});

it('keeps a failed verification available for retry', async () => {
  vi.mocked(requestContactVerification).mockRejectedValue(new Error('offline'));
  show('EMAIL');
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.verify }));
  await waitFor(() =>
    expect(mocks.error).toHaveBeenCalledWith(
      messages.customerProfile.errors.verificationUnavailable,
    ),
  );
  expect(
    screen.getByRole('button', { name: messages.customerProfile.verify }).hasAttribute('disabled'),
  ).toBe(false);
});
