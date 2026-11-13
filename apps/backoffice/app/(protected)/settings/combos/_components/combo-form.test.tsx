import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ComboForm } from '@/app/(protected)/settings/combos/_components/combo-form';
import type { Branch } from '@/lib/api/branches';
import messages from '@/translations/es.json';

const push = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ push, refresh: vi.fn() }) }));
vi.mock('@/app/(protected)/settings/combos/actions', () => ({
  createCombo: vi.fn(),
  updateCombo: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { createCombo } = await import('@/app/(protected)/settings/combos/actions');
const copy = messages.combos.form;

function branch(id: string, name: string): Branch {
  return { id, name, address: null, email: null, defaultExpiryDays: 7, isActive: true };
}

const VALUES = {
  name: 'Obra gruesa',
  description: '',
  isActive: true,
  items: [
    { productId: 'p1', label: 'Cemento', quantity: '10' },
    { productId: 'p2', label: 'Arena', quantity: '2' },
  ],
  branchIds: ['b1'],
};

function renderForm(branches: Branch[]) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ComboForm initialValues={VALUES} branches={branches} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('ComboForm', () => {
  it('shows the chosen products and offers no branch choice with a single branch', () => {
    renderForm([branch('b1', 'Central')]);
    expect(screen.getByText('Cemento')).toBeTruthy();
    expect(screen.getByText('Arena')).toBeTruthy();
    expect(screen.queryByText(copy.sections.branches)).toBeNull();
  });

  it('refuses to leave a combo without a branch', async () => {
    renderForm([branch('b1', 'Central'), branch('b2', 'Norte')]);
    fireEvent.click(screen.getByRole('checkbox', { name: 'Central' }));
    fireEvent.click(screen.getByRole('button', { name: copy.submit.create }));
    await waitFor(() => expect(screen.getByText(copy.branches.required)).toBeTruthy());
    expect(createCombo).not.toHaveBeenCalled();
  });

  it('creates the combo and goes back to the list', async () => {
    vi.mocked(createCombo).mockResolvedValue({ ok: true });
    renderForm([branch('b1', 'Central')]);
    fireEvent.click(screen.getByRole('button', { name: copy.submit.create }));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/settings/combos'));
  });
});
