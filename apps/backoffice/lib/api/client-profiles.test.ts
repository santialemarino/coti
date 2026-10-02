import { describe, expect, it } from 'vitest';

import {
  clientMatchFromSummary,
  mapClientProfile,
  mapClientSummary,
  mapQuoteClientAssociation,
} from '@/lib/api/client-profiles';

const CLIENT = {
  id: 'client-1',
  name: 'Constructora Horizonte',
  phone: '+5491155550101',
  email: 'compras@horizonte.test',
  origin_channel: 'WHATSAPP',
  notes: null,
  created_at: '2026-09-20T12:00:00Z',
  updated_at: '2026-09-21T12:00:00Z',
};

const TAG = {
  id: 'tag-1',
  name: 'Recurrente',
  created_at: '2026-09-20T12:00:00Z',
};

describe('client profile API mapping', () => {
  it('maps every client summary field from snake_case', () => {
    expect(
      mapClientSummary({
        ...CLIENT,
        tags: [TAG],
        accepted_quote_count: 3,
        last_accepted_at: '2026-09-22T12:00:00Z',
      }),
    ).toEqual({
      id: 'client-1',
      name: 'Constructora Horizonte',
      phone: '+5491155550101',
      email: 'compras@horizonte.test',
      originChannel: 'WHATSAPP',
      notes: null,
      createdAt: '2026-09-20T12:00:00Z',
      updatedAt: '2026-09-21T12:00:00Z',
      tags: [{ id: 'tag-1', name: 'Recurrente', createdAt: '2026-09-20T12:00:00Z' }],
      acceptedQuoteCount: 3,
      lastAcceptedAt: '2026-09-22T12:00:00Z',
    });
  });

  it('turns a directory summary into an association candidate', () => {
    const summary = mapClientSummary({
      ...CLIENT,
      tags: [TAG],
      accepted_quote_count: 3,
      last_accepted_at: '2026-09-22T12:00:00Z',
    });

    expect(clientMatchFromSummary(summary)).toEqual({
      client: {
        id: CLIENT.id,
        name: CLIENT.name,
        phone: CLIENT.phone,
        email: CLIENT.email,
        originChannel: CLIENT.origin_channel,
        notes: CLIENT.notes,
        createdAt: CLIENT.created_at,
        updatedAt: CLIENT.updated_at,
      },
      tags: [{ id: 'tag-1', name: 'Recurrente', createdAt: '2026-09-20T12:00:00Z' }],
    });
  });

  it('maps profile sale identifiers and decimal totals without coercion', () => {
    expect(
      mapClientProfile({
        client: CLIENT,
        tags: [TAG],
        sales: [
          {
            quote_id: 'quote-1',
            rfq_id: 'rfq-1',
            quote_number: 42,
            branch_id: 'branch-1',
            branch_name: 'Morón',
            total: '145184.00',
            accepted_at: '2026-09-22T12:00:00Z',
          },
        ],
      }).sales[0],
    ).toEqual({
      quoteId: 'quote-1',
      rfqId: 'rfq-1',
      quoteNumber: 42,
      branchId: 'branch-1',
      branchName: 'Morón',
      total: '145184.00',
      acceptedAt: '2026-09-22T12:00:00Z',
    });
  });

  it('keeps suggestions distinct from the confirmed client', () => {
    const mapped = mapQuoteClientAssociation({
      current_client: null,
      suggestions: [{ client: CLIENT, tags: [TAG] }],
      contact_hints: { phone: CLIENT.phone, email: CLIENT.email },
      available_tags: [TAG],
    });

    expect(mapped.currentClient).toBeNull();
    expect(mapped.suggestions).toHaveLength(1);
    expect(mapped.suggestions[0]?.client.id).toBe(CLIENT.id);
    expect(mapped.contactHints).toEqual({ phone: CLIENT.phone, email: CLIENT.email });
  });
});
