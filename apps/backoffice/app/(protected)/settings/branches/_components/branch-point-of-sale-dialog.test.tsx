import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { BranchPointOfSaleDialog } from '@/app/(protected)/settings/branches/_components/branch-point-of-sale-dialog';
import { verifyBranchPointOfSale } from '@/app/(protected)/settings/branches/actions';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));
vi.mock('@/app/(protected)/settings/branches/actions', () => ({
  verifyBranchPointOfSale: vi.fn(),
}));
const branch = {
  id: 'b0000000-0000-4000-8000-000000000001',
  name: 'Centro',
  address: null,
  email: null,
  defaultExpiryDays: 7,
  isActive: true,
};
const setup = {
  enabled: true,
  taxId: '20329642330',
  csr: 'request',
  hasCertificate: true,
  certificateExpiresAt: null,
  pointOfSale: 99,
  verifiedAt: null,
};
const close = vi.fn();
function show(hasCertificate = true) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <BranchPointOfSaleDialog
        branch={branch}
        setup={{ ...setup, hasCertificate }}
        companyTaxId="20450751457"
        pointOfSale={1}
        open
        onOpenChange={close}
      />
    </NextIntlClientProvider>,
  );
}
beforeEach(() => vi.clearAllMocks());
describe('branch fiscal settings', () => {
  it('uses the row point of sale and pins its verification to that row', async () => {
    vi.mocked(verifyBranchPointOfSale).mockResolvedValue({
      ok: true,
      connection: { verified: true, failure: '', lastNumber: 0 },
    });
    const view = show();
    expect(view.getByRole('textbox', { name: /Punto de venta/ })).toHaveProperty('value', '1');
    fireEvent.submit(view.baseElement.querySelector('form')!);
    await waitFor(() =>
      expect(verifyBranchPointOfSale).toHaveBeenCalledWith({ pointOfSale: '1' }, branch.id),
    );
    await waitFor(() => expect(close).toHaveBeenCalledWith(false));
  });
  it('keeps a rejected verification open and actionable', async () => {
    vi.mocked(verifyBranchPointOfSale).mockResolvedValue({
      ok: true,
      connection: { verified: false, failure: 'POINT_OF_SALE_REJECTED', lastNumber: 0 },
    });
    const view = show();
    fireEvent.submit(view.baseElement.querySelector('form')!);
    await waitFor(() =>
      expect(view.getByText(messages.arca.failures.POINT_OF_SALE_REJECTED)).toBeTruthy(),
    );
    expect(close).not.toHaveBeenCalled();
  });
  it('requires the company certificate before accepting a point of sale', () => {
    const view = show(false);
    expect(view.queryByRole('textbox')).toBeNull();
    expect(
      view.getByRole('link', { name: messages.branches.billing.connection }).getAttribute('href'),
    ).toBe('/settings/arca');
  });
});
