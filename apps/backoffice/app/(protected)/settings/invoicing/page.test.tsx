import { render } from '@testing-library/react';
import { createTranslator } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: vi.fn(async (namespace: string) =>
    createTranslator({ locale: 'es', messages, namespace: namespace as never }),
  ),
}));
vi.mock('@/lib/auth/session', () => ({ requireAdmin: vi.fn() }));
vi.mock('@/lib/api/account', () => ({ getAccount: vi.fn() }));
vi.mock('@/lib/api/arca-setup', () => ({ getARCASetup: vi.fn() }));
vi.mock('@/lib/api/invoicing-settings', () => ({ getInvoicingSettings: vi.fn() }));
vi.mock('@/lib/utils/arca-connection', () => ({ arcaConnectionState: vi.fn(() => 'connected') }));
vi.mock('@/app/(protected)/_components/page-header', () => ({ PageHeader: () => null }));
vi.mock('@/app/(protected)/settings/invoicing/_components/invoicing-switch-card', () => ({
  InvoicingSwitchCard: ({ enabled }: { enabled: boolean }) => (
    <div data-testid="switch" data-enabled={String(enabled)} />
  ),
}));
vi.mock('@/app/(protected)/settings/invoicing/_components/arca-connection-card', () => ({
  ARCAConnectionCard: () => <div data-testid="connection" />,
}));
vi.mock('@/app/(protected)/settings/invoicing/_components/invoicing-settings-form', () => ({
  InvoicingSettingsForm: () => <div data-testid="settings-form" />,
}));

const { getAccount } = await import('@/lib/api/account');
const { getARCASetup } = await import('@/lib/api/arca-setup');
const { getInvoicingSettings } = await import('@/lib/api/invoicing-settings');
const { default: InvoicingSettingsPage } = await import('./page');

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getARCASetup).mockResolvedValue({} as never);
  vi.mocked(getInvoicingSettings).mockResolvedValue({ branches: [], taxId: null } as never);
});

describe('InvoicingSettingsPage', () => {
  // Off, the page is the switch alone, and the connection and fiscal data are not even read.
  it('shows only the switch while invoicing is off', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: false } as never);

    const view = render(await InvoicingSettingsPage());

    expect(view.getByTestId('switch').dataset.enabled).toBe('false');
    expect(view.queryByTestId('connection')).toBeNull();
    expect(view.queryByTestId('settings-form')).toBeNull();
    expect(getInvoicingSettings).not.toHaveBeenCalled();
    expect(getARCASetup).not.toHaveBeenCalled();
  });

  it('adds the connection and the settings while invoicing is on', async () => {
    vi.mocked(getAccount).mockResolvedValue({ invoicingEnabled: true } as never);

    const view = render(await InvoicingSettingsPage());

    expect(view.getByTestId('switch').dataset.enabled).toBe('true');
    expect(view.getByTestId('connection')).toBeTruthy();
    expect(view.getByTestId('settings-form')).toBeTruthy();
  });
});
