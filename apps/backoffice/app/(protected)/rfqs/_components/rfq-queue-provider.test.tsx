import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { RfqQueueGate } from '@/app/(protected)/rfqs/_components/rfq-queue-gate';
import messages from '@/translations/es.json';
import { RfqQueueProvider } from './rfq-queue-provider';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn(async () => []) }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getEffectiveBranchId: vi.fn(async () => undefined) }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));

const { apiRequest } = await import('@/lib/api/client');
const { getBranches } = await import('@/lib/api/branches');
const { getSession } = await import('@/lib/auth/session');

describe('RfqQueueProvider', () => {
  async function renderShell(role: string, branches: unknown[]) {
    vi.mocked(getSession).mockResolvedValue({ role, name: 'Ana', userId: 'u1' } as never);
    vi.mocked(getBranches).mockResolvedValue(branches as never);
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        {await RfqQueueProvider({
          children: (
            <>
              <RfqQueueGate>
                <p>cola</p>
              </RfqQueueGate>
              <p>ajustes</p>
            </>
          ),
        })}
      </NextIntlClientProvider>,
    );
  }

  // Each queue screen would otherwise fail its own way for a seller nobody assigned anywhere.
  it('tells a seller with no branch why there is no queue, without asking for one', async () => {
    vi.mocked(apiRequest).mockClear();
    await renderShell('SELLER', []);

    expect(screen.getByText(messages.home.noBranch.title)).toBeTruthy();
    expect(screen.queryByText('cola')).toBeNull();
    expect(apiRequest).not.toHaveBeenCalled();
  });

  // The provider wraps the whole shell, so it must not take the seller's own settings with it.
  it('leaves the screens outside the queue to a seller with no branch', async () => {
    await renderShell('SELLER', []);

    expect(screen.getByText('ajustes')).toBeTruthy();
  });

  it('renders the queue for a seller with a branch', async () => {
    await renderShell('SELLER', [{ id: 'b1' }]);

    expect(screen.getByText('cola')).toBeTruthy();
  });

  // A failed read fails the queue screens alone; the rest of the shell must still render.
  it('fails only the queue screens when the queue cannot be read', async () => {
    vi.mocked(getSession).mockResolvedValue({ role: 'ADMIN', name: 'Ana', userId: 'u1' } as never);
    vi.mocked(getBranches).mockResolvedValue([{ id: 'b1' }] as never);
    vi.mocked(apiRequest).mockRejectedValue(new Error('down'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const shell = (children: React.ReactNode) => (
      <NextIntlClientProvider locale="es" messages={messages}>
        {children}
      </NextIntlClientProvider>
    );

    render(shell(await RfqQueueProvider({ children: <p>ajustes</p> })));
    expect(screen.getByText('ajustes')).toBeTruthy();

    const queue = await RfqQueueProvider({ children: <RfqQueueGate>cola</RfqQueueGate> });
    expect(() => render(shell(queue))).toThrow('could not be read');
    vi.mocked(apiRequest).mockReset();
  });
});
