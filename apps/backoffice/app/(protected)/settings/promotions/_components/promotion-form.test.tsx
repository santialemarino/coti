import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { PromotionForm } from '@/app/(protected)/settings/promotions/_components/promotion-form';
import type { PromotionValues } from '@/app/(protected)/settings/promotions/form-schema';
import { EMPTY_PROMOTION } from '@/app/(protected)/settings/promotions/promotion-values';
import type { Branch } from '@/lib/api/branches';
import type { ProductFamily } from '@/lib/api/products';
import messages from '@/translations/es.json';

const push = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ push, refresh: vi.fn() }) }));
vi.mock('@/app/(protected)/settings/promotions/actions', () => ({
  createPromotion: vi.fn(),
  updatePromotion: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { createPromotion, updatePromotion } =
  await import('@/app/(protected)/settings/promotions/actions');

const copy = messages.promotions.form;
const FAMILIES: ProductFamily[] = [
  { id: 'f1', name: 'Cementos', subgroups: [{ id: 's1', name: 'Portland' }] },
];
const BRANCHES: Branch[] = [
  { id: 'b1', name: 'Central', address: null, email: null, defaultExpiryDays: 7, isActive: true },
];

function renderForm(initialValues: PromotionValues, promotionId?: string) {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <PromotionForm
        promotionId={promotionId}
        initialValues={initialValues}
        families={FAMILIES}
        branches={BRANCHES}
      />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('PromotionForm', () => {
  it('renders a stored item set with every kind of target', () => {
    renderForm({
      ...EMPTY_PROMOTION,
      name: 'Obra gruesa',
      conditionType: 'ITEM_SET',
      actionValue: '10',
      conditions: [
        { kind: 'product', targetId: 'p1', label: 'Cemento 50 kg', minQuantity: '' },
        { kind: 'family', targetId: 'f1', label: '', minQuantity: '2' },
        { kind: 'subgroup', targetId: 's1', label: '', minQuantity: '' },
      ],
    });

    expect(screen.getByText('Cemento 50 kg')).toBeTruthy();
    expect(screen.getByText('Cementos')).toBeTruthy();
    expect(screen.getByText('Portland')).toBeTruthy();
  });

  it('shows the bands of a tiered rule instead of the single value', () => {
    renderForm({
      ...EMPTY_PROMOTION,
      name: 'Escalonada',
      conditionType: 'QUANTITY_TIERED',
      conditions: [{ kind: 'product', targetId: 'p1', label: 'Cemento', minQuantity: '' }],
      tiers: [{ fromQuantity: '10', toQuantity: '', value: '5' }],
    });

    expect(screen.getByText(copy.sections.tiers.title)).toBeTruthy();
    expect(screen.queryByText(copy.actionValue.PERCENTAGE)).toBeNull();
  });

  // One open branch makes "every branch" and "this branch" the same rule.
  it('offers no scope with a single branch', () => {
    renderForm(EMPTY_PROMOTION);
    expect(screen.queryByText(copy.scope.label)).toBeNull();
  });

  it('refuses to submit an incomplete rule and calls nothing', async () => {
    renderForm(EMPTY_PROMOTION);
    fireEvent.click(screen.getByRole('button', { name: copy.submit.create }));

    await waitFor(() => expect(screen.getByText(copy.name.required)).toBeTruthy());
    expect(createPromotion).not.toHaveBeenCalled();
  });

  it('updates a stored rule and goes back to the list', async () => {
    vi.mocked(updatePromotion).mockResolvedValue({ ok: true, id: 'p9' });
    renderForm(
      {
        ...EMPTY_PROMOTION,
        name: 'Cemento 10 %',
        actionValue: '10',
        conditions: [{ kind: 'product', targetId: 'p1', label: 'Cemento', minQuantity: '' }],
      },
      'p9',
    );
    fireEvent.click(screen.getByRole('button', { name: copy.submit.update }));

    await waitFor(() => expect(push).toHaveBeenCalledWith('/settings/promotions'));
    expect(vi.mocked(updatePromotion).mock.calls[0]?.[0]).toBe('p9');
  });
});
