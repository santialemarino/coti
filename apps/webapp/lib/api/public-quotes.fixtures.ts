import type { PublicQuoteSend, QuotePayload } from '@/lib/api/public-quotes';

export const EXPIRES_AT = '2026-09-22T12:00:00Z';
export const PDF_URL = 'https://files.example.test/cot-42.pdf?sig=abc';

/*
 * A quote as the route hands it to its components, shared by the four specs so the values a page
 * test asserts against are the same ones the pieces were rendered with.
 */
export function makeQuotePayload(overrides: Partial<QuotePayload> = {}): QuotePayload {
  return {
    reference: 'COT-000042',
    versionNumber: 2,
    approvedAt: '2026-09-15T12:00:00Z',
    currency: 'ARS',
    supplier: {
      name: 'Corralón Centro',
      legalName: 'Corralón Centro SRL',
      taxId: '30-70123456-8',
      brandColor: '#C2410C',
      logoUrl: 'https://cdn.example.test/corralon-centro.png',
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
      {
        requestedDescription: 'diez kilos de cal',
        productName: 'Cal común',
        quantity: '10',
        unitPrice: '15.00',
        subtotal: '150.00',
        alternatives: [],
      },
    ],
    discounts: [{ description: 'Bonificación', amount: '20.00' }],
    total: '350.00',
    validityNote: 'Precios sujetos a disponibilidad.',
    ...overrides,
  };
}

export function makePublicQuoteSend(overrides: Partial<PublicQuoteSend> = {}): PublicQuoteSend {
  return {
    status: 'ACTIVE',
    expiresAt: EXPIRES_AT,
    quote: makeQuotePayload(),
    pdfUrl: PDF_URL,
    ...overrides,
  };
}

export const PREVIEW_PDF_URL = 'https://files.example.test/cot-000008.pdf?sig=dev';

const DAY_MS = 86_400_000;

const dateFromNow = (days: number) => new Date(Date.now() + days * DAY_MS).toISOString();

/*
 * The dev-only /quotes/preview route feeds PublicQuoteView a SENT quote frozen in the send's shape,
 * so the developer inspects the same representation a client link renders. It is a fixture, not a
 * wire read: the dates are relative to now — approved two days ago, valid for thirty more — so the
 * preview is always open, unlike the seeded send token, whose expiry was frozen when the seed ran.
 */
export function makeSentPreviewSend(): PublicQuoteSend {
  return makePublicQuoteSend({
    expiresAt: dateFromNow(30),
    pdfUrl: PREVIEW_PDF_URL,
    quote: makeQuotePayload({
      reference: 'COT-000008',
      versionNumber: 1,
      approvedAt: dateFromNow(-2),
      currency: 'ARS',
      supplier: {
        name: 'Corralón San Martín',
        legalName: 'Corralón San Martín S.R.L.',
        taxId: '30-71234567-9',
        brandColor: '#C2410C',
      },
      branch: { name: 'Morón', address: 'Rivadavia 18400, Morón' },
      customerName: 'Constructora del Riachuelo',
      items: [
        {
          requestedDescription: '80 bolsas de cemento portland',
          productCode: 'CEM-01',
          productName: 'Cemento Portland CP40 50 kg',
          quantity: '80.00',
          unit: 'bolsa',
          unitPrice: '9500.00',
          subtotal: '760000.00',
          alternatives: [
            {
              code: 'CEM-02',
              name: 'Cemento Portland CP40 25 kg',
              unit: 'bolsa',
              unitPrice: '5100.00',
            },
          ],
        },
        {
          requestedDescription: '12 m3 de arena fina',
          productCode: 'ARE-FIN-01',
          productName: 'Arena fina lavada',
          quantity: '12.00',
          unit: 'm3',
          unitPrice: '32000.00',
          subtotal: '384000.00',
          alternatives: [
            { code: 'ARE-GRU-01', name: 'Arena gruesa', unit: 'm3', unitPrice: '31000.00' },
          ],
        },
        {
          requestedDescription: '10 hierros del 8',
          productCode: 'HIER-08',
          productName: 'Hierro ADN 420 8 mm',
          quantity: '10.00',
          unit: 'barra',
          unitPrice: '4800.00',
          subtotal: '48000.00',
          alternatives: [],
        },
        {
          requestedDescription: '3 rollos de membrana asfáltica de 4 mm',
          productCode: 'MEMB-ASF-04',
          productName: 'Membrana asfáltica 4 mm',
          quantity: '3.00',
          unit: 'rollo',
          unitPrice: '47000.00',
          subtotal: '141000.00',
          alternatives: [],
        },
        {
          requestedDescription: '2 baldes de hidrófugo',
          productCode: 'HIDR-01',
          productName: 'Hidrófugo para mezclas',
          quantity: '2.00',
          unit: 'balde',
          unitPrice: '18900.00',
          subtotal: '37800.00',
          alternatives: [],
        },
      ],
      discounts: [{ description: 'Bonificación por pronto pago', amount: '30800.00' }],
      total: '1340000.00',
      validityNote: 'Precios sujetos a disponibilidad.',
    }),
  });
}
