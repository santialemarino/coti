import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { PriceImport } from '@/app/(protected)/settings/prices/_components/price-import';
import type { Branch } from '@/lib/api/branches';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/settings/prices/actions', () => ({
  confirmPriceImport: vi.fn(),
  exportPrices: vi.fn(),
  previewPriceImport: vi.fn(),
}));

const { exportPrices, previewPriceImport } =
  await import('@/app/(protected)/settings/prices/actions');

const copy = messages.priceImport;
const BRANCH: Branch = {
  id: '11111111-1111-4111-8111-111111111111',
  name: 'Villa Bosch',
  address: null,
  defaultExpiryDays: 7,
  isActive: true,
};

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderImport() {
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <PriceImport branch={BRANCH} />
    </NextIntlClientProvider>,
  );

  // By type for the submit; the other two are the only non-submit buttons on the idle screen.
  const button = (label: string) => {
    const match = [...view.container.querySelectorAll('button')].find((candidate) =>
      candidate.textContent?.includes(label),
    );
    if (!match) throw new Error(`no button labelled ${label}`);
    return match;
  };

  const form = view.container.querySelector('form');
  if (!form) throw new Error('no form rendered');

  return { ...view, form, preview: button(copy.form.preview), export: button(copy.export.submit) };
}

/*
 * Submitted rather than clicked: jsdom does not implement requestSubmit, so a click on the
 * submit button never reaches the form action React installed and nothing runs.
 */
function submitPreview(form: HTMLFormElement) {
  fireEvent.submit(form);
}

beforeEach(() => vi.clearAllMocks());

describe('PriceImport pending state', () => {
  /*
   * The defect this pins: all three buttons share one transition, so a naive `pending` makes
   * exporting announce itself on the preview button. Only the button that started the action
   * may say it is busy — the others merely go disabled, because they are mutually exclusive.
   */
  it('only lets the running action claim the pending state', async () => {
    let release = () => {};
    vi.mocked(exportPrices).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ ok: false, error: 'INTERNAL' }))),
    );

    const view = renderImport();
    fireEvent.click(view.export);

    await waitFor(() => expect(view.export.getAttribute('aria-busy')).toBe('true'));
    expect(view.export.textContent).toContain(copy.export.submitting);
    expect(view.preview.getAttribute('aria-busy')).toBe('false');
    expect(view.preview.textContent).toContain(copy.form.preview);
    expect(view.preview.textContent).not.toContain(copy.form.previewing);
    // Still unusable, because the two actions cannot overlap.
    expect(view.preview.disabled).toBe(true);

    release();
    await waitFor(() => expect(exportPrices).toHaveBeenCalledOnce());
  });

  it('goes busy on the submit button when the preview is the action in flight', async () => {
    let release = () => {};
    vi.mocked(previewPriceImport).mockImplementation(
      () => new Promise((resolve) => (release = () => resolve({ ok: false, error: 'INTERNAL' }))),
    );

    const view = renderImport();
    submitPreview(view.form);

    await waitFor(() => expect(view.preview.getAttribute('aria-busy')).toBe('true'));
    expect(view.preview.textContent).toContain(copy.form.previewing);
    expect(view.export.getAttribute('aria-busy')).toBe('false');

    release();
    await waitFor(() => expect(previewPriceImport).toHaveBeenCalledOnce());
  });

  // The page resolves the branch and the component carries it, so no call can be made without
  // one — the API rejects a price write that names no branch.
  it('sends the page-resolved branch with every call', async () => {
    vi.mocked(exportPrices).mockResolvedValue({ ok: false, error: 'INTERNAL' });

    const view = renderImport();
    fireEvent.click(view.export);

    await waitFor(() => expect(exportPrices).toHaveBeenCalledWith(BRANCH.id));

    vi.mocked(previewPriceImport).mockResolvedValue({ ok: false, error: 'INVALID_INPUT' });
    submitPreview(view.form);

    await waitFor(() =>
      expect(previewPriceImport).toHaveBeenCalledWith(BRANCH.id, expect.any(FormData)),
    );
  });
});

describe('PriceImport partial confirmation', () => {
  it('allows valid rows and explains that invalid rows are skipped', async () => {
    vi.mocked(previewPriceImport).mockResolvedValue({
      ok: true,
      preview: {
        branchId: BRANCH.id,
        rows: [
          {
            rowNumber: 2,
            code: 'CEM-001',
            productName: 'Cemento',
            currentPrice: null,
            currentMinPrice: null,
            price: '10000.00',
            minPrice: null,
            currency: 'ARS',
            errors: [],
          },
          {
            rowNumber: 3,
            code: 'DESCONOCIDO',
            productName: '',
            currentPrice: null,
            currentMinPrice: null,
            price: '5000.00',
            minPrice: null,
            currency: 'ARS',
            errors: ['unknown_product'],
          },
        ],
        validRows: 1,
        invalidRows: 1,
        canConfirm: true,
        previewedAt: '2026-08-05T12:00:00Z',
      },
    });

    const view = renderImport();
    submitPreview(view.form);

    await waitFor(() => expect(view.container.textContent).toContain('se omitirán 1 fila'));
    const confirm = [...view.container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes(copy.confirm),
    );
    expect(confirm).toBeDefined();
    await waitFor(() => expect(confirm?.disabled).toBe(false));
  });
});
