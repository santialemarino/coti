import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { UserFormDialog } from '@/components/user-form-dialog';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

describe('UserFormDialog', () => {
  // The onboarding team step creates users through this dialog: most people added are sellers.
  it('creates a seller by default and still lets the role be chosen', () => {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <UserFormDialog
          open
          onOpenChange={vi.fn()}
          mode="create"
          user={null}
          assigned={[]}
          branches={[]}
          isSelf={false}
          mailDelivery
          onSubmit={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.getByRole('radio', { name: 'Vendedor' }).getAttribute('aria-checked')).toBe(
      'true',
    );
    expect(screen.getByRole('radio', { name: 'Administrador' }).getAttribute('aria-checked')).toBe(
      'false',
    );
    expect(
      screen.getByRole('radio', { name: 'Mandarle una invitación' }).getAttribute('aria-checked'),
    ).toBe('true');
  });

  // With mail going only to a log an invite could never arrive, so there is one way in to offer.
  it('offers no invite, only the password, while mail cannot be delivered', () => {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <UserFormDialog
          open
          onOpenChange={vi.fn()}
          mode="create"
          user={null}
          assigned={[]}
          branches={[]}
          isSelf={false}
          mailDelivery={false}
          onSubmit={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.queryByRole('radio', { name: 'Mandarle una invitación' })).toBeNull();
    expect(screen.getByText(messages.users.access.passwordOnly)).toBeTruthy();
    expect(document.querySelector('input[name="password"]')).toBeTruthy();
  });

  // The API refuses an admin's own address change from here, so the field is not offered at all.
  it('shows an admin their own address as text, with the way to change it', () => {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <UserFormDialog
          open
          onOpenChange={vi.fn()}
          mode="edit"
          user={{
            id: 'u1',
            name: 'Ana Admin',
            email: 'ana@corralon.test',
            role: 'ADMIN',
            isActive: true,
            branchIds: [],
            inviteStatus: null,
            lastLoginAt: null,
          }}
          assigned={[]}
          branches={[]}
          isSelf
          mailDelivery
          onSubmit={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    expect(screen.queryByRole('textbox', { name: /Correo electrónico/ })).toBeNull();
    expect(screen.getByText('ana@corralon.test')).toBeTruthy();
    expect(
      screen.getByRole('link', { name: messages.users.email.ownAddressLink }).getAttribute('href'),
    ).toBe(ROUTES.emailSettings);
  });

  it("lets an admin edit another user's address", () => {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <UserFormDialog
          open
          onOpenChange={vi.fn()}
          mode="edit"
          user={{
            id: 'u2',
            name: 'Vera Vendedora',
            email: 'vera@corralon.test',
            role: 'SELLER',
            isActive: true,
            branchIds: [],
            inviteStatus: null,
            lastLoginAt: null,
          }}
          assigned={[]}
          branches={[]}
          isSelf={false}
          mailDelivery
          onSubmit={vi.fn()}
        />
      </NextIntlClientProvider>,
    );

    expect(
      (screen.getByRole('textbox', { name: /Correo electrónico/ }) as HTMLInputElement).value,
    ).toBe('vera@corralon.test');
    expect(screen.queryByRole('link', { name: messages.users.email.ownAddressLink })).toBeNull();
  });
});
