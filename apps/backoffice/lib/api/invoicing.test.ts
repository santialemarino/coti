import { describe, expect, it } from 'vitest';

import {
  formatCuit,
  formatInvoiceNumber,
  mapClientFiscal,
  mapInvoicePreview,
  mapInvoicingSettings,
  vatRateOf,
} from '@/lib/api/invoicing';

const RECEIVER = {
  name: 'Constructora Horizonte SA',
  doc_type: 'CUIT',
  doc_number: '30701234568',
  iva_condition: 'REGISTERED',
};

const AMOUNTS = {
  net: '1000.00',
  exempt: '0.00',
  vat: '210.00',
  total: '1210.00',
  by_rate: [{ rate: 'VAT_21', base: '1000.00', amount: '210.00' }],
};

describe('invoicing API mapping', () => {
  it('maps every settings field from snake_case', () => {
    expect(
      mapInvoicingSettings({
        enabled: true,
        environment: 'production',
        legal_name: 'Corralón del Sur SRL',
        tax_id: '30711111112',
        iva_condition: 'REGISTERED',
        prices_include_vat: true,
        branches: [{ branch_id: 'b1', name: 'Centro', is_active: false, point_of_sale: 3 }],
        credential: {
          cuit: '30711111112',
          subject: 'SERIALNUMBER=CUIT 30711111112, CN=coti',
          expires_at: '2028-01-01T00:00:00Z',
          updated_at: '2026-10-01T12:00:00Z',
        },
      }),
    ).toEqual({
      enabled: true,
      homologation: false,
      legalName: 'Corralón del Sur SRL',
      taxId: '30711111112',
      ivaCondition: 'REGISTERED',
      pricesIncludeVat: true,
      branches: [{ branchId: 'b1', name: 'Centro', isActive: false, pointOfSale: 3 }],
      credential: {
        cuit: '30711111112',
        subject: 'SERIALNUMBER=CUIT 30711111112, CN=coti',
        expiresAt: '2028-01-01T00:00:00Z',
        updatedAt: '2026-10-01T12:00:00Z',
      },
    });
  });

  it('reads homologation, nulls and an unknown condition as unset', () => {
    const settings = mapInvoicingSettings({
      enabled: false,
      environment: 'homologation',
      legal_name: null,
      tax_id: null,
      iva_condition: 'SOMETHING_NEW',
      prices_include_vat: false,
      branches: null,
      credential: null,
    });

    expect(settings.homologation).toBe(true);
    expect(settings.ivaCondition).toBeNull();
    expect(settings.branches).toEqual([]);
    expect(settings.credential).toBeNull();
  });

  it('maps a preview and the invoice it already has', () => {
    expect(
      mapInvoicePreview({
        version_id: 'v-1',
        type: 'A',
        point_of_sale: 3,
        receiver: RECEIVER,
        amounts: AMOUNTS,
        currency: 'ARS',
        issues: null,
        invoice: {
          id: 'inv-1',
          status: 'ISSUED',
          type: 'A',
          point_of_sale: 3,
          number: 23,
          issued_on: '2026-10-05',
          cae: '76543210987654',
          cae_expires_on: '2026-10-15',
          issuer_cuit: '30711111112',
          receiver: RECEIVER,
          amounts: { ...AMOUNTS, by_rate: null },
          currency: 'ARS',
          issues: null,
          qr_url: 'https://www.afip.gob.ar/fe/qr/?p=abc',
          created_at: '2026-10-05T15:00:00Z',
        },
      }),
    ).toEqual({
      versionId: 'v-1',
      type: 'A',
      pointOfSale: 3,
      receiver: {
        name: 'Constructora Horizonte SA',
        docType: 'CUIT',
        docNumber: '30701234568',
        ivaCondition: 'REGISTERED',
      },
      amounts: {
        net: '1000.00',
        exempt: '0.00',
        vat: '210.00',
        total: '1210.00',
        byRate: [{ rate: 'VAT_21', base: '1000.00', amount: '210.00' }],
      },
      currency: 'ARS',
      issues: [],
      invoice: {
        id: 'inv-1',
        status: 'ISSUED',
        type: 'A',
        pointOfSale: 3,
        number: 23,
        issuedOn: '2026-10-05',
        cae: '76543210987654',
        caeExpiresOn: '2026-10-15',
        issuerCuit: '30711111112',
        receiver: {
          name: 'Constructora Horizonte SA',
          docType: 'CUIT',
          docNumber: '30701234568',
          ivaCondition: 'REGISTERED',
        },
        amounts: { net: '1000.00', exempt: '0.00', vat: '210.00', total: '1210.00', byRate: [] },
        currency: 'ARS',
        issues: [],
        qrUrl: 'https://www.afip.gob.ar/fe/qr/?p=abc',
        createdAt: '2026-10-05T15:00:00Z',
      },
    });
  });

  it('maps a client fiscal identity', () => {
    expect(
      mapClientFiscal({
        id: 'c1',
        name: 'Horizonte',
        legal_name: 'Constructora Horizonte SA',
        tax_id: '30701234568',
        iva_condition: 'MONOTRIBUTO',
      }),
    ).toEqual({
      id: 'c1',
      name: 'Horizonte',
      legalName: 'Constructora Horizonte SA',
      taxId: '30701234568',
      ivaCondition: 'MONOTRIBUTO',
    });
  });
});

describe('formatInvoiceNumber', () => {
  it('pads the point of sale to four digits and the number to eight', () => {
    expect(formatInvoiceNumber(3, 23)).toBe('0003-00000023');
  });

  it('widens the point of sale to five digits once it outgrows four', () => {
    expect(formatInvoiceNumber(9999, 1)).toBe('9999-00000001');
    expect(formatInvoiceNumber(12345, 12345678)).toBe('12345-12345678');
  });
});

describe('formatCuit', () => {
  it('writes eleven digits with the two hyphens', () => {
    expect(formatCuit('30701234568')).toBe('30-70123456-8');
  });

  it('leaves anything else as it came', () => {
    expect(formatCuit('28123456')).toBe('28123456');
  });
});

describe('vatRateOf', () => {
  it('keeps a known rate and defaults anything else to 21 %', () => {
    expect(vatRateOf('VAT_10_5')).toBe('VAT_10_5');
    expect(vatRateOf('EXEMPT')).toBe('EXEMPT');
    expect(vatRateOf(undefined)).toBe('VAT_21');
    expect(vatRateOf('VAT_99')).toBe('VAT_21');
  });
});
