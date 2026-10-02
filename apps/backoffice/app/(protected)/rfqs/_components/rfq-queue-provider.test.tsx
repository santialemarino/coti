import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import type { RfqListItem } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';
import { mapListItem, RfqQueueProvider } from './rfq-queue-provider';

vi.mock('@/lib/api/client', () => ({ apiRequest: vi.fn(async () => []) }));
vi.mock('@/lib/api/branches', () => ({ getBranches: vi.fn() }));
vi.mock('@/lib/auth/branch', () => ({ getEffectiveBranchId: vi.fn(async () => undefined) }));
vi.mock('@/lib/auth/session', () => ({ getSession: vi.fn() }));

const { getBranches } = await import('@/lib/api/branches');
const { getSession } = await import('@/lib/auth/session');

const RAW: RfqListItem = {
  id: '10000000-0000-4000-8000-000000000001',
  quote_number: 12,
  client: null,
  created_at: '2026-09-20T12:00:00.000Z',
  channel: 'whatsapp',
  seller_id: null,
  seller: '',
  branch: 'Villa Bosch',
  branch_id: 'b0000000-0000-4000-8000-000000000001',
  quote_id: '20000000-0000-4000-8000-000000000001',
  item_count: 3,
  review_count: 1,
  total: null,
  status: 'SENT',
  needs_followup: true,
  followup_flagged_at: '2026-09-27T09:00:00.000Z',
  archived_at: null,
};

describe('mapListItem', () => {
  it('maps every field of the list row, the follow-up date included', () => {
    expect(mapListItem(RAW)).toEqual({
      id: RAW.id,
      quoteNumber: 12,
      client: '',
      createdAt: RAW.created_at,
      channel: 'whatsapp',
      seller: '',
      sellerId: null,
      branch: 'Villa Bosch',
      branchId: RAW.branch_id,
      quoteId: RAW.quote_id,
      itemCount: 3,
      reviewCount: 1,
      total: undefined,
      status: 'SENT',
      needsFollowup: true,
      followupFlaggedAt: '2026-09-27T09:00:00.000Z',
      archived: false,
    });
  });
});

describe('RfqQueueProvider', () => {
  async function renderQueue(role: string, branches: unknown[]) {
    vi.mocked(getSession).mockResolvedValue({ role, name: 'Ana', userId: 'u1' } as never);
    vi.mocked(getBranches).mockResolvedValue(branches as never);
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        {await RfqQueueProvider({ children: <p>cola</p> })}
      </NextIntlClientProvider>,
    );
  }

  // Each queue screen would otherwise fail its own way for a seller nobody assigned anywhere.
  it('tells a seller with no branch why there is no queue', async () => {
    await renderQueue('SELLER', []);

    expect(screen.getByText(messages.home.noBranch.title)).toBeTruthy();
    expect(screen.queryByText('cola')).toBeNull();
  });

  it('renders the queue for a seller with a branch', async () => {
    await renderQueue('SELLER', [{ id: 'b1' }]);

    expect(screen.getByText('cola')).toBeTruthy();
  });
});
