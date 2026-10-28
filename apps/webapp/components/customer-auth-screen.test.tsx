import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { signUpCustomer } from '@/app/login/actions';
import type { CustomerAuthMode } from '@/components/customer-auth-schema';
import { CustomerAuthScreen } from '@/components/customer-auth-screen';
import messages from '@/translations/es.json';

vi.mock('@/app/login/actions', () => ({
  signInCustomer: vi.fn(),
  signUpCustomer: vi.fn(),
  completeCustomerGoogle: vi.fn(),
}));

function screen(mode: CustomerAuthMode) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <CustomerAuthScreen
        mode={mode}
        supplierName="Corralón Norte"
        next="/orders"
        minimum={12}
        googleEnabled
      />
    </NextIntlClientProvider>,
  );
}
beforeEach(() => vi.clearAllMocks());

it('offers Google first with explicit local login and registration alternatives', () => {
  const view = screen('welcome');
  expect(view.getByRole('link', { name: 'Continuar con Google' }).getAttribute('href')).toContain(
    '/api/customer/google/start',
  );
  expect(view.getByRole('link', { name: 'Ingresar con CUIT/CUIL' })).toBeTruthy();
  expect(view.getByRole('link', { name: 'Crear cuenta' })).toBeTruthy();
  expect(view.queryByLabelText('Contraseña', { exact: false })).toBeNull();
});

it('creates password credentials with a single commercial contact', async () => {
  vi.mocked(signUpCustomer).mockResolvedValue({ error: 'unavailable' });
  const view = screen('signup');
  fireEvent.change(view.getByLabelText('Nombre y apellido', { exact: false }), {
    target: { value: 'Juan Perez' },
  });
  fireEvent.change(view.getByLabelText(/^WhatsApp/), { target: { value: '11 5555 0101' } });
  fireEvent.change(view.getByLabelText('CUIT/CUIL', { exact: false }), {
    target: { value: '20123456786' },
  });
  fireEvent.change(view.getByLabelText(/^Contraseña/), { target: { value: 'Customer-pass123!' } });
  fireEvent.change(view.getByLabelText(/^Repetí la contraseña/), {
    target: { value: 'Customer-pass123!' },
  });
  fireEvent.click(view.getByRole('button', { name: 'Crear cuenta' }));
  await waitFor(() =>
    expect(signUpCustomer).toHaveBeenCalledWith(
      expect.objectContaining({ taxId: '20123456786', email: '', password: 'Customer-pass123!' }),
    ),
  );
});

it('asks only for CUIT/CUIL after Google has authenticated the user', () => {
  const view = screen('google');
  expect(view.getByLabelText('CUIT/CUIL', { exact: false })).toBeTruthy();
  expect(view.queryByLabelText('Email', { exact: false })).toBeNull();
  expect(view.queryByLabelText('Contraseña', { exact: false })).toBeNull();
});
