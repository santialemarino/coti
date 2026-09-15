import { afterEach, describe, expect, it, vi } from 'vitest';

import { getPublicQuoteByToken, type PublicQuoteSend } from '@/lib/api/public-quotes';
import { API_URL } from '@/lib/config';

function rawSend(overrides: Record<string, unknown> = {}) {
  return {
    status: 'ACTIVE',
    expires_at: '2026-09-22T12:00:00Z',
    message: 'Hola, Obra Norte.',
    pdf_url: 'https://files.example.test/cot-42.pdf?sig=abc',
    quote: {
      reference: 'COT-000042',
      version_number: 2,
      approved_at: '2026-09-15T12:00:00Z',
      currency: 'ARS',
      supplier: {
        name: 'Corralón Centro',
        legal_name: 'Corralón Centro SRL',
        tax_id: '30-70123456-8',
        brand_color: '#C2410C',
        logo_url: '/v1/public/account-logos/acc/logo',
      },
      branch: { name: 'Casa central', address: 'Av. Siempreviva 742' },
      customer: { name: 'Obra Norte' },
      items: [
        {
          requested_description: 'dos bolsas de cemento',
          product_code: 'CEM-01',
          product_name: 'Cemento Portland 50kg',
          quantity: '2.00',
          unit: 'bolsa',
          unit_price: '110.00',
          subtotal: '220.00',
          alternatives: [
            { code: 'CEM-02', name: 'Cemento Portland 25kg', unit: 'bolsa', unit_price: '60.00' },
          ],
        },
      ],
      discounts: [{ description: 'Bonificación', amount: '20.00' }],
      total: '200.00',
      validity_note: 'Precios sujetos a disponibilidad.',
    },
    ...overrides,
  };
}

function stubFetch(status: number, body?: unknown) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getPublicQuoteByToken', () => {
  /*
   * The whole mapped object, so a field added to the raw type and forgotten in a mapper fails
   * here rather than surfacing as `undefined` in the middle of the customer's quote.
   */
  it('maps every field of an active send', async () => {
    stubFetch(200, rawSend());

    const expected: PublicQuoteSend = {
      status: 'ACTIVE',
      expiresAt: '2026-09-22T12:00:00Z',
      message: 'Hola, Obra Norte.',
      pdfUrl: 'https://files.example.test/cot-42.pdf?sig=abc',
      quote: {
        reference: 'COT-000042',
        versionNumber: 2,
        approvedAt: '2026-09-15T12:00:00Z',
        currency: 'ARS',
        supplier: {
          name: 'Corralón Centro',
          legalName: 'Corralón Centro SRL',
          taxId: '30-70123456-8',
          brandColor: '#C2410C',
          logoUrl: `${API_URL}/v1/public/account-logos/acc/logo`,
        },
        branch: { name: 'Casa central', address: 'Av. Siempreviva 742' },
        customerName: 'Obra Norte',
        items: [
          {
            requestedDescription: 'dos bolsas de cemento',
            productCode: 'CEM-01',
            productName: 'Cemento Portland 50kg',
            quantity: '2.00',
            unit: 'bolsa',
            unitPrice: '110.00',
            subtotal: '220.00',
            alternatives: [
              { code: 'CEM-02', name: 'Cemento Portland 25kg', unit: 'bolsa', unitPrice: '60.00' },
            ],
          },
        ],
        discounts: [{ description: 'Bonificación', amount: '20.00' }],
        total: '200.00',
        validityNote: 'Precios sujetos a disponibilidad.',
      },
    };

    await expect(getPublicQuoteByToken('tok-1')).resolves.toEqual(expected);
  });

  it('sends the token url-encoded and attaches no credential', async () => {
    const fetchMock = stubFetch(200, rawSend());
    await getPublicQuoteByToken('tok/with space');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`${API_URL}/v1/public/quote-sends/tok%2Fwith%20space`);
    expect(init.headers).toEqual({ Accept: 'application/json' });
    expect(init).not.toHaveProperty('credentials');
  });

  /*
   * An uploaded logo is stored as the API's own path and needs its origin; a pasted one is already
   * absolute. Getting this backwards is a broken image on the one screen the corralón's brand is
   * the point of.
   */
  it('resolves a stored logo path against the API and leaves an absolute URL alone', async () => {
    stubFetch(200, rawSend({ quote: { ...rawSend().quote, supplier: supplier('/logos/a.png') } }));
    expect(logoOf(await getPublicQuoteByToken('tok-1'))).toBe(`${API_URL}/logos/a.png`);

    stubFetch(
      200,
      rawSend({ quote: { ...rawSend().quote, supplier: supplier('https://cdn.test/a.png') } }),
    );
    expect(logoOf(await getPublicQuoteByToken('tok-1'))).toBe('https://cdn.test/a.png');

    stubFetch(200, rawSend({ quote: { ...rawSend().quote, supplier: supplier(undefined) } }));
    expect(logoOf(await getPublicQuoteByToken('tok-1'))).toBeUndefined();
  });

  it('maps an expired send, which carries no quote', async () => {
    stubFetch(200, { status: 'EXPIRED', expires_at: '2026-09-01T12:00:00Z' });

    await expect(getPublicQuoteByToken('tok-1')).resolves.toEqual({
      status: 'EXPIRED',
      expiresAt: '2026-09-01T12:00:00Z',
      quote: undefined,
      message: undefined,
      pdfUrl: undefined,
    });
  });

  // A status this app cannot word must not fall through to rendering a quote as if it were live.
  it('treats an unknown status as expired', async () => {
    stubFetch(200, rawSend({ status: 'REVOKED' }));
    await expect(getPublicQuoteByToken('tok-1')).resolves.toMatchObject({ status: 'EXPIRED' });
  });

  it('returns null for a token the API does not know', async () => {
    stubFetch(404);
    await expect(getPublicQuoteByToken('nope')).resolves.toBeNull();
  });

  it('throws on any other refusal, so the page does not render an empty quote', async () => {
    stubFetch(500);
    await expect(getPublicQuoteByToken('tok-1')).rejects.toThrow(/500/);
  });
});

function supplier(logoUrl: string | undefined) {
  return { name: 'Corralón Centro', brand_color: '#C2410C', logo_url: logoUrl };
}

function logoOf(send: PublicQuoteSend | null) {
  return send?.quote?.supplier.logoUrl;
}
