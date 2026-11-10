import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ProductPriceDialog } from '@/app/(protected)/settings/catalog/_components/product-price-dialog';
import { getProductPrice, setProductPrice } from '@/app/(protected)/settings/catalog/actions';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/catalog/actions', () => ({
  getProductPrice: vi.fn(),
  setProductPrice: vi.fn(),
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

const BRANCH_ID = 'b0000000-0000-4000-8000-000000000002';
const BRANCHES = [
  {
    id: BRANCH_ID,
    name: 'Morón',
    address: null,
    email: null,
    defaultExpiryDays: 7,
    isActive: true,
  },
];
const copy = messages.products;

function renderEditor() {
  const onOpenChange = vi.fn();
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ProductPriceDialog
        open
        onOpenChange={onOpenChange}
        productId="product-1"
        productName="Cal Test"
        branches={BRANCHES}
        initialBranchId={BRANCH_ID}
      />
    </NextIntlClientProvider>,
  );
  return { ...view, onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getProductPrice).mockResolvedValue({
    ok: true,
    current: { price: '5000.00', minPrice: '4900.00', currency: 'ARS' },
  });
  vi.mocked(setProductPrice).mockResolvedValue({ ok: true, productId: 'product-1' });
});

describe('ProductPriceDialog', () => {
  it('loads the branch price, saves canonical decimals, and closes on success', async () => {
    const view = renderEditor();
    const price = await view.findByDisplayValue('5.000,00');
    await waitFor(() => expect((price as HTMLInputElement).value).toBe('5.000,00'));
    fireEvent.change(price, { target: { value: '5100,25' } });
    fireEvent.click(view.getByRole('button', { name: copy.priceEditor.save }));
    await waitFor(() =>
      expect(setProductPrice).toHaveBeenCalledWith('product-1', {
        branchId: BRANCH_ID,
        price: '5100.25',
        minPrice: '4900.00',
        currency: 'ARS',
      }),
    );
    await waitFor(() => expect(view.onOpenChange).toHaveBeenCalledWith(false));
  });

  it('refuses a floor above the price inline without writing', async () => {
    const view = renderEditor();
    const minimum = await view.findByLabelText(copy.minPrice.label);
    fireEvent.change(minimum, { target: { value: '6000' } });
    fireEvent.click(view.getByRole('button', { name: copy.priceEditor.save }));
    await view.findByText(copy.minPrice.abovePrice);
    expect(setProductPrice).not.toHaveBeenCalled();
  });

  it('lets the seller retry a failed read instead of saving a blank replacement', async () => {
    vi.mocked(getProductPrice).mockResolvedValueOnce({ error: 'UNREACHABLE' });
    const view = renderEditor();
    await view.findByRole('button', { name: copy.priceEditor.retry });
    expect(
      (view.getByRole('button', { name: copy.priceEditor.save }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(view.getByRole('button', { name: copy.priceEditor.retry }));
    await view.findByDisplayValue('5.000,00');
    expect(getProductPrice).toHaveBeenLastCalledWith('product-1', BRANCH_ID);
  });

  it('keeps the editor open when the API refuses the write', async () => {
    vi.mocked(setProductPrice).mockResolvedValueOnce({ error: 'FORBIDDEN' });
    const view = renderEditor();
    await view.findByDisplayValue('5.000,00');
    fireEvent.click(view.getByRole('button', { name: copy.priceEditor.save }));
    await waitFor(() => expect(setProductPrice).toHaveBeenCalled());
    expect(view.onOpenChange).not.toHaveBeenCalled();
    expect(view.getByRole('dialog')).toBeTruthy();
  });
});
