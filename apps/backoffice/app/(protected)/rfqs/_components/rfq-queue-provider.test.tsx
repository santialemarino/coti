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

  // The provider wraps the whole shell now, so it must not take the seller's own settings with it.
  it('leaves the screens outside the queue to a seller with no branch', async () => {
    await renderShell('SELLER', []);

    expect(screen.getByText('ajustes')).toBeTruthy();
  });

  it('renders the queue for a seller with a branch', async () => {
    await renderShell('SELLER', [{ id: 'b1' }]);

    expect(screen.getByText('cola')).toBeTruthy();
  });
});
