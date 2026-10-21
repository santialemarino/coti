import { afterEach, describe, expect, it, vi } from 'vitest';

import { fetchQueue, sendQuote } from '@/lib/api/rfqs-client';

afterEach(() => vi.unstubAllGlobals());

function answer(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(body), { status })),
  );
}

describe('fetchQueue', () => {
  it('reads the queue through the BFF and maps each row', async () => {
    answer(200, [
      {
        id: 'r1',
        quote_number: 3,
        client: null,
        created_at: '2026-10-01T12:00:00.000Z',
        channel: 'email',
        seller_id: null,
        seller: '',
        branch: 'Morón',
        branch_id: 'b1',
        quote_id: null,
        item_count: 0,
        review_count: 0,
        total: null,
        status: 'DRAFT',
        needs_followup: false,
        followup_flagged_at: null,
        archived_at: '2026-10-02T12:00:00.000Z',
      },
    ]);

    const [record] = await fetchQueue();

    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe('/api/rfqs');
    expect(record).toMatchObject({ id: 'r1', client: '', status: 'GENERATED', archived: true });
  });

  it('asks for the archived orders only when told to', async () => {
    answer(200, []);

    await fetchQueue(true);

    expect(vi.mocked(fetch).mock.calls[0]?.[0]).toBe('/api/rfqs?include_archived=true');
  });

  // The column keeps the last good list on a failure, which only works if a failure is not an
  // empty queue.
  it('throws on a refused read instead of answering an empty queue', async () => {
    answer(500, {});

    await expect(fetchQueue()).rejects.toThrow();
  });
});

describe('sendQuote client association boundary', () => {
  it('maps the reviewed profile selection to the send API contract', async () => {
    answer(200, {});
    await sendQuote('quote-1', 'branch-1', {
      recipient_phone: '+5491155550101',
      clientAssociation: {
        taxId: '20123456786',
        newClient: { name: 'Example customer', email: 'customer@example.test' },
        tagIds: ['tag-1'],
      },
    });
    const [url, options] = vi.mocked(fetch).mock.calls[0]!;
    expect(url).toBe('/api/quotes/quote-1/sends');
    expect(JSON.parse(options?.body as string)).toEqual({
      recipient_phone: '+5491155550101',
      client_association: {
        tax_id: '20123456786',
        new_client: { name: 'Example customer', email: 'customer@example.test' },
        tag_ids: ['tag-1'],
      },
    });
    expect(new Headers(options?.headers).get('X-Branch-Id')).toBe('branch-1');
    expect(new Headers(options?.headers).get('Idempotency-Key')).toBeTruthy();
  });
});
