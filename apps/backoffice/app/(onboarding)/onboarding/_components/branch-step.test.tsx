import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { BranchStep } from '@/app/(onboarding)/onboarding/_components/branch-step';
import messages from '@/translations/es.json';

describe('BranchStep', () => {
  // The step does not show every field the branch schema reads; one it leaves without a value
  // would fail validation where the seller cannot see it, and the step could never be confirmed.
  it('confirms the branch as registered, with no field it does not show in the way', async () => {
    const onSubmit = vi.fn();
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <BranchStep
          branch={{
            id: 'b1',
            name: 'Morón',
            address: 'Rivadavia 18400',
            email: null,
            defaultExpiryDays: 7,
            isActive: true,
          }}
          formId="branch-step"
          onSubmit={onSubmit}
        />
        <button type="submit" form="branch-step">
          Continuar
        </button>
      </NextIntlClientProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Continuar' }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalled());
    expect(onSubmit.mock.calls[0]?.[0]).toMatchObject({ name: 'Morón', defaultExpiryDays: '7' });
  });
});
