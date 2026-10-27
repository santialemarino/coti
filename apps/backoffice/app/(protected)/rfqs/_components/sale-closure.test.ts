import { describe, expect, it } from 'vitest';

import { closingActions, saleNextStep } from '@/app/(protected)/rfqs/_components/sale-closure';
import type { RfqRecord } from '@/lib/api/rfqs';

const AT = '2026-10-10T12:00:00.000Z';

type Facts = Parameters<typeof saleNextStep>[0];

const OPEN_SALE: Facts = {
  status: 'ACCEPTED',
  archived: false,
  quoteId: 'q1',
  paidAt: null,
  invoiceWaivedAt: null,
  invoiced: false,
  invoicePending: false,
  saleComplete: false,
};

function sale(facts: Partial<RfqRecord>): Facts {
  return { ...OPEN_SALE, ...facts };
}

describe('saleNextStep', () => {
  it.each([
    ['unpaid, invoicing on', sale({}), true, 'MARK_PAID'],
    ['unpaid, invoicing off', sale({}), false, 'MARK_PAID'],
    // An invoice on account still waits for its payment.
    ['invoiced, unpaid', sale({ invoiced: true }), true, 'MARK_PAID'],
    ['paid, no invoice', sale({ paidAt: AT }), true, 'INVOICE'],
    ['paid, invoice pending in ARCA', sale({ paidAt: AT, invoicePending: true }), true, null],
    ['paid, waived', sale({ paidAt: AT, invoiceWaivedAt: AT, saleComplete: true }), true, null],
    ['paid, invoicing off', sale({ paidAt: AT, saleComplete: true }), false, null],
    ['paid and invoiced', sale({ paidAt: AT, invoiced: true, saleComplete: true }), true, null],
    ['archived', sale({ archived: true }), true, null],
    ['no quote', sale({ quoteId: null }), true, null],
    ['sent', sale({ status: 'SENT' }), true, null],
    ['rejected', sale({ status: 'REJECTED' }), true, null],
  ] as const)('%s → %s', (_, record, invoicingEnabled, step) => {
    expect(saleNextStep(record, invoicingEnabled)).toBe(step);
  });

  // The flag the API derives wins over a partial reading of the facts.
  it('offers nothing once the API says the sale is complete', () => {
    expect(saleNextStep(sale({ saleComplete: true }), true)).toBeNull();
  });
});

describe('closingActions', () => {
  it.each([
    ['unpaid, invoicing on', sale({}), true, ['MARK_PAID', 'INVOICE', 'WAIVE']],
    ['unpaid, invoicing off', sale({}), false, ['MARK_PAID']],
    ['paid, no invoice', sale({ paidAt: AT }), true, ['UNMARK_PAID', 'INVOICE', 'WAIVE']],
    ['paid, waived', sale({ paidAt: AT, invoiceWaivedAt: AT }), true, ['UNMARK_PAID', 'UNWAIVE']],
    ['waived, invoicing off', sale({ invoiceWaivedAt: AT }), false, ['MARK_PAID']],
    ['invoice pending', sale({ paidAt: AT, invoicePending: true }), true, ['UNMARK_PAID']],
    ['invoiced', sale({ paidAt: AT, invoiced: true }), true, ['UNMARK_PAID']],
    ['archived', sale({ archived: true }), true, []],
    ['quoted', sale({ status: 'QUOTED' }), true, []],
  ] as const)('%s → %j', (_, record, invoicingEnabled, actions) => {
    expect(closingActions(record, invoicingEnabled)).toEqual(actions);
  });
});
