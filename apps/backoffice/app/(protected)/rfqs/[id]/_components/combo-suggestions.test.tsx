import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import { ComboSuggestions } from '@/app/(protected)/rfqs/[id]/_components/combo-suggestions';
import messages from '@/translations/es.json';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({
  fetchComboSuggestions: vi.fn(),
  addComboItems: vi.fn(),
}));

const { fetchComboSuggestions, addComboItems } = await import('@/lib/api/rfqs-client');
const { toast } = await import('sonner');

const SUGGESTION = {
  comboId: 'c1',
  name: 'Obra gruesa',
  description: null,
  present: [
    {
      productId: 'p1',
      quantity: '10.00',
      productCode: null,
      productName: 'Cemento',
      productUnit: 'bolsa',
    },
  ],
  missing: [
    {
      productId: 'p2',
      quantity: '2.00',
      productCode: null,
      productName: 'Arena',
      productUnit: 'm3',
    },
  ],
};

beforeEach(() => vi.clearAllMocks());

function renderPanel(onAdded = vi.fn().mockResolvedValue(undefined)) {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <ComboSuggestions quoteId="q1" branchId="b1" items={[]} onAdded={onAdded} />
    </NextIntlClientProvider>,
  );
  return { view, onAdded };
}

it('renders nothing while no combo applies', async () => {
  vi.mocked(fetchComboSuggestions).mockResolvedValue([]);
  const { view } = renderPanel();
  await waitFor(() => expect(fetchComboSuggestions).toHaveBeenCalledWith('q1', 'b1'));
  expect(view.queryByText(messages.rfqs.detail.combos.title)).toBeNull();
});

it('adds the missing products and reloads the quote', async () => {
  vi.mocked(fetchComboSuggestions).mockResolvedValue([SUGGESTION]);
  vi.mocked(addComboItems).mockResolvedValue([]);
  const { view, onAdded } = renderPanel();

  const button = await view.findByRole('button', { name: /Agregar el que falta/ });
  expect(view.getByText('Obra gruesa')).toBeTruthy();
  fireEvent.click(button);

  await waitFor(() => expect(onAdded).toHaveBeenCalled());
  expect(addComboItems).toHaveBeenCalledWith('q1', 'c1', 'b1');
  expect(toast.success).toHaveBeenCalled();
});

it('stays quiet when the suggestions cannot load', async () => {
  vi.mocked(fetchComboSuggestions).mockRejectedValue(new Error('down'));
  const { view } = renderPanel();
  await waitFor(() => expect(fetchComboSuggestions).toHaveBeenCalled());
  expect(view.queryByText(messages.rfqs.detail.combos.title)).toBeNull();
  expect(toast.error).not.toHaveBeenCalled();
});
