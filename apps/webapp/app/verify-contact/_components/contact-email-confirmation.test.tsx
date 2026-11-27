import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { confirmContactVerification } from '@/app/profile/actions';
import {
  ContactEmailConfirmation,
  type EmailConfirmationState,
} from '@/app/verify-contact/_components/contact-email-confirmation';
import messages from '@/translations/es.json';

vi.mock('@/app/profile/actions', () => ({ confirmContactVerification: vi.fn() }));

const copy = messages.customerVerification;

beforeEach(() => vi.clearAllMocks());

function renderConfirmation(initialState: EmailConfirmationState, next?: string) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ContactEmailConfirmation
        challenge="challenge"
        token="token"
        email="obra@cliente.test"
        next={next}
        supplierName="Corralón San Martín"
        initialState={initialState}
      />
    </NextIntlClientProvider>,
  );
}

it('names the address and what confirming it opens', () => {
  renderConfirmation('pending', '/quotes/tok-1');
  expect(
    screen.getByText(
      'Confirmá que obra@cliente.test es tuyo para ver tu cotización de Corralón San Martín.',
    ),
  ).toBeTruthy();
});

it('returns to the quote the server kept with the proof once confirmed', async () => {
  vi.mocked(confirmContactVerification).mockResolvedValue({ next: '/quotes/tok-1' });
  renderConfirmation('pending');

  fireEvent.click(screen.getByRole('button', { name: copy.confirm.submit }));

  const link = await screen.findByRole('link', { name: copy.verified.quote });
  expect(link.getAttribute('href')).toBe('/quotes/tok-1');
  expect(confirmContactVerification).toHaveBeenCalledWith('challenge', 'token');
  expect(document.activeElement).toBe(link);
});

it('falls back to the orders when the proof kept no destination', async () => {
  vi.mocked(confirmContactVerification).mockResolvedValue({});
  renderConfirmation('pending');

  fireEvent.click(screen.getByRole('button', { name: copy.confirm.submit }));

  const link = await screen.findByRole('link', { name: copy.verified.orders });
  expect(link.getAttribute('href')).toBe('/orders');
});

it('keeps the customer on the form with the reason when the confirmation is refused', async () => {
  vi.mocked(confirmContactVerification).mockResolvedValue({ error: 'El enlace venció.' });
  renderConfirmation('pending');

  fireEvent.click(screen.getByRole('button', { name: copy.confirm.submit }));

  expect(await screen.findByText('El enlace venció.')).toBeTruthy();
  expect(screen.getByRole('button', { name: copy.confirm.submit })).toBeTruthy();
});

it('offers a new link instead of a button that cannot work', () => {
  renderConfirmation('invalid', '/quotes/tok-1');

  expect(screen.getByText(copy.invalid.title)).toBeTruthy();
  expect(screen.queryByRole('button', { name: copy.confirm.submit })).toBeNull();
  expect(screen.getByRole('link', { name: copy.invalid.action }).getAttribute('href')).toBe(
    '/verify-contact?next=%2Fquotes%2Ftok-1',
  );
});
