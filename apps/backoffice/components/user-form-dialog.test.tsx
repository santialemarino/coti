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
  });
});
