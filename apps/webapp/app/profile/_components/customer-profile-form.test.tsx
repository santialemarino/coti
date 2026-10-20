import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { CustomerProfileForm } from '@/app/profile/_components/customer-profile-form';
import { saveCustomerProfile } from '@/app/profile/actions';
import messages from '@/translations/es.json';

vi.mock('@/app/profile/actions', () => ({ saveCustomerProfile: vi.fn(async () => ({})) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

beforeEach(() => vi.clearAllMocks());

function renderForm(
  contacts: { kind: 'EMAIL' | 'WHATSAPP'; value: string; primary: boolean }[] = [],
) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <CustomerProfileForm
        profile={{
          clientId: null,
          taxId: '20123456786',
          name: 'Customer',
          reviewPending: false,
          contacts,
        }}
      />
    </NextIntlClientProvider>,
  );
}

it('adds fields one at a time and saves partial optional contact data', async () => {
  renderForm();
  expect(screen.getByLabelText('Emails 1')).toBeDefined();
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.addEmail }));
  fireEvent.change(screen.getByLabelText('Emails 1'), { target: { value: 'one@example.test' } });
  expect(screen.getByLabelText('Emails 2')).toBeDefined();
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.save }));
  await waitFor(() => expect(saveCustomerProfile).toHaveBeenCalledTimes(1));
  expect(
    vi.mocked(saveCustomerProfile).mock.calls[0]?.[0].contacts.filter((contact) => contact.value),
  ).toEqual([{ kind: 'EMAIL', value: 'one@example.test', primary: true }]);
});

it('requires choosing a replacement after removing a primary contact', async () => {
  renderForm([
    { kind: 'EMAIL', value: 'one@example.test', primary: true },
    { kind: 'EMAIL', value: 'two@example.test', primary: false },
  ]);
  const remove = screen.getAllByRole('button', { name: messages.customerProfile.remove });
  fireEvent.click(remove[1]!);
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(
    Array.from(dialog.querySelectorAll('button')).find(
      (button) => button.textContent === messages.customerProfile.remove,
    )!,
  );
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.save }));
  await waitFor(() =>
    expect(screen.getByText(messages.customerProfile.errors.choosePrimary)).toBeDefined(),
  );
  expect(saveCustomerProfile).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.makePrimary }));
  fireEvent.click(screen.getByRole('button', { name: messages.customerProfile.save }));
  await waitFor(() => expect(saveCustomerProfile).toHaveBeenCalledTimes(1));
});
