import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { checkPortal, loadPortal, savePortal } from '@/components/customer-portal/actions';
import { CustomerPortalEditor } from '@/components/customer-portal/customer-portal-editor';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock('@/components/customer-portal/actions', () => ({
  loadPortal: vi.fn(),
  checkPortal: vi.fn(),
  savePortal: vi.fn(),
}));
vi.mock('@/lib/config', () => ({ PORTAL_AVAILABILITY_DELAY_SECONDS: 0 }));

const portal = {
  baseUrl: 'http://localhost:3001',
  hostname: '',
  url: '',
  subdomain: '',
  legacy: false,
};
const copy = messages.customerPortal;
function mount() {
  return render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <CustomerPortalEditor />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(loadPortal).mockResolvedValue({ data: portal });
  vi.mocked(checkPortal).mockResolvedValue({ available: true });
  vi.mocked(savePortal).mockResolvedValue({ ok: true });
});

describe('CustomerPortalEditor', () => {
  it('previews the environment origin and preserves input after a save collision', async () => {
    vi.mocked(savePortal).mockResolvedValue({ error: 'CONFLICT' });
    const view = mount();
    const input = await view.findByLabelText(copy.label, { exact: false });
    fireEvent.change(input, { target: { value: 'Marko' } });
    expect(view.getByText('http://marko.localhost:3001')).toBeTruthy();
    await view.findByText(copy.available);
    fireEvent.click(view.getByRole('button', { name: copy.save }));
    await waitFor(() => expect(savePortal).toHaveBeenCalledWith('marko', '', false));
    await view.findByText(copy.conflict);
    expect((input as HTMLInputElement).value).toBe('Marko');
  });

  it('ignores availability results from an older edit', async () => {
    let resolveOld!: (result: { available: boolean }) => void;
    vi.mocked(checkPortal).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    );
    const view = mount();
    const input = await view.findByLabelText(copy.label, { exact: false });
    fireEvent.change(input, { target: { value: 'primero' } });
    await waitFor(() => expect(checkPortal).toHaveBeenCalledWith('primero'));
    fireEvent.change(input, { target: { value: 'segundo' } });
    await view.findByText(copy.available);
    resolveOld({ available: false });
    await waitFor(() => expect(view.queryByText(copy.taken)).toBeNull());
  });

  it('requires explicit confirmation before changing existing access', async () => {
    vi.mocked(loadPortal).mockResolvedValue({
      data: {
        ...portal,
        hostname: 'old.localhost',
        subdomain: 'old',
        url: 'http://old.localhost:3001',
      },
    });
    const view = mount();
    const input = await view.findByLabelText(copy.label, { exact: false });
    fireEvent.change(input, { target: { value: 'nuevo' } });
    await view.findByText(copy.available);
    fireEvent.click(view.getByRole('button', { name: copy.save }));
    await view.findByText(copy.changeWarning);
    expect(savePortal).not.toHaveBeenCalled();
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));
    await waitFor(() => expect(savePortal).toHaveBeenCalledWith('nuevo', 'old.localhost', true));
  });

  it('preserves an inherited hostname and explains why it cannot be changed', async () => {
    vi.mocked(loadPortal).mockResolvedValue({
      data: {
        ...portal,
        hostname: 'temporary.host.test',
        url: 'https://temporary.host.test',
        legacy: true,
      },
    });
    const view = mount();
    await view.findByText(copy.legacy);
    expect(view.getByRole('link').getAttribute('href')).toBe('https://temporary.host.test');
    expect(view.queryByRole('textbox')).toBeNull();
    expect(view.queryByRole('button', { name: copy.save })).toBeNull();
  });
});
