import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { UserFormDialog } from '@/components/user-form-dialog';
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
});
