'use client';

import { useState, useTransition } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ReceiptTextIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Card,
  EmptyState,
  InlineLink,
  PendingButton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { IssueInvoiceDialog } from '@/app/(protected)/rfqs/_components/issue-invoice-dialog';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import type { SaleAwaitingInvoice } from '@/lib/api/invoices';
import { formatRfqReference } from '@/lib/api/rfqs';
import { setInvoiceWaived } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

interface PendingInvoicesTableProps {
  sales: SaleAwaitingInvoice[];
  // With every branch in view, each row says which one it belongs to.
  showBranch: boolean;
}

/*
 * «Por facturar»: the accepted sales still waiting for an invoice, each with the two ways out of the
 * list — issuing it, or marking that it needs none. Both write in the sale's own branch, and the
 * page is read again afterwards so the row leaves the list.
 */
export function PendingInvoicesTable({ sales, showBranch }: PendingInvoicesTableProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('invoicing.pending');
  const tClosing = useTranslations('rfqs.list.closing');
  const tWaiver = useTranslations('invoicing.card.waiver');
  const waiverMessage = useApiErrorMessage('invoicing.card');
  const [invoicing, setInvoicing] = useState<SaleAwaitingInvoice | null>(null);
  const [waiving, setWaiving] = useState<string | null>(null);
  // Waived rows leave at once; the refresh that follows confirms it.
  const [waived, setWaived] = useState<ReadonlySet<string>>(new Set());
  const [, startRefresh] = useTransition();

  const shown = useHeldWhileClosed(invoicing, invoicing !== null);
  const visible = sales.filter((sale) => !waived.has(sale.quoteId));

  function reload() {
    startRefresh(() => router.refresh());
  }

  async function waive(sale: SaleAwaitingInvoice, reference: string) {
    if (waiving) return;
    setWaiving(sale.quoteId);
    try {
      await setInvoiceWaived(sale.quoteId, true, sale.branchId);
      setWaived((previous) => new Set(previous).add(sale.quoteId));
      toast.success(tWaiver('waived', { reference }));
      reload();
    } catch (error) {
      toast.error(waiverMessage(errorCodeOf(error)));
    } finally {
      setWaiving(null);
    }
  }

  if (visible.length === 0) {
    return <EmptyState icon={ReceiptTextIcon} title={t(showBranch ? 'emptyAll' : 'empty')} />;
  }

  return (
    <>
      <Card className="py-0 gap-y-0 overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead kind="short">{t('order')}</TableHead>
              <TableHead kind="text">{t('client')}</TableHead>
              {showBranch ? <TableHead kind="text">{t('branch')}</TableHead> : null}
              <TableHead kind="date">{t('accepted')}</TableHead>
              <TableHead kind="money">{t('total')}</TableHead>
              <TableHead kind="status">{t('payment')}</TableHead>
              <TableHead kind="actionsWide">{t('actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.map((sale) => {
              const reference = formatRfqReference(sale.quoteNumber) ?? '';
              return (
                <TableRow key={sale.quoteId}>
                  <TableCell kind="short">
                    <InlineLink asChild>
                      <Link href={ROUTES.rfqsDetail(sale.rfqId)}>{reference}</Link>
                    </InlineLink>
                  </TableCell>
                  <TableCell>
                    {sale.client || <span className="text-foreground-muted">{t('noClient')}</span>}
                  </TableCell>
                  {showBranch ? <TableCell>{sale.branchName}</TableCell> : null}
                  <TableCell kind="date">{fmt.dateNumeric(sale.acceptedAt)}</TableCell>
                  <TableCell kind="money">{fmt.currency(sale.total)}</TableCell>
                  <TableCell kind="status">
                    <Badge tone={sale.paidAt ? 'success' : 'outline'}>
                      {t(sale.paidAt ? 'paid' : 'unpaid')}
                    </Badge>
                  </TableCell>
                  <TableCell kind="actionsWide">
                    <div className="flex items-center justify-center gap-x-2">
                      <Button
                        variant="outline"
                        size="sm"
                        aria-label={tClosing('labelled', {
                          action: tClosing('INVOICE'),
                          id: reference,
                        })}
                        onClick={() => setInvoicing(sale)}
                      >
                        {tClosing('INVOICE')}
                      </Button>
                      <PendingButton
                        variant="ghost"
                        size="sm"
                        pending={waiving === sale.quoteId}
                        pendingLabel={tWaiver('pending')}
                        disabled={waiving !== null}
                        aria-label={tClosing('labelled', {
                          action: tClosing('WAIVE'),
                          id: reference,
                        })}
                        onClick={() => waive(sale, reference)}
                      >
                        {tClosing('WAIVE')}
                      </PendingButton>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </Card>
      {shown ? (
        <IssueInvoiceDialog
          open={invoicing !== null}
          onOpenChange={(open) => !open && setInvoicing(null)}
          quoteId={shown.quoteId}
          branchId={shown.branchId}
          onIssued={reload}
          onFailed={reload}
        />
      ) : null}
    </>
  );
}
