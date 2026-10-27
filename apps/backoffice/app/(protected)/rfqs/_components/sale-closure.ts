import type { RfqRecord } from '@/lib/api/rfqs';

// What closes an accepted sale from the list: the payment mark, the invoice, and the waiver.
export type ClosingAction = 'MARK_PAID' | 'UNMARK_PAID' | 'INVOICE' | 'WAIVE' | 'UNWAIVE';

// The step that closes the sale next, the one a queue card offers.
export type SaleNextStep = Extract<ClosingAction, 'MARK_PAID' | 'INVOICE'>;

type ClosureFacts = Pick<
  RfqRecord,
  | 'status'
  | 'archived'
  | 'quoteId'
  | 'paidAt'
  | 'invoiceWaivedAt'
  | 'invoiced'
  | 'invoicePending'
  | 'saleComplete'
>;

// An accepted sale the seller can still act on: not archived, and with a quote to write to.
function closable(record: ClosureFacts): boolean {
  return record.status === 'ACCEPTED' && !record.archived && record.quoteId !== null;
}

// The sale still waits for an invoice nobody has issued, asked ARCA for, or waived.
function awaitsInvoice(record: ClosureFacts, invoicingEnabled: boolean): boolean {
  return invoicingEnabled && !record.invoiced && !record.invoicePending && !record.invoiceWaivedAt;
}

/*
 * The next step of an accepted sale that is not complete: payment first, since marking it paid is
 * the closing fact for everyone, then the invoice while the account invoices and nothing stands for
 * one. Null for a complete sale, any other status, or one waiting on ARCA.
 */
export function saleNextStep(record: ClosureFacts, invoicingEnabled: boolean): SaleNextStep | null {
  if (!closable(record) || record.saleComplete) return null;
  if (!record.paidAt) return 'MARK_PAID';
  return awaitsInvoice(record, invoicingEnabled) ? 'INVOICE' : null;
}

/*
 * Every closing action that applies to the row now, in the order the menu lists them. Invoicing
 * does not wait for the payment: the corralón decides whether to invoice an unpaid sale.
 */
export function closingActions(record: ClosureFacts, invoicingEnabled: boolean): ClosingAction[] {
  if (!closable(record)) return [];
  const actions: ClosingAction[] = [record.paidAt ? 'UNMARK_PAID' : 'MARK_PAID'];
  if (awaitsInvoice(record, invoicingEnabled)) actions.push('INVOICE', 'WAIVE');
  if (invoicingEnabled && record.invoiceWaivedAt) actions.push('UNWAIVE');
  return actions;
}
