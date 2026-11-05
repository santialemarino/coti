import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { AccountRecovery } from '@/app/forgot-password/_components/account-recovery';
import {
  listRecoveryContacts,
  resetWithRecoveryCode,
  sendRecoveryCode,
  type RecoveryContact,
} from '@/app/forgot-password/actions';
import messages from '@/translations/es.json';

vi.mock('@/app/forgot-password/actions', () => ({
  listRecoveryContacts: vi.fn(),
  sendRecoveryCode: vi.fn(),
  resetWithRecoveryCode: vi.fn(),
}));
beforeEach(() => vi.clearAllMocks());

const t = messages.customerRecovery;
const whatsapp: RecoveryContact = {
  id: 'wa',
  kind: 'WHATSAPP',
  masked: '+54 9 ••• ••••-0101',
  main: true,
  available: false,
  preselected: false,
};
const email: RecoveryContact = {
  id: 'mail',
  kind: 'EMAIL',
  masked: 'j•••@example.test',
  main: false,
  available: true,
  preselected: true,
};

async function findContacts(contacts: RecoveryContact[]) {
  vi.mocked(listRecoveryContacts).mockResolvedValue({ contacts });
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <AccountRecovery minimum={12} next="/orders" />
    </NextIntlClientProvider>,
  );
  fireEvent.change(screen.getByLabelText('CUIT/CUIL', { exact: false }), {
    target: { value: '20123456786' },
  });
  fireEvent.click(screen.getByRole('button', { name: t.continue }));
  await waitFor(() => expect(listRecoveryContacts).toHaveBeenCalledWith('20123456786'));
}

it('offers the masked contacts, disables WhatsApp without the template and preselects a usable one', async () => {
  await findContacts([whatsapp, email]);
  const phone = await screen.findByRole('radio', { name: /\+54 9 ••• ••••-0101/ });
  const mail = screen.getByRole('radio', { name: /j•••@example\.test/ });
  expect(phone.hasAttribute('disabled')).toBe(true);
  expect(phone.getAttribute('aria-checked')).toBe('false');
  expect(mail.getAttribute('aria-checked')).toBe('true');
  expect(screen.getByText(t.whatsappSoon)).toBeTruthy();
  expect(screen.getByText(t.main)).toBeTruthy();
});

it('explains that there is nothing to recover with when no contact is verified', async () => {
  await findContacts([]);
  expect(await screen.findByText(t.noContacts)).toBeTruthy();
  expect(screen.queryByRole('button', { name: t.send })).toBeNull();
});

it('explains that recovery by WhatsApp is not available yet when it is the only contact', async () => {
  await findContacts([whatsapp]);
  expect(await screen.findByText(t.whatsappOnly)).toBeTruthy();
  expect(screen.queryByRole('button', { name: t.send })).toBeNull();
});

it('sends the code to the chosen contact and resets the password with it', async () => {
  vi.mocked(sendRecoveryCode).mockResolvedValue({ id: 'challenge' });
  vi.mocked(resetWithRecoveryCode).mockResolvedValue({ error: 'code' });
  await findContacts([whatsapp, email]);
  fireEvent.click(await screen.findByRole('button', { name: t.send }));
  await waitFor(() => expect(sendRecoveryCode).toHaveBeenCalledWith('20123456786', 'mail'));
  const code = await screen.findByRole('textbox', { name: new RegExp(`^${t.code}`) });
  fireEvent.change(code, { target: { value: '123456' } });
  fireEvent.change(screen.getByLabelText(new RegExp(`^${t.password}`)), {
    target: { value: 'Replacement-pass123!' },
  });
  fireEvent.change(screen.getByLabelText(new RegExp(`^${t.confirmation}`)), {
    target: { value: 'Replacement-pass123!' },
  });
  fireEvent.click(screen.getByRole('button', { name: t.save }));
  await waitFor(() =>
    expect(resetWithRecoveryCode).toHaveBeenCalledWith({
      id: 'challenge',
      code: '123456',
      password: 'Replacement-pass123!',
      next: '/orders',
    }),
  );
  await waitFor(() => expect(code.getAttribute('aria-invalid')).toBe('true'));
});
