import { render } from '@testing-library/react';
import { createTranslator } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: vi.fn(async (namespace: string) =>
    createTranslator({ locale: 'es', messages, namespace: namespace as never }),
  ),
}));
vi.mock('@/lib/i18n/formatters-server', () => ({
  getFormatters: vi.fn(async () => ({ dateNumeric: String, currency: String })),
}));
vi.mock('@/lib/api/account', () => ({ getAccount: vi.fn() }));
vi.mock('@/lib/api/clients', () => ({
  getClient: vi.fn(),
  getClientTags: vi.fn(),
  getClientFiscal: vi.fn(),
}));
vi.mock('@/app/(protected)/_components/page-header', () => ({ PageHeader: () => null }));
vi.mock('@/components/back-link', () => ({ BackLink: () => null }));
vi.mock('@/app/(protected)/clients/[id]/_components/client-tag-editor', () => ({
  ClientTagEditor: () => null,
}));
vi.mock('@/app/(protected)/clients/[id]/_components/client-fiscal-form', () => ({
  ClientFiscalForm: () => <div data-testid="fiscal-form" />,
}));

const { getAccount } = await import('@/lib/api/account');
const { getClient, getClientFiscal, getClientTags } = await import('@/lib/api/clients');
const { default: ClientProfilePage } = await import('./page');

async function renderPage() {
  return render(await ClientProfilePage({ params: Promise.resolve({ id: 'c1' }) }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getClient).mockResolvedValue({
    client: { id: 'c1', name: 'Obra Norte', phone: null, email: null },
    tags: [],
    sales: [],
  } as never);
  vi.mocked(getClientTags).mockResolvedValue([]);
  vi.mocked(getClientFiscal).mockResolvedValue({} as never);
});

describe('ClientProfilePage fiscal data', () => {
  // A client's fiscal data only serves invoicing with Coti, so off it is neither read nor shown.
  it('leaves the fiscal form out while invoicing is off', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: false } as never);

    const view = await renderPage();

    expect(view.queryByTestId('fiscal-form')).toBeNull();
    expect(getClientFiscal).not.toHaveBeenCalled();
  });

  it('shows the fiscal form while invoicing is on', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: true } as never);

    const view = await renderPage();

    expect(view.getByTestId('fiscal-form')).toBeTruthy();
    expect(getClientFiscal).toHaveBeenCalledWith('c1');
  });
});
